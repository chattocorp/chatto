package core

import (
	"context"
	"errors"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/notification/v1"
	"hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
	"slices"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/parallel"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

const (
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
// notification effects. It makes each decision from current projected state,
// then stores self-contained durable output before it acknowledges EVT. EVT
// contains no notification-only planning events and RUNTIME_STATE contains no
// notification work queue.
type NotificationMaterializer struct {
	core                      *ChattoCore
	decisions                 events.ProjectionHandle[*NotificationDecisionProjection]
	assignConfiguredOwnerRole func(context.Context, string) error
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
		ready: make(chan struct{}),
	}
	return materializer
}

// Initialize creates the DeliverNew consumer before projectors start. This
// closes the gap in which a source fact could commit before durable work
// discovery exists.
func (m *NotificationMaterializer) Initialize(ctx context.Context) error {
	consumer, err := m.createConsumer(ctx)
	if err != nil {
		return err
	}
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

	err = worker.Run(ctx)
	if err == nil && ctx.Err() == nil {
		return errors.New("notification worker stopped unexpectedly")
	}
	return err
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
	var event evtv1.Event
	if err := proto.Unmarshal(delivery.Data, &event); err != nil {
		m.core.logger.Error("Terminating malformed notification delivery", "error", err)
		return events.TerminateDelivery("invalid Chatto event envelope", err)
	}
	position := events.SubjectPosition(delivery.Subject, delivery.StreamSequence)
	// Every worker subject is also a logical Notification Decisions subject.
	// Wait until the local current-state projection includes this source fact;
	// it may include later facts too, which intentionally affect the decision.
	if err := m.decisions.Projector().WaitFor(ctx, position); err != nil {
		return fmt.Errorf("wait for notification decision projection: %w", err)
	}
	switch event.GetEvent().(type) {
	case *evtv1.Event_UserAccountDeleted, *evtv1.Event_UserVerifiedEmailAdded:
		if err := m.core.userModel.waitForUsers(ctx, position); err != nil {
			return fmt.Errorf("wait for user projection: %w", err)
		}
	case *evtv1.Event_RbacRoleDeleted,
		*evtv1.Event_RbacRoleAssigned,
		*evtv1.Event_RbacRoleRevoked,
		*evtv1.Event_RbacPermissionGranted,
		*evtv1.Event_RbacPermissionDenied,
		*evtv1.Event_RbacPermissionCleared:
		if err := m.core.rbacModel.waitFor(ctx, position); err != nil {
			return fmt.Errorf("wait for RBAC projection: %w", err)
		}
	case *evtv1.Event_RoomAddedToGroup, *evtv1.Event_RoomRemovedFromGroup, *evtv1.Event_RoomGroupDeleted:
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
	return nil
}

func (m *NotificationMaterializer) materializeEvent(ctx context.Context, event *evtv1.Event, streamSequence uint64) error {
	if event == nil {
		return nil
	}
	if event.GetEvent() == nil && len(event.ProtoReflect().GetUnknown()) > 0 {
		return errUnsupportedNotificationEvent
	}
	visibilityAt := time.Now().UTC()
	switch payload := event.GetEvent().(type) {
	case *evtv1.Event_MessagePosted:
		if streamSequence == 0 {
			return invalidArgument("notification materialization requires an EVT stream sequence")
		}
		return m.materializeMessage(ctx, event, streamSequence, visibilityAt)
	case *evtv1.Event_ReactionAdded:
		if streamSequence == 0 {
			return invalidArgument("notification materialization requires an EVT stream sequence")
		}
		return m.materializeReaction(ctx, event, payload.ReactionAdded, streamSequence, visibilityAt)
	case *evtv1.Event_ReactionRemoved:
		if streamSequence == 0 {
			return invalidArgument("notification materialization requires an EVT stream sequence")
		}
		return m.removeReaction(ctx, event, payload.ReactionRemoved, streamSequence)
	case *evtv1.Event_MessageRetracted:
		_, err := m.core.notificationOccurrences.RemoveTarget(ctx, payload.MessageRetracted.GetRoomId(), payload.MessageRetracted.GetEventId())
		if err == nil {
			m.core.notificationOccurrences.publishUnreadMarkerTargetInvalidations(
				ctx, payload.MessageRetracted.GetRoomId(), payload.MessageRetracted.GetEventId(), event.GetActorId(), "",
			)
		}
		return err
	case *evtv1.Event_UserLeftRoom:
		if err := m.recordVisibilityBoundary(ctx, event.GetActorId(), payload.UserLeftRoom.GetRoomId(), streamSequence); err != nil {
			return err
		}
		_, err := m.core.notificationOccurrences.RemoveRoomForUser(ctx, event.GetActorId(), payload.UserLeftRoom.GetRoomId(), streamSequence)
		return err
	case *evtv1.Event_RoomMemberRemoved:
		if err := m.recordVisibilityBoundary(ctx, payload.RoomMemberRemoved.GetUserId(), payload.RoomMemberRemoved.GetRoomId(), streamSequence); err != nil {
			return err
		}
		_, err := m.core.notificationOccurrences.RemoveRoomForUser(ctx, payload.RoomMemberRemoved.GetUserId(), payload.RoomMemberRemoved.GetRoomId(), streamSequence)
		return err
	case *evtv1.Event_RoomMemberBanned:
		return m.reconcileOccurrenceVisibility(ctx, payload.RoomMemberBanned.GetUserId(), payload.RoomMemberBanned.GetRoomId(), streamSequence, visibilityAt)
	case *evtv1.Event_RoomUniversalChanged:
		return m.reconcileOccurrenceVisibility(ctx, "", payload.RoomUniversalChanged.GetRoomId(), streamSequence, visibilityAt)
	case *evtv1.Event_RoomAddedToGroup:
		return m.reconcileOccurrenceVisibility(ctx, "", payload.RoomAddedToGroup.GetRoomId(), streamSequence, visibilityAt)
	case *evtv1.Event_RoomRemovedFromGroup:
		return m.reconcileOccurrenceVisibility(ctx, "", payload.RoomRemovedFromGroup.GetRoomId(), streamSequence, visibilityAt)
	case *evtv1.Event_RoomGroupDeleted:
		return m.reconcileOccurrenceVisibility(ctx, "", "", streamSequence, visibilityAt)
	case *evtv1.Event_RoomDeleted:
		_, err := m.core.notificationOccurrences.RemoveRoom(ctx, payload.RoomDeleted.GetRoomId())
		return err
	case *evtv1.Event_UserAccountDeleted:
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
	case *evtv1.Event_UserVerifiedEmailAdded:
		return m.materializeConfiguredOwner(ctx, payload.UserVerifiedEmailAdded.GetUserId())
	case *evtv1.Event_RbacRoleAssigned:
		return m.reconcileOccurrenceVisibility(ctx, payload.RbacRoleAssigned.GetUserId(), "", streamSequence, visibilityAt)
	case *evtv1.Event_RbacRoleRevoked:
		return m.reconcileOccurrenceVisibility(ctx, payload.RbacRoleRevoked.GetUserId(), "", streamSequence, visibilityAt)
	case *evtv1.Event_RbacRoleDeleted:
		return m.reconcileOccurrenceVisibility(ctx, "", "", streamSequence, visibilityAt)
	case *evtv1.Event_RbacPermissionGranted:
		return m.reconcilePermissionVisibility(ctx, payload.RbacPermissionGranted.GetPermission(), payload.RbacPermissionGranted.GetScope(), payload.RbacPermissionGranted.GetSubject(), streamSequence, visibilityAt)
	case *evtv1.Event_RbacPermissionDenied:
		return m.reconcilePermissionVisibility(ctx, payload.RbacPermissionDenied.GetPermission(), payload.RbacPermissionDenied.GetScope(), payload.RbacPermissionDenied.GetSubject(), streamSequence, visibilityAt)
	case *evtv1.Event_RbacPermissionCleared:
		return m.reconcilePermissionVisibility(ctx, payload.RbacPermissionCleared.GetPermission(), payload.RbacPermissionCleared.GetScope(), payload.RbacPermissionCleared.GetSubject(), streamSequence, visibilityAt)
	}
	return nil
}

// materializeConfiguredOwner keeps owners.emails authorization represented by
// the same durable RBAC fact used by current notification visibility. The
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
	scope *evtv1.RbacPermissionScope,
	subject *evtv1.RbacPermissionSubject,
	streamSequence uint64,
	visibilityAt time.Time,
) error {
	if !notificationVisibilityPermission(permission) {
		return nil
	}
	var userID, roomID string
	if subject.GetKind() == evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER {
		userID = subject.GetId()
	}
	if scope.GetKind() == evtv1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_ROOM {
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
		marker *runtimestatev1.NotificationUnreadMarker
	}
	type visibilityCandidates struct {
		occurrences []*notificationv1.NotificationOccurrence
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
	type currentVisibility struct {
		broad       bool
		interaction bool
	}
	visibilityByPair := make(map[recipientRoom]currentVisibility, len(candidatesByPair))
	if err := m.decisions.Projection().withCurrent(visibilityAt, func(snapshot *notificationDecisionSnapshot) error {
		for pair := range candidatesByPair {
			visibilityByPair[pair] = currentVisibility{
				broad:       snapshot.notificationVisibilityExists(pair.recipientID, pair.roomID),
				interaction: snapshot.notificationInteractionVisibilityExists(pair.recipientID, pair.roomID),
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("derive current notification visibility: %w", err)
	}

	toRemove := make([]*notificationv1.NotificationOccurrence, 0)
	unreadInvalidations := make([]notificationUnreadInvalidation, 0)
	for pair, candidates := range candidatesByPair {
		visibility := visibilityByPair[pair]
		broadVisibility := visibility.broad
		interactionVisibility := visibility.interaction
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

func (m *NotificationMaterializer) notificationTargetHasInteraction(userID, roomID string, signal *notificationv1.NotificationSignal) bool {
	message := notificationSignalMessage(signal)
	if message == nil {
		return false
	}
	rootID, exists := m.core.roomModel.threadRootForMessage(roomID, message.GetEventId())
	return exists && m.core.roomModel.hasThreadInteraction(userID, roomID, rootID)
}

func (m *NotificationMaterializer) removeReaction(ctx context.Context, event *evtv1.Event, reaction *evtv1.ReactionRemovedEvent, streamSequence uint64) error {
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

func (m *NotificationMaterializer) materializeMessage(ctx context.Context, event *evtv1.Event, streamSequence uint64, evaluatedAt time.Time) error {
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
	room, err := m.core.FindRoomByID(ctx, message.GetRoomId())
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve notification source room: %w", err)
	}
	resolveActor := func(eventID string) (string, error) {
		if eventID == "" {
			return "", nil
		}
		target, err := m.core.GetRoomEventByEventID(ctx, KindOfRoom(room), room.GetId(), eventID)
		if err != nil {
			return "", err
		}
		return target.GetActorId(), nil
	}
	parentActorID, err := resolveActor(message.GetInReplyTo())
	if err != nil {
		return fmt.Errorf("resolve notification reply target: %w", err)
	}
	threadRootActorID, err := resolveActor(message.GetInThread())
	if err != nil {
		return fmt.Errorf("resolve notification thread root: %w", err)
	}
	var decisions []notificationRecipientDecision
	if err := m.decisions.Projection().withCurrent(evaluatedAt, func(snapshot *notificationDecisionSnapshot) error {
		decisions = buildMessageNotificationDecisions(snapshot, event, parentActorID, threadRootActorID)
		return nil
	}); err != nil {
		return fmt.Errorf("derive current message notification decisions: %w", err)
	}
	return m.materializeInputs(ctx, newNotificationOccurrenceInputs(event, decisions), streamSequence)
}

func (m *NotificationMaterializer) materializeReaction(ctx context.Context, event *evtv1.Event, reaction *evtv1.ReactionAddedEvent, streamSequence uint64, evaluatedAt time.Time) error {
	if createdAt := event.GetCreatedAt(); createdAt != nil && !createdAt.AsTime().UTC().Add(notificationTTL).After(time.Now().UTC()) {
		return nil
	}
	current := m.core.roomModel.reactionMutationSnapshot(reaction.GetRoomId(), reaction.GetMessageEventId(), reaction.GetEmoji(), event.GetActorId())
	if !current.Exists || current.SourceEventID != event.GetId() {
		return nil
	}
	room, err := m.core.FindRoomByID(ctx, reaction.GetRoomId())
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve reaction notification room: %w", err)
	}
	target, err := m.core.GetRoomEventByEventID(ctx, KindOfRoom(room), reaction.GetRoomId(), reaction.GetMessageEventId())
	if err != nil {
		return fmt.Errorf("resolve reaction notification target: %w", err)
	}
	if target == nil {
		return nil
	}
	var inputs []CreateNotificationOccurrenceInput
	if err := m.decisions.Projection().withCurrent(evaluatedAt, func(snapshot *notificationDecisionSnapshot) error {
		recipientID := target.GetActorId()
		_, active := snapshot.activeUsers[recipientID]
		if recipientID == "" || !active || recipientID == event.GetActorId() || !snapshot.notificationVisibilityExists(recipientID, reaction.GetRoomId()) {
			return nil
		}
		reference := newNotificationMessageReference(reaction.GetRoomId(), reaction.GetMessageEventId())
		if threadRootEventID := target.GetMessagePosted().GetInThread(); threadRootEventID != "" {
			reference.ThreadRootEventId = &threadRootEventID
		}
		signal := &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_ReactionReceived{ReactionReceived: &notificationv1.ReactionReceived{Message: reference, Emoji: reaction.GetEmoji()}}}
		mode := snapshot.effectiveNotificationMode(recipientID, reaction.GetRoomId(), signal)
		if notificationModeProducesAttention(mode) {
			inputs = newNotificationOccurrenceInputs(event, []notificationRecipientDecision{{recipientID: recipientID, signal: signal, mode: mode}})
		}
		return nil
	}); err != nil {
		return fmt.Errorf("derive current reaction notification decision: %w", err)
	}
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
		if input.Mode != evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE {
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
		if write.notify {
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
