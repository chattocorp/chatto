package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/parallel"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	notificationMaterializerPollEvery = 250 * time.Millisecond
	// The durable name and its filter set form one schema-capability
	// generation. Adding a source event that older binaries cannot decode must
	// use a new consumer name so those binaries cannot acknowledge its work.
	notificationWorkerConsumerName = "chatto-notification-materializer-v1"
	notificationWorkerAckWait      = time.Minute
	notificationWorkerHeartbeat    = 15 * time.Second
	notificationWorkerRetryDelay   = 10 * time.Second
	notificationWorkerAckTimeout   = 5 * time.Second
	// Badge writes use distinct per-user keys. Bounded pipelining prevents a
	// large room post from serializing one broker round trip per recipient.
	notificationUnreadMarkerWriteConcurrency = 32
	// Notification lifecycle is causal: one shared in-flight delivery keeps a
	// later leave/retraction/removal behind the source it supersedes, including
	// when several Chatto replicas share the consumer.
	notificationWorkerMaxPending = 1
)

var errUnsupportedNotificationEvent = errors.New("unsupported notification event")

// NotificationMaterializer consumes existing domain facts and applies their
// notification effects. Its sequence-faithful projection reconstructs exact
// event-time recipient and policy decisions; EVT contains no notification-only
// planning events and RUNTIME_STATE contains no notification work queue.
type NotificationMaterializer struct {
	core                      *ChattoCore
	decisions                 events.ProjectionHandle[*NotificationDecisionProjection]
	assignConfiguredOwnerRole func(context.Context, string) error
	pollEvery                 time.Duration
	ready                     chan struct{}
	consumer                  jetstream.Consumer
	consumerInfoMu            sync.Mutex
}

func NewNotificationMaterializer(core *ChattoCore, decisions events.ProjectionHandle[*NotificationDecisionProjection]) *NotificationMaterializer {
	materializer := &NotificationMaterializer{
		core:      core,
		decisions: decisions,
		assignConfiguredOwnerRole: func(ctx context.Context, userID string) error {
			return core.AssignServerRoleToExistingUser(ctx, SystemActorID, userID, RoleOwner)
		},
		pollEvery: notificationMaterializerPollEvery,
		ready:     make(chan struct{}),
	}
	return materializer
}

// Initialize creates the DeliverNew consumer before projectors start. Its
// acknowledged floor caps decision-state snapshot restore, ensuring every pending
// administrative fact is replayed into an exact event-time boundary.
func (m *NotificationMaterializer) Initialize(ctx context.Context) error {
	consumer, err := m.createConsumer(ctx)
	if err != nil {
		return err
	}
	// Capture the stream tail before reading consumer state. If the consumer is
	// idle at the later read, every worker fact through this earlier tail is
	// acknowledged; facts racing after the tail remain beyond the restore cap.
	tail, err := m.eventStreamTail(ctx)
	if err != nil {
		return fmt.Errorf("read notification consumer initialization tail: %w", err)
	}
	info, err := consumer.Info(ctx)
	if err != nil {
		return fmt.Errorf("read notification consumer initialization floor: %w", err)
	}
	processed, err := m.initialNotificationAcknowledgedThrough(ctx, tail, info)
	if err != nil {
		return fmt.Errorf("reconstruct notification consumer initialization floor: %w", err)
	}
	m.decisions.Projection().SetAcknowledgedThrough(processed)
	m.consumer = consumer
	close(m.ready)
	return nil
}

func (m *NotificationMaterializer) Run(ctx context.Context) error {
	if err := m.core.WaitForProjectionsCurrent(ctx); err != nil {
		return fmt.Errorf("wait for projections before notification worker: %w", err)
	}
	if err := m.core.notificationOccurrences.WaitReady(ctx); err != nil {
		return fmt.Errorf("wait for notification projection before worker: %w", err)
	}

	worker, err := evtstream.NewEffectWorker(m.consumer, m.processDelivery, evtstream.EffectWorkerOptions{
		MaxConcurrent:     notificationWorkerMaxPending,
		RetryDelay:        notificationWorkerRetryDelay,
		AckTimeout:        notificationWorkerAckTimeout,
		HeartbeatInterval: notificationWorkerHeartbeat,
		Logger:            m.core.logger.WithPrefix("NotificationWorker"),
	})
	if err != nil {
		return fmt.Errorf("configure notification worker: %w", err)
	}

	workerDone := make(chan error, 1)
	go func() { workerDone <- worker.Run(ctx) }()

	ticker := time.NewTicker(m.pollEvery)
	defer ticker.Stop()
	for {
		select {
		case err := <-workerDone:
			if err == nil && ctx.Err() == nil {
				return errors.New("notification worker stopped unexpectedly")
			}
			return err
		case <-ctx.Done():
			return <-workerDone
		case <-ticker.C:
			m.releaseAcknowledgedDecisionBoundaries(ctx)
		}
	}
}

func (m *NotificationMaterializer) releaseAcknowledgedDecisionBoundaries(ctx context.Context) {
	// Capture the tail before consumer state, matching initialization. If the
	// later consumer read is idle, every worker fact through this earlier tail
	// is confirmed; a fact racing after the tail remains beyond the safe floor.
	tail, err := m.eventStreamTail(ctx)
	if err != nil {
		m.core.logger.Warn("Failed to read EVT tail for notification decision cleanup", "error", err)
		return
	}
	info, err := m.consumerInfo(ctx)
	if err != nil {
		m.core.logger.Warn("Failed to read notification worker floor for decision cleanup", "error", err)
		return
	}
	if err := m.decisions.Projection().ReleaseThrough(notificationAcknowledgedThrough(tail, info)); err != nil {
		m.core.logger.Warn("Failed to compact acknowledged notification decision boundaries", "error", err)
	}
}

// consumerInfo serializes nats.go's cached consumer metadata mutation. The
// materializer polls this from its lifecycle loop while request paths wait for
// read-your-writes, and the shared consumer handle is not safe for concurrent
// Info calls.
func (m *NotificationMaterializer) consumerInfo(ctx context.Context) (*jetstream.ConsumerInfo, error) {
	m.consumerInfoMu.Lock()
	defer m.consumerInfoMu.Unlock()
	return m.consumer.Info(ctx)
}

// eventStreamTail opens an isolated stream handle because nats.go mutates a
// handle's cached StreamInfo during Info while direct message reads inspect the
// same cache. The materializer polls concurrently with ordinary EVT reads and
// must not call Info through their shared handle.
func (m *NotificationMaterializer) eventStreamTail(ctx context.Context) (uint64, error) {
	stream, err := m.core.js.Stream(ctx, "EVT")
	if err != nil {
		return 0, err
	}
	info := stream.CachedInfo()
	if info == nil {
		return 0, fmt.Errorf("EVT stream info is unavailable")
	}
	return info.State.LastSeq, nil
}

// notificationAcknowledgedThrough returns a race-safe full-EVT floor for the
// filtered consumer. When the later consumer read is idle, no matching fact at
// or below the earlier tail can still be outstanding. Otherwise AckFloor is the
// only confirmed bound, including when the pending fact is not a decision
// boundary itself.
func notificationAcknowledgedThrough(tail uint64, info *jetstream.ConsumerInfo) uint64 {
	if info.NumPending == 0 && info.NumAckPending == 0 {
		return tail
	}
	return info.AckFloor.Stream
}

// initialNotificationAcknowledgedThrough reconstructs the full-EVT prefix
// immediately before the earliest fact that could still be pending for the
// filtered consumer. Unlike its sparse AckFloor, this bound remains derivable
// after restart without adding another persisted watermark.
func (m *NotificationMaterializer) initialNotificationAcknowledgedThrough(ctx context.Context, tail uint64, info *jetstream.ConsumerInfo) (uint64, error) {
	if info.NumPending == 0 && info.NumAckPending == 0 {
		return tail, nil
	}
	if info.AckFloor.Stream >= tail {
		return tail, nil
	}
	firstPending := uint64(0)
	for _, filter := range notificationWorkerFilterSubjects() {
		message, err := m.core.storage.serverEvtStream.GetMsg(ctx, info.AckFloor.Stream+1, jetstream.WithGetMsgSubject(filter))
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("read next notification fact for %q: %w", filter, err)
		}
		if firstPending == 0 || message.Sequence < firstPending {
			firstPending = message.Sequence
		}
	}
	if firstPending == 0 {
		// EVT is append-only in normal operation, so this is defensive. The raw
		// floor is conservative if consumer state and direct reads disagree.
		return info.AckFloor.Stream, nil
	}
	if firstPending > tail {
		return tail, nil
	}
	return firstPending - 1, nil
}

// WaitReady waits until the durable consumer exists. Serving must not begin
// before this boundary: DeliverNew can recover only source facts committed
// after the consumer was created.
func (m *NotificationMaterializer) WaitReady(ctx context.Context) error {
	select {
	case <-m.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WaitThrough waits until the shared durable consumer has acknowledged the
// triggering EVT sequence. Request paths use this only for read-your-writes;
// the worker remains the sole owner of occurrence creation and cleanup.
func (m *NotificationMaterializer) WaitThrough(ctx context.Context, streamSequence uint64) error {
	if m == nil || streamSequence == 0 {
		return nil
	}
	if err := m.WaitReady(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, err := m.consumerInfo(ctx)
		if err != nil {
			return fmt.Errorf("read notification consumer progress: %w", err)
		}
		if info.AckFloor.Stream >= streamSequence ||
			(info.Delivered.Consumer == 0 && info.NumAckPending == 0 && info.Delivered.Stream >= streamSequence) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// WaitCurrent captures the latest EVT sequence relevant to the notification
// worker and waits until the durable consumer has acknowledged it. A fresh
// DeliverNew consumer reports its creation-time stream floor as delivered with
// consumer sequence zero; that boundary is intentionally considered current
// because Notifications 2.0 does not replay older facts.
func (m *NotificationMaterializer) WaitCurrent(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if err := m.WaitReady(ctx); err != nil {
		return err
	}
	var boundary uint64
	for _, filter := range notificationWorkerFilterSubjects() {
		position, err := m.core.EventPublisher.LastSubjectPosition(ctx, filter)
		if err != nil {
			return fmt.Errorf("capture current notification worker boundary for %s: %w", filter, err)
		}
		if position.Seq > boundary {
			boundary = position.Seq
		}
	}
	if err := m.WaitThrough(ctx, boundary); err != nil {
		return err
	}
	return m.core.notificationOccurrences.projection.Projector().WaitForCurrent(ctx)
}

func notificationWorkerFilterSubjects() []string {
	return []string{
		evtstream.RoomEventTypeFilter(evtstream.EventMessagePosted),
		evtstream.RoomEventTypeFilter(evtstream.EventReactionAdded),
		evtstream.RoomEventTypeFilter(evtstream.EventReactionRemoved),
		evtstream.RoomEventTypeFilter(evtstream.EventMessageRetracted),
		evtstream.RoomEventTypeFilter(evtstream.EventUserLeftRoom),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomMemberRemoved),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomMemberBanned),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomUniversalChanged),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomDeleted),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomAddedToGroup),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomRemovedFromGroup),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomGroupDeleted),
		evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted),
		evtstream.UserEventTypeFilter(evtstream.EventUserVerifiedEmailAdded),
		evtstream.RBACEventTypeFilter(evtstream.EventRBACRoleDeleted),
		evtstream.RBACEventTypeFilter(evtstream.EventRBACRoleAssigned),
		evtstream.RBACEventTypeFilter(evtstream.EventRBACRoleRevoked),
		evtstream.RBACEventTypeFilter(evtstream.EventRBACPermissionGranted),
		evtstream.RBACEventTypeFilter(evtstream.EventRBACPermissionDenied),
		evtstream.RBACEventTypeFilter(evtstream.EventRBACPermissionCleared),
	}
}

func (m *NotificationMaterializer) createConsumer(ctx context.Context) (jetstream.Consumer, error) {
	filterSubjects := notificationWorkerFilterSubjects()
	existing, err := m.core.storage.serverEvtStream.Consumer(ctx, notificationWorkerConsumerName)
	if err == nil {
		info, infoErr := existing.Info(ctx)
		if infoErr != nil {
			return nil, fmt.Errorf("read existing notification materializer consumer: %w", infoErr)
		}
		if !sameNotificationWorkerFilterSubjects(filterSubjects, info.Config.FilterSubjects) {
			return nil, fmt.Errorf("notification materializer filter contract changed without a new consumer generation")
		}
	} else if !errors.Is(err, jetstream.ErrConsumerNotFound) {
		return nil, fmt.Errorf("read notification materializer consumer: %w", err)
	}
	// Notification derivation starts with Notifications 2.0. DeliverNew
	// begins at the consumer's creation boundary and avoids manufacturing
	// occurrences for the server's pre-upgrade message history on rollout.
	consumer, err := evtstream.CreateEffectConsumer(ctx, m.core.storage.serverEvtStream, evtstream.EffectConsumerConfig{
		Name:           notificationWorkerConsumerName,
		Description:    "Shared durable worker for Chatto notification materialization",
		FilterSubjects: filterSubjects,
		AckWait:        notificationWorkerAckWait,
		MaxAckPending:  notificationWorkerMaxPending,
		DeliverPolicy:  jetstream.DeliverNewPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("create notification materializer consumer: %w", err)
	}
	return consumer, nil
}

func sameNotificationWorkerFilterSubjects(left, right []string) bool {
	left = slices.Clone(left)
	right = slices.Clone(right)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}

func (m *NotificationMaterializer) processDelivery(ctx context.Context, delivery events.DurableDelivery) error {
	var event corev1.Event
	if err := proto.Unmarshal(delivery.Data, &event); err != nil {
		m.core.logger.Error("Terminating malformed notification delivery", "error", err)
		return events.TerminateDelivery("invalid Chatto event envelope", err)
	}
	position := events.SubjectPosition(delivery.Subject, delivery.StreamSequence)
	// Every worker subject is also a logical Notification Decisions subject.
	// Waiting for all deliveries means AdvanceThrough can consume every
	// intervening policy/membership/thread delta before moving its lagging
	// evaluator to this exact EVT sequence.
	if err := m.decisions.Projector().WaitFor(ctx, position); err != nil {
		return fmt.Errorf("wait for notification decision projection: %w", err)
	}
	switch event.GetEvent().(type) {
	case *corev1.Event_UserAccountDeleted, *corev1.Event_UserVerifiedEmailAdded:
		if err := m.core.userModel.waitForUsers(ctx, position); err != nil {
			return fmt.Errorf("wait for user projection: %w", err)
		}
	case *corev1.Event_RbacRoleDeleted,
		*corev1.Event_RbacRoleAssigned,
		*corev1.Event_RbacRoleRevoked,
		*corev1.Event_RbacPermissionGranted,
		*corev1.Event_RbacPermissionDenied,
		*corev1.Event_RbacPermissionCleared:
		if err := m.core.rbacModel.waitFor(ctx, position); err != nil {
			return fmt.Errorf("wait for RBAC projection: %w", err)
		}
	case *corev1.Event_RoomAddedToGroup, *corev1.Event_RoomRemovedFromGroup, *corev1.Event_RoomGroupDeleted:
		if err := m.core.roomModel.waitForGroupLayout(ctx, position); err != nil {
			return fmt.Errorf("wait for room group projection: %w", err)
		}
	default:
		if err := m.core.roomModel.waitForLiveEVTEvent(ctx, position, &event); err != nil {
			return fmt.Errorf("wait for room projections: %w", err)
		}
	}
	if err := m.materializeEvent(ctx, &event, delivery.StreamSequence); err != nil {
		return err
	}
	if err := m.decisions.Projection().AdvanceThrough(delivery.StreamSequence); err != nil {
		return fmt.Errorf("advance notification decision evaluator: %w", err)
	}
	return nil
}

func (m *NotificationMaterializer) materializeEvent(ctx context.Context, event *corev1.Event, streamSequence uint64) error {
	if event == nil {
		return nil
	}
	if event.GetEvent() == nil && len(event.ProtoReflect().GetUnknown()) > 0 {
		return errUnsupportedNotificationEvent
	}
	visibilityAt := time.Now().UTC()
	if event.GetCreatedAt() != nil {
		visibilityAt = event.GetCreatedAt().AsTime()
	}
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		if streamSequence == 0 {
			return invalidArgument("notification materialization requires an EVT stream sequence")
		}
		return m.materializeMessage(ctx, event, streamSequence, visibilityAt)
	case *corev1.Event_ReactionAdded:
		if streamSequence == 0 {
			return invalidArgument("notification materialization requires an EVT stream sequence")
		}
		return m.materializeReaction(ctx, event, payload.ReactionAdded, streamSequence, visibilityAt)
	case *corev1.Event_ReactionRemoved:
		if streamSequence == 0 {
			return invalidArgument("notification materialization requires an EVT stream sequence")
		}
		return m.removeReaction(ctx, event, payload.ReactionRemoved, streamSequence)
	case *corev1.Event_MessageRetracted:
		_, err := m.core.notificationOccurrences.RemoveTarget(ctx, payload.MessageRetracted.GetRoomId(), payload.MessageRetracted.GetEventId())
		if err == nil {
			m.core.notificationOccurrences.publishUnreadMarkerTargetInvalidations(
				ctx, payload.MessageRetracted.GetRoomId(), payload.MessageRetracted.GetEventId(), event.GetActorId(), "",
			)
		}
		return err
	case *corev1.Event_UserLeftRoom:
		if err := m.recordVisibilityBoundary(ctx, event.GetActorId(), payload.UserLeftRoom.GetRoomId(), streamSequence); err != nil {
			return err
		}
		_, err := m.core.notificationOccurrences.RemoveRoomForUser(ctx, event.GetActorId(), payload.UserLeftRoom.GetRoomId(), streamSequence)
		return err
	case *corev1.Event_RoomMemberRemoved:
		if err := m.recordVisibilityBoundary(ctx, payload.RoomMemberRemoved.GetUserId(), payload.RoomMemberRemoved.GetRoomId(), streamSequence); err != nil {
			return err
		}
		_, err := m.core.notificationOccurrences.RemoveRoomForUser(ctx, payload.RoomMemberRemoved.GetUserId(), payload.RoomMemberRemoved.GetRoomId(), streamSequence)
		return err
	case *corev1.Event_RoomMemberBanned:
		return m.reconcileOccurrenceVisibility(ctx, payload.RoomMemberBanned.GetUserId(), payload.RoomMemberBanned.GetRoomId(), streamSequence, visibilityAt)
	case *corev1.Event_RoomUniversalChanged:
		return m.reconcileOccurrenceVisibility(ctx, "", payload.RoomUniversalChanged.GetRoomId(), streamSequence, visibilityAt)
	case *corev1.Event_RoomAddedToGroup:
		return m.reconcileOccurrenceVisibility(ctx, "", payload.RoomAddedToGroup.GetRoomId(), streamSequence, visibilityAt)
	case *corev1.Event_RoomRemovedFromGroup:
		return m.reconcileOccurrenceVisibility(ctx, "", payload.RoomRemovedFromGroup.GetRoomId(), streamSequence, visibilityAt)
	case *corev1.Event_RoomGroupDeleted:
		return m.reconcileOccurrenceVisibility(ctx, "", "", streamSequence, visibilityAt)
	case *corev1.Event_RoomDeleted:
		_, err := m.core.notificationOccurrences.RemoveRoom(ctx, payload.RoomDeleted.GetRoomId())
		return err
	case *corev1.Event_UserAccountDeleted:
		userID := payload.UserAccountDeleted.GetUserId()
		if _, err := m.core.notificationOccurrences.PurgeUser(ctx, userID); err != nil {
			return err
		}
		if err := m.core.notificationOccurrences.purgeNotificationReadBoundaries(ctx, userID); err != nil {
			return err
		}
		if err := m.core.notificationOccurrences.purgeNotificationUnreadMarkers(ctx, userID); err != nil {
			return err
		}
		return m.purgeVisibilityBoundaries(ctx, userID)
	case *corev1.Event_UserVerifiedEmailAdded:
		return m.materializeConfiguredOwner(ctx, payload.UserVerifiedEmailAdded.GetUserId())
	case *corev1.Event_RbacRoleAssigned:
		return m.reconcileOccurrenceVisibility(ctx, payload.RbacRoleAssigned.GetUserId(), "", streamSequence, visibilityAt)
	case *corev1.Event_RbacRoleRevoked:
		return m.reconcileOccurrenceVisibility(ctx, payload.RbacRoleRevoked.GetUserId(), "", streamSequence, visibilityAt)
	case *corev1.Event_RbacRoleDeleted:
		return m.reconcileOccurrenceVisibility(ctx, "", "", streamSequence, visibilityAt)
	case *corev1.Event_RbacPermissionGranted:
		return m.reconcilePermissionVisibility(ctx, payload.RbacPermissionGranted.GetPermission(), payload.RbacPermissionGranted.GetScope(), payload.RbacPermissionGranted.GetSubject(), streamSequence, visibilityAt)
	case *corev1.Event_RbacPermissionDenied:
		return m.reconcilePermissionVisibility(ctx, payload.RbacPermissionDenied.GetPermission(), payload.RbacPermissionDenied.GetScope(), payload.RbacPermissionDenied.GetSubject(), streamSequence, visibilityAt)
	case *corev1.Event_RbacPermissionCleared:
		return m.reconcilePermissionVisibility(ctx, payload.RbacPermissionCleared.GetPermission(), payload.RbacPermissionCleared.GetScope(), payload.RbacPermissionCleared.GetSubject(), streamSequence, visibilityAt)
	}
	return nil
}

// materializeConfiguredOwner keeps owners.emails authorization represented by
// the same durable RBAC fact used by event-time notification visibility. The
// source email fact remains pending and is redelivered until this converges.
func (m *NotificationMaterializer) materializeConfiguredOwner(ctx context.Context, userID string) error {
	if userID == "" || len(m.core.config.Owners.Emails) == 0 {
		return nil
	}
	emails, err := m.core.userModel.verifiedEmails(ctx, userID)
	if err != nil {
		return fmt.Errorf("read configured-owner verified emails: %w", err)
	}
	for _, verified := range emails {
		if !m.core.config.Owners.IsServerOwnerEmail(verified.Email) {
			continue
		}
		if m.core.rbacModel.hasRole(userID, RoleOwner) {
			return nil
		}
		if err := m.assignConfiguredOwnerRole(ctx, userID); err != nil {
			return fmt.Errorf("materialize configured-owner role: %w", err)
		}
		return nil
	}
	return nil
}

func (m *NotificationMaterializer) reconcilePermissionVisibility(
	ctx context.Context,
	permission string,
	scope *corev1.RbacPermissionScope,
	subject *corev1.RbacPermissionSubject,
	streamSequence uint64,
	visibilityAt time.Time,
) error {
	if !notificationVisibilityPermission(permission) {
		return nil
	}
	var userID, roomID string
	if subject.GetKind() == corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER {
		userID = subject.GetId()
	}
	if scope.GetKind() == corev1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_ROOM {
		roomID = scope.GetId()
	}
	return m.reconcileOccurrenceVisibility(ctx, userID, roomID, streamSequence, visibilityAt)
}

// reconcileOccurrenceVisibility handles effective membership changes that do
// not emit an explicit leave event, such as disabling a universal room,
// moving it across permission scopes, or changing room.join or message.read
// RBAC. These facts are rare administrative operations. The materializer is
// the sole lifecycle writer. The NOTIFICATIONS projection and the Badge marker
// index supply the complete candidate set. Selected occurrence removals and
// visibility boundaries are committed as idempotent lifecycle facts.
func (m *NotificationMaterializer) reconcileOccurrenceVisibility(ctx context.Context, userID, roomID string, streamSequence uint64, visibilityAt time.Time) error {
	entries := m.core.notificationOccurrences.projection.Projection().allOccurrences(m.core.notificationOccurrences.now().UTC())
	type recipientRoom struct {
		recipientID string
		roomID      string
	}
	type unreadMarkerCandidate struct {
		scope  notificationReadBoundaryScope
		marker *corev1.NotificationUnreadMarker
	}
	type visibilityCandidates struct {
		occurrences []*corev1.NotificationOccurrence
		markers     []unreadMarkerCandidate
	}
	candidatesByPair := make(map[recipientRoom]*visibilityCandidates)
	candidatesFor := func(pair recipientRoom) *visibilityCandidates {
		candidates := candidatesByPair[pair]
		if candidates == nil {
			candidates = &visibilityCandidates{}
			candidatesByPair[pair] = candidates
		}
		return candidates
	}
	for _, occurrence := range entries {
		message := notificationSignalMessage(occurrence.GetSignal())
		if message == nil {
			continue
		}
		targetRoomID := message.GetRoomId()
		if (userID != "" && occurrence.GetRecipientId() != userID) || targetRoomID == "" || (roomID != "" && targetRoomID != roomID) ||
			(streamSequence != 0 && occurrence.GetSourceStreamSequence() >= streamSequence) {
			continue
		}
		pair := recipientRoom{recipientID: occurrence.GetRecipientId(), roomID: targetRoomID}
		candidates := candidatesFor(pair)
		candidates.occurrences = append(candidates.occurrences, occurrence)
	}
	for _, scope := range m.core.notificationBoundaries.unreadMarkerScopes(userID, roomID, streamSequence) {
		marker, _, exists, err := m.core.notificationBoundaries.unreadMarker(ctx, scope)
		if err != nil {
			return err
		}
		if !exists || marker == nil {
			continue
		}
		pair := recipientRoom{recipientID: scope.userID, roomID: scope.roomID}
		candidates := candidatesFor(pair)
		candidates.markers = append(candidates.markers, unreadMarkerCandidate{scope: scope, marker: marker})
	}
	if len(candidatesByPair) == 0 {
		return nil
	}
	snapshot, err := m.decisions.Projection().Boundary(streamSequence, visibilityAt)
	if err != nil {
		return err
	}

	toRemove := make([]*corev1.NotificationOccurrence, 0)
	unreadInvalidations := make([]notificationUnreadInvalidation, 0)
	for pair, candidates := range candidatesByPair {
		broadVisibility := snapshot.notificationVisibilityExists(pair.recipientID, pair.roomID)
		interactionVisibility := snapshot.notificationInteractionVisibilityExists(pair.recipientID, pair.roomID)
		if !broadVisibility && !interactionVisibility {
			if err := m.recordVisibilityBoundary(ctx, pair.recipientID, pair.roomID, streamSequence); err != nil {
				return err
			}
		}
		for _, occurrence := range candidates.occurrences {
			if broadVisibility || (interactionVisibility && m.notificationTargetHasInteraction(pair.recipientID, pair.roomID, occurrence.GetSignal())) {
				continue
			}
			toRemove = append(toRemove, occurrence)
		}
		if !interactionVisibility || broadVisibility {
			continue
		}
		for _, candidate := range candidates.markers {
			if m.notificationTargetHasInteraction(pair.recipientID, pair.roomID, candidate.marker.GetSignal()) {
				continue
			}
			deleted, err := m.core.notificationOccurrences.deleteNotificationUnreadMarkerBefore(ctx, candidate.scope, streamSequence)
			if err != nil {
				return err
			}
			if deleted {
				unreadInvalidations = append(unreadInvalidations, notificationUnreadInvalidation{
					userID: pair.recipientID, roomID: pair.roomID, threadRootEventID: candidate.scope.threadRootEventID,
				})
			}
		}
	}
	if len(toRemove) > 0 {
		if _, err := m.core.notificationOccurrences.deleteOccurrences(ctx, toRemove); err != nil {
			return err
		}
	}
	m.core.publishNotificationUnreadInvalidations(ctx, unreadInvalidations)
	return nil
}

func (m *NotificationMaterializer) notificationTargetHasInteraction(userID, roomID string, signal *corev1.NotificationSignal) bool {
	message := notificationSignalMessage(signal)
	if message == nil {
		return false
	}
	rootID, exists := m.core.roomModel.threadRootForMessage(roomID, message.GetEventId())
	return exists && m.core.roomModel.hasThreadInteraction(userID, roomID, rootID)
}

func (m *NotificationMaterializer) removeReaction(ctx context.Context, event *corev1.Event, reaction *corev1.ReactionRemovedEvent, streamSequence uint64) error {
	room, err := m.core.FindRoomByID(ctx, reaction.GetRoomId())
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve reaction-removal room: %w", err)
	}
	target, err := m.core.GetRoomEventByEventID(ctx, KindOfRoom(room), room.GetId(), reaction.GetMessageEventId())
	if err != nil {
		return fmt.Errorf("resolve reaction-removal target: %w", err)
	}
	if target == nil || target.GetActorId() == "" || target.GetActorId() == event.GetActorId() {
		return nil
	}
	_, err = m.core.notificationOccurrences.RemoveReaction(
		ctx,
		target.GetActorId(),
		reaction.GetRoomId(),
		reaction.GetMessageEventId(),
		event.GetActorId(),
		reaction.GetEmoji(),
		streamSequence,
	)
	if err == nil {
		m.core.notificationOccurrences.publishUnreadMarkerTargetInvalidations(
			ctx, reaction.GetRoomId(), reaction.GetMessageEventId(), event.GetActorId(), reaction.GetEmoji(),
		)
	}
	return err
}

func (m *NotificationMaterializer) materializeMessage(ctx context.Context, event *corev1.Event, streamSequence uint64, evaluatedAt time.Time) error {
	if createdAt := event.GetCreatedAt(); createdAt != nil && !createdAt.AsTime().UTC().Add(notificationTTL).After(time.Now().UTC()) {
		return nil
	}
	message := event.GetMessagePosted()
	if message == nil || message.GetEchoOfEventId() != "" {
		return nil
	}
	if _, retracted, known := m.core.roomModel.latestBody(event.GetId()); known && retracted {
		return nil
	}
	snapshot, err := m.decisions.Projection().Boundary(streamSequence, evaluatedAt)
	if err != nil {
		return err
	}
	decisions, err := m.core.buildMessageNotificationDecisionsAt(ctx, snapshot, event)
	if err != nil {
		return fmt.Errorf("derive message notification decisions: %w", err)
	}
	return m.materializeInputs(ctx, newNotificationOccurrenceInputs(event, decisions), streamSequence)
}

func (m *NotificationMaterializer) materializeReaction(ctx context.Context, event *corev1.Event, reaction *corev1.ReactionAddedEvent, streamSequence uint64, evaluatedAt time.Time) error {
	if createdAt := event.GetCreatedAt(); createdAt != nil && !createdAt.AsTime().UTC().Add(notificationTTL).After(time.Now().UTC()) {
		return nil
	}
	current := m.core.roomModel.reactionMutationSnapshot(reaction.GetRoomId(), reaction.GetMessageEventId(), reaction.GetEmoji(), event.GetActorId())
	if !current.Exists || current.SourceEventID != event.GetId() {
		return nil
	}
	snapshot, err := m.decisions.Projection().Boundary(streamSequence, evaluatedAt)
	if err != nil {
		return err
	}
	roomKind, exists := snapshot.roomKind(reaction.GetRoomId())
	if !exists {
		return nil
	}
	target, err := m.core.GetRoomEventByEventID(ctx, roomKind, reaction.GetRoomId(), reaction.GetMessageEventId())
	if err != nil {
		return fmt.Errorf("resolve reaction notification target: %w", err)
	}
	if target == nil {
		return nil
	}
	recipientID := target.GetActorId()
	_, active := snapshot.activeUsers[recipientID]
	if recipientID == "" || !active || recipientID == event.GetActorId() || !snapshot.notificationVisibilityExists(recipientID, reaction.GetRoomId()) {
		return nil
	}
	reference := newNotificationMessageReference(reaction.GetRoomId(), reaction.GetMessageEventId())
	if threadRootEventID := target.GetMessagePosted().GetInThread(); threadRootEventID != "" {
		reference.ThreadRootEventId = &threadRootEventID
	}
	signal := &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_ReactionReceived{ReactionReceived: &corev1.ReactionReceived{Message: reference, Emoji: reaction.GetEmoji()}}}
	mode := snapshot.effectiveNotificationMode(recipientID, reaction.GetRoomId(), signal)
	if !notificationModeProducesAttention(mode) {
		return nil
	}
	inputs := newNotificationOccurrenceInputs(event, []notificationRecipientDecision{{
		recipientID: recipientID,
		signal:      signal,
		mode:        mode,
	}})
	for _, input := range inputs {
		if err := m.materializeInput(ctx, input, streamSequence); err != nil {
			return err
		}
	}
	return nil
}

func (m *NotificationMaterializer) materializeInput(ctx context.Context, input CreateNotificationOccurrenceInput, streamSequence uint64) error {
	return m.materializeInputs(ctx, []CreateNotificationOccurrenceInput{input}, streamSequence)
}

func (m *NotificationMaterializer) materializeInputs(ctx context.Context, inputs []CreateNotificationOccurrenceInput, streamSequence uint64) error {
	eligible := make([]CreateNotificationOccurrenceInput, 0, len(inputs))
	for _, input := range inputs {
		message := notificationSignalMessage(input.Signal)
		if message == nil {
			continue
		}
		afterBoundary, err := m.sourceAfterVisibilityBoundary(ctx, input.RecipientID, message.GetRoomId(), streamSequence)
		if err != nil {
			return err
		}
		if !afterBoundary {
			continue
		}
		input.SourceStreamSequence = streamSequence
		eligible = append(eligible, input)
	}
	badgeInputs := make([]CreateNotificationOccurrenceInput, 0, len(eligible))
	seenMarkerKeys := make(map[string]struct{})
	for _, input := range eligible {
		if input.Mode != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE {
			continue
		}
		message := notificationSignalMessage(input.Signal)
		key := notificationUnreadMarkerKey(input.RecipientID, message.GetRoomId(), message.GetThreadRootEventId())
		if _, exists := seenMarkerKeys[key]; exists {
			continue
		}
		seenMarkerKeys[key] = struct{}{}
		badgeInputs = append(badgeInputs, input)
	}
	writes, err := parallel.Map(ctx, notificationUnreadMarkerWriteConcurrency, badgeInputs,
		func(ctx context.Context, _ int, input CreateNotificationOccurrenceInput) (notificationUnreadMarkerWrite, error) {
			return m.core.notificationOccurrences.writeNotificationUnreadMarker(ctx, input)
		},
	)
	if err != nil {
		return fmt.Errorf("record notification unread markers: %w", err)
	}
	var barrier notificationUnreadMarkerWrite
	for _, write := range writes {
		if write.revision > barrier.revision {
			barrier = write
		}
	}
	if barrier.revision != 0 {
		if err := m.core.notificationBoundaries.waitForRevision(ctx, barrier.key, barrier.revision); err != nil {
			return fmt.Errorf("wait for notification unread markers: %w", err)
		}
	}
	invalidations := make([]notificationUnreadInvalidation, 0, len(writes))
	for index, write := range writes {
		if write.changed {
			input := badgeInputs[index]
			message := notificationSignalMessage(input.Signal)
			invalidations = append(invalidations, notificationUnreadInvalidation{
				userID: input.RecipientID, actorID: input.ActorID,
				roomID: message.GetRoomId(), threadRootEventID: message.GetThreadRootEventId(),
			})
		}
	}
	m.core.publishNotificationUnreadInvalidations(ctx, invalidations)
	if err := m.core.notificationOccurrences.CreateMany(ctx, eligible); err != nil {
		return fmt.Errorf("create notification occurrences: %w", err)
	}
	return nil
}
