package core

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/jetstreamutil"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	notificationMaterializerPollEvery = 250 * time.Millisecond
	notificationWorkerConsumerName    = "chatto-notification-materializer-v2"
	notificationWorkerAckWait         = time.Minute
	notificationWorkerHeartbeat       = 15 * time.Second
	notificationWorkerRetryDelay      = 10 * time.Second
	notificationWorkerAckTimeout      = 5 * time.Second
	// Notification lifecycle is causal: one shared in-flight delivery keeps a
	// later leave/retraction/removal behind the source it supersedes, including
	// when several Chatto replicas share the consumer.
	notificationWorkerMaxPending    = 1
	notificationWorkKeyPrefix       = "notification_work."
	notificationReadFenceKey        = "notification_v2.read_fence"
	maxNotificationWorkWriteRetries = 8
)

// NotificationMaterializer consumes existing domain facts and applies their
// notification effects. Exact source-time recipient decisions are staged as
// short-lived RUNTIME_STATE work records before the source fact commits; EVT
// contains no notification-only planning events.
type NotificationMaterializer struct {
	core                      *ChattoCore
	visibility                events.ProjectionHandle[*NotificationVisibilityProjection]
	assignConfiguredOwnerRole func(context.Context, string) error
	pollEvery                 time.Duration
	ready                     chan struct{}
	consumer                  jetstream.Consumer
	consumerInfoMu            sync.Mutex
}

func NewNotificationMaterializer(core *ChattoCore, visibility events.ProjectionHandle[*NotificationVisibilityProjection]) *NotificationMaterializer {
	materializer := &NotificationMaterializer{
		core:       core,
		visibility: visibility,
		assignConfiguredOwnerRole: func(ctx context.Context, userID string) error {
			return core.AssignServerRoleToExistingUser(ctx, SystemActorID, userID, RoleOwner)
		},
		pollEvery: notificationMaterializerPollEvery,
		ready:     make(chan struct{}),
	}
	return materializer
}

// Initialize creates the DeliverNew consumer before projectors start. Its
// acknowledged floor caps visibility snapshot restore, ensuring every pending
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
	m.visibility.Projection().SetAcknowledgedThrough(processed)
	m.consumer = consumer
	close(m.ready)
	return nil
}

func (m *NotificationMaterializer) Run(ctx context.Context) error {
	if err := m.core.WaitForProjectionsCurrent(ctx); err != nil {
		return fmt.Errorf("wait for projections before notification worker: %w", err)
	}
	if err := m.core.notificationOccurrences.WaitReady(ctx); err != nil {
		return fmt.Errorf("wait for notification index before worker: %w", err)
	}

	worker, err := events.NewDurableWorker(
		m.consumer,
		m.processDelivery,
		events.DurableWorkerOptions{
			MaxConcurrent:     notificationWorkerMaxPending,
			FetchMaxWait:      time.Second,
			RetryDelay:        notificationWorkerRetryDelay,
			AckTimeout:        notificationWorkerAckTimeout,
			HeartbeatInterval: notificationWorkerHeartbeat,
			Logger:            m.core.logger.WithPrefix("NotificationWorker"),
		},
	)
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
			m.releaseAcknowledgedVisibilityBoundaries(ctx)
		}
	}
}

func (m *NotificationMaterializer) releaseAcknowledgedVisibilityBoundaries(ctx context.Context) {
	// Capture the tail before consumer state, matching initialization. If the
	// later consumer read is idle, every worker fact through this earlier tail
	// is confirmed; a fact racing after the tail remains beyond the safe floor.
	tail, err := m.eventStreamTail(ctx)
	if err != nil {
		m.core.logger.Warn("Failed to read EVT tail for visibility cleanup", "error", err)
		return
	}
	info, err := m.consumerInfo(ctx)
	if err != nil {
		m.core.logger.Warn("Failed to read notification worker floor for visibility cleanup", "error", err)
		return
	}
	if err := m.visibility.Projection().ReleaseThrough(notificationAcknowledgedThrough(tail, info)); err != nil {
		m.core.logger.Warn("Failed to compact acknowledged notification visibility boundaries", "error", err)
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
// only confirmed bound, including when the pending fact is not a visibility
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
	return m.fenceLocalOccurrenceIndex(ctx, boundary)
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
	consumer, err := m.core.storage.serverEvtStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:        notificationWorkerConsumerName,
		Durable:     notificationWorkerConsumerName,
		Description: "Shared durable queue for Chatto notification materialization",
		// Prepared work exists only for events committed after Notifications
		// 2.0 starts. Beginning at the consumer's creation boundary avoids
		// replaying the server's entire message history on first rollout.
		DeliverPolicy:   jetstream.DeliverNewPolicy,
		AckPolicy:       jetstream.AckExplicitPolicy,
		AckWait:         notificationWorkerAckWait,
		MaxDeliver:      -1,
		FilterSubjects:  notificationWorkerFilterSubjects(),
		ReplayPolicy:    jetstream.ReplayInstantPolicy,
		MaxAckPending:   notificationWorkerMaxPending,
		MaxRequestBatch: notificationWorkerMaxPending,
	})
	if err != nil {
		return nil, fmt.Errorf("create notification materializer consumer: %w", err)
	}
	return consumer, nil
}

func (m *NotificationMaterializer) processDelivery(ctx context.Context, delivery events.DurableDelivery) error {
	var event corev1.Event
	if err := proto.Unmarshal(delivery.Data, &event); err != nil {
		m.core.logger.Error("Terminating malformed notification delivery", "error", err)
		return events.TerminateDelivery("invalid Chatto event envelope", err)
	}
	position := events.SubjectPosition(delivery.Subject, delivery.StreamSequence)
	hasVisibilityBoundary := notificationVisibilityBoundaryEvent(&event)
	if hasVisibilityBoundary {
		if err := m.visibility.Projector().WaitFor(ctx, position); err != nil {
			return fmt.Errorf("wait for notification visibility projection: %w", err)
		}
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
	case *corev1.Event_RoomAddedToGroup:
		if err := m.core.roomModel.waitForGroupLayout(ctx, position); err != nil {
			return fmt.Errorf("wait for room group projection: %w", err)
		}
	default:
		if err := m.core.roomModel.waitForLiveEVTEvent(ctx, position, &event); err != nil {
			return fmt.Errorf("wait for room projections: %w", err)
		}
	}
	if err := m.materializeEvent(ctx, &event, delivery.StreamSequence, true); err != nil {
		return err
	}
	return nil
}

// fenceLocalOccurrenceIndex appends a marker to the same KV stream as
// occurrences after the shared worker has acknowledged the captured EVT
// boundary. Observing its revision proves this replica's ordered watcher has
// applied every earlier occurrence mutation without adding a write to every
// worker delivery.
func (m *NotificationMaterializer) fenceLocalOccurrenceIndex(ctx context.Context, streamSequence uint64) error {
	value := make([]byte, 8)
	binary.BigEndian.PutUint64(value, streamSequence)
	revision, err := m.core.storage.runtimeStateKV.Put(ctx, notificationReadFenceKey, value)
	if err != nil {
		return fmt.Errorf("write notification read fence: %w", err)
	}
	if err := m.core.notificationOccurrences.index.waitForObservedRevision(ctx, revision); err != nil {
		return fmt.Errorf("wait for local notification index through read fence: %w", err)
	}
	return nil
}

// StoreWork writes exact prepared occurrences before their triggering domain
// event is appended. Orphans from a failed append expire at the same absolute
// 90-day boundary as the occurrence they would have created.
func (m *NotificationMaterializer) StoreWork(ctx context.Context, trigger *corev1.Event, work []*corev1.NotificationOccurrence) error {
	if m == nil {
		if len(work) == 0 {
			return nil
		}
		return errors.New("notification materializer is not configured")
	}
	if trigger == nil || trigger.GetId() == "" || trigger.GetCreatedAt() == nil {
		return nil
	}
	ttl := trigger.GetCreatedAt().AsTime().UTC().Add(notificationTTL).Sub(time.Now().UTC())
	if ttl <= 0 {
		return nil
	}
	desired := make(map[string][]byte, len(work))
	for _, occurrence := range work {
		if occurrence == nil || occurrence.GetRecipientId() == "" || occurrence.GetSourceEventId() == "" {
			return invalidArgument("notification work requires recipient_id and source_event_id")
		}
		data, err := proto.Marshal(occurrence)
		if err != nil {
			return fmt.Errorf("marshal notification work: %w", err)
		}
		desired[notificationWorkKey(trigger.GetId(), occurrence.GetRecipientId())] = data
	}

	// StoreWork can run more than once while an OCC mutation retries. A marker
	// means an earlier attempt prepared a complete set, so reconcile it exactly;
	// the overwhelmingly common first no-work decision remains one direct Get.
	existingKeys := make([]string, 0)
	markerKey := notificationWorkMarkerKey(trigger.GetId())
	_, markerErr := m.core.storage.runtimeStateKV.Get(ctx, markerKey)
	markerExists := markerErr == nil
	if markerErr != nil && !errors.Is(markerErr, jetstream.ErrKeyNotFound) && !errors.Is(markerErr, jetstream.ErrKeyDeleted) {
		return fmt.Errorf("read existing notification work marker: %w", markerErr)
	}
	if !markerExists && len(desired) == 0 {
		return nil
	}
	if markerExists {
		lister, err := m.core.storage.runtimeStateKV.ListKeysFiltered(ctx, notificationWorkFilter(trigger.GetId()))
		if err != nil && !errors.Is(err, jetstream.ErrNoKeysFound) {
			return fmt.Errorf("list existing notification work: %w", err)
		}
		if err == nil {
			for key := range lister.Keys() {
				existingKeys = append(existingKeys, key)
			}
		}
	}

	if len(desired) == 0 {
		if err := m.deleteRuntimeStateKey(ctx, markerKey); err != nil {
			return err
		}
	}
	for key, data := range desired {
		if err := m.putWorkWithTTL(ctx, key, data, ttl); err != nil {
			return err
		}
	}
	for _, key := range existingKeys {
		if _, keep := desired[key]; keep {
			continue
		}
		if err := m.deleteRuntimeStateKey(ctx, key); err != nil {
			return err
		}
	}
	if len(desired) == 0 {
		return nil
	}
	// The marker turns the overwhelmingly common no-work delivery into one
	// direct KV lookup. Recipient keys remain separate so one failed recipient
	// can be retried without rebuilding decisions for the others.
	return m.putWorkWithTTL(ctx, markerKey, nil, ttl)
}

func (m *NotificationMaterializer) deleteRuntimeStateKey(ctx context.Context, key string) error {
	for attempt := 0; attempt < maxNotificationWorkWriteRetries; attempt++ {
		entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read runtime-state key for deletion: %w", err)
		}
		err = m.core.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
		if err == nil || errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil
		}
		if !jetstreamutil.IsSequenceConflict(err) {
			return fmt.Errorf("delete runtime-state key: %w", err)
		}
	}
	return fmt.Errorf("delete runtime-state key after %d attempts", maxNotificationWorkWriteRetries)
}

func (m *NotificationMaterializer) putWorkWithTTL(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	for attempt := 0; attempt < maxNotificationWorkWriteRetries; attempt++ {
		entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			if _, err := m.core.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(ttl)); err == nil {
				return nil
			} else if !jetstreamutil.IsSequenceConflict(err) {
				return fmt.Errorf("create notification work: %w", err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("read notification work: %w", err)
		}
		_, err = m.core.js.Publish(
			ctx,
			"$KV.RUNTIME_STATE."+key,
			data,
			jetstream.WithExpectLastSequencePerSubject(entry.Revision()),
			jetstream.WithMsgTTL(ttl),
		)
		if err == nil {
			return nil
		}
		if !jetstreamutil.IsSequenceConflict(err) {
			return fmt.Errorf("update notification work: %w", err)
		}
	}
	return fmt.Errorf("write notification work after %d attempts", maxNotificationWorkWriteRetries)
}

func notificationWorkKey(triggerEventID, recipientID string) string {
	return notificationWorkKeyPrefix + triggerEventID + "." + recipientID
}

func notificationWorkMarkerKey(triggerEventID string) string {
	return notificationWorkKeyPrefix + triggerEventID
}

func notificationWorkFilter(triggerEventID string) string {
	return notificationWorkKeyPrefix + triggerEventID + ".*"
}

func (m *NotificationMaterializer) materializeEvent(ctx context.Context, event *corev1.Event, streamSequence uint64, durableDelivery bool) error {
	if event == nil {
		return nil
	}
	visibilityAt := time.Now().UTC()
	if event.GetCreatedAt() != nil {
		visibilityAt = event.GetCreatedAt().AsTime()
	}
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted, *corev1.Event_ReactionAdded:
		if streamSequence == 0 {
			return invalidArgument("notification materialization requires an EVT stream sequence")
		}
		_, err := m.materializeWork(ctx, event, streamSequence, durableDelivery)
		return err
	case *corev1.Event_ReactionRemoved:
		hadWork, err := m.materializeWork(ctx, event, streamSequence, durableDelivery)
		if err != nil || hadWork || streamSequence == 0 {
			return err
		}
		return m.removeReactionWithoutPreparedWork(ctx, event, payload.ReactionRemoved, streamSequence)
	case *corev1.Event_MessageRetracted:
		_, err := m.core.notificationOccurrences.RemoveTarget(ctx, payload.MessageRetracted.GetRoomId(), payload.MessageRetracted.GetEventId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_TARGET_RETRACTED)
		return err
	case *corev1.Event_UserLeftRoom:
		if err := m.recordVisibilityBoundary(ctx, event.GetActorId(), payload.UserLeftRoom.GetRoomId(), streamSequence); err != nil {
			return err
		}
		_, err := m.core.notificationOccurrences.RemoveRoomForUser(ctx, event.GetActorId(), payload.UserLeftRoom.GetRoomId(), streamSequence, corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST)
		return err
	case *corev1.Event_RoomMemberRemoved:
		if err := m.recordVisibilityBoundary(ctx, payload.RoomMemberRemoved.GetUserId(), payload.RoomMemberRemoved.GetRoomId(), streamSequence); err != nil {
			return err
		}
		_, err := m.core.notificationOccurrences.RemoveRoomForUser(ctx, payload.RoomMemberRemoved.GetUserId(), payload.RoomMemberRemoved.GetRoomId(), streamSequence, corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST)
		return err
	case *corev1.Event_RoomMemberBanned:
		return m.reconcileOccurrenceVisibility(ctx, payload.RoomMemberBanned.GetUserId(), payload.RoomMemberBanned.GetRoomId(), streamSequence, visibilityAt)
	case *corev1.Event_RoomUniversalChanged:
		return m.reconcileOccurrenceVisibility(ctx, "", payload.RoomUniversalChanged.GetRoomId(), streamSequence, visibilityAt)
	case *corev1.Event_RoomAddedToGroup:
		return m.reconcileOccurrenceVisibility(ctx, "", payload.RoomAddedToGroup.GetRoomId(), streamSequence, visibilityAt)
	case *corev1.Event_RoomDeleted:
		_, err := m.core.notificationOccurrences.RemoveRoom(ctx, payload.RoomDeleted.GetRoomId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST)
		return err
	case *corev1.Event_UserAccountDeleted:
		userID := payload.UserAccountDeleted.GetUserId()
		if _, err := m.core.notificationOccurrences.PurgeUser(ctx, userID); err != nil {
			return err
		}
		if err := m.core.notificationOccurrences.purgeNotificationReadBoundaries(ctx, userID); err != nil {
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
	if permission != string(PermRoomJoin) {
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
// moving it across permission scopes, or changing room.join RBAC. These facts
// are rare administrative operations, so an authoritative occurrence scan is
// preferable to maintaining another derived recipient index.
func (m *NotificationMaterializer) reconcileOccurrenceVisibility(ctx context.Context, userID, roomID string, streamSequence uint64, visibilityAt time.Time) error {
	entries, err := m.core.notificationOccurrences.storedOccurrenceEntries(ctx, userID)
	if err != nil {
		return err
	}
	type recipientRoom struct {
		recipientID string
		roomID      string
	}
	entriesByPair := make(map[recipientRoom][]notificationOccurrenceIndexEntry)
	for _, entry := range entries {
		occurrence := entry.occurrence
		targetRoomID := occurrence.GetTarget().GetRoomId()
		if occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED ||
			targetRoomID == "" || (roomID != "" && targetRoomID != roomID) ||
			(streamSequence != 0 && occurrence.GetSourceStreamSequence() >= streamSequence) {
			continue
		}
		pair := recipientRoom{recipientID: occurrence.GetRecipientId(), roomID: targetRoomID}
		entriesByPair[pair] = append(entriesByPair[pair], entry)
	}
	if len(entriesByPair) == 0 {
		return nil
	}
	snapshot, err := m.visibility.Projection().Boundary(streamSequence, visibilityAt)
	if err != nil {
		return err
	}

	for pair, pairEntries := range entriesByPair {
		if snapshot.membershipExists(pair.recipientID, pair.roomID) {
			continue
		}
		if err := m.recordVisibilityBoundary(ctx, pair.recipientID, pair.roomID, streamSequence); err != nil {
			return err
		}
		for _, entry := range pairEntries {
			written, removed, err := m.core.notificationOccurrences.deleteStoredOccurrence(
				ctx,
				pair.recipientID,
				entry.occurrence.GetSourceEventId(),
				corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST,
			)
			if err != nil {
				return err
			}
			if removed {
				m.core.publishNotificationOccurrenceChanged(ctx, written, false, true)
			}
		}
	}
	return nil
}

func (m *NotificationMaterializer) removeReactionWithoutPreparedWork(ctx context.Context, event *corev1.Event, reaction *corev1.ReactionRemovedEvent, streamSequence uint64) error {
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
	return err
}

func (m *NotificationMaterializer) materializeWork(ctx context.Context, event *corev1.Event, streamSequence uint64, durableDelivery bool) (bool, error) {
	// StoreWork uses the same absolute retention boundary, so an event older
	// than the notification TTL cannot have live prepared work. This also keeps
	// a lagging consumer from spending KV operations on already-expired facts.
	if createdAt := event.GetCreatedAt(); createdAt != nil && !createdAt.AsTime().UTC().Add(notificationTTL).After(time.Now().UTC()) {
		return false, nil
	}

	markerKey := notificationWorkMarkerKey(event.GetId())
	marker, err := m.core.storage.runtimeStateKV.Get(ctx, markerKey)
	if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read notification work marker: %w", err)
	}

	lister, err := m.core.storage.runtimeStateKV.ListKeysFiltered(ctx, notificationWorkFilter(event.GetId()))
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		if durableDelivery {
			return true, m.deleteWorkMarker(ctx, markerKey, marker.Revision())
		}
		return true, nil
	}
	if err != nil {
		return true, fmt.Errorf("list notification work: %w", err)
	}
	for key := range lister.Keys() {
		entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			continue
		}
		if err != nil {
			return true, fmt.Errorf("read notification work: %w", err)
		}
		var occurrence corev1.NotificationOccurrence
		if err := proto.Unmarshal(entry.Value(), &occurrence); err != nil {
			m.core.logger.Error("Discarding invalid notification work", "key", key, "error", err)
			if durableDelivery {
				if deleteErr := m.core.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); deleteErr != nil &&
					!errors.Is(deleteErr, jetstream.ErrKeyNotFound) && !errors.Is(deleteErr, jetstream.ErrKeyDeleted) {
					return true, fmt.Errorf("delete invalid notification work: %w", deleteErr)
				}
			}
			continue
		}
		if err := m.materializeOccurrence(ctx, event, &occurrence, streamSequence); err != nil {
			return true, err
		}
		if durableDelivery {
			if err := m.core.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err != nil &&
				!errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
				return true, fmt.Errorf("delete notification work: %w", err)
			}
		}
	}
	if durableDelivery {
		return true, m.deleteWorkMarker(ctx, markerKey, marker.Revision())
	}
	return true, nil
}

func (m *NotificationMaterializer) deleteWorkMarker(ctx context.Context, key string, revision uint64) error {
	for attempt := 0; attempt < maxNotificationWorkWriteRetries; attempt++ {
		err := m.core.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(revision))
		if err == nil || errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil
		}
		if !jetstreamutil.IsSequenceConflict(err) {
			return fmt.Errorf("delete notification work marker: %w", err)
		}
		entry, getErr := m.core.storage.runtimeStateKV.Get(ctx, key)
		if errors.Is(getErr, jetstream.ErrKeyNotFound) || errors.Is(getErr, jetstream.ErrKeyDeleted) {
			return nil
		}
		if getErr != nil {
			return fmt.Errorf("refresh notification work marker: %w", getErr)
		}
		revision = entry.Revision()
	}
	return fmt.Errorf("delete notification work marker after %d attempts", maxNotificationWorkWriteRetries)
}

func (m *NotificationMaterializer) materializeOccurrence(ctx context.Context, event *corev1.Event, occurrence *corev1.NotificationOccurrence, streamSequence uint64) error {
	if occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED {
		_, err := m.core.notificationOccurrences.RemoveSource(ctx, occurrence.GetRecipientId(), occurrence.GetSourceEventId(), occurrence.GetRemovalReason())
		return err
	}
	if occurrence.GetTarget() == nil || len(occurrence.GetReasons()) == 0 {
		return nil
	}
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		if _, retracted, known := m.core.roomModel.latestBody(event.GetId()); known && retracted {
			return nil
		}
	case *corev1.Event_ReactionAdded:
		reaction := payload.ReactionAdded
		snapshot := m.core.roomModel.reactionMutationSnapshot(reaction.GetRoomId(), reaction.GetMessageEventId(), reaction.GetEmoji(), event.GetActorId())
		if !snapshot.Exists || snapshot.SourceEventID != event.GetId() {
			return nil
		}
	default:
		return nil
	}
	visible, err := m.activeVisibleRecipient(ctx, occurrence.GetRecipientId(), occurrence.GetTarget().GetRoomId())
	if err != nil {
		return err
	}
	if !visible {
		return nil
	}
	afterBoundary, err := m.sourceAfterVisibilityBoundary(ctx, occurrence.GetRecipientId(), occurrence.GetTarget().GetRoomId(), streamSequence)
	if err != nil {
		return err
	}
	if !afterBoundary {
		return nil
	}
	createdOccurrence, _, err := m.core.notificationOccurrences.Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:          occurrence.GetRecipientId(),
		SourceEventID:        occurrence.GetSourceEventId(),
		SourceCreated:        occurrence.GetSourceCreatedAt().AsTime(),
		ActorID:              occurrence.GetActorId(),
		Target:               occurrence.GetTarget(),
		Reasons:              occurrence.GetReasons(),
		ReactionEmoji:        occurrence.GetReactionEmoji(),
		AttentionLevel:       occurrence.GetAttentionLevel(),
		SourceStreamSequence: streamSequence,
		EvaluatedAt:          occurrence.GetEvaluatedAt().AsTime(),
	})
	if err != nil {
		return fmt.Errorf("create occurrence for recipient %s: %w", occurrence.GetRecipientId(), err)
	}
	if err := m.core.notificationAlertDelivery.enqueue(ctx, createdOccurrence); err != nil {
		return err
	}
	return nil
}

func (m *NotificationMaterializer) activeVisibleRecipient(ctx context.Context, userID, roomID string) (bool, error) {
	active, err := m.activeRecipient(ctx, userID)
	if err != nil || !active {
		return active, err
	}
	room, err := m.core.FindRoomByID(ctx, roomID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("verify notification room: %w", err)
	}
	member, err := m.core.RoomMembershipExists(ctx, KindOfRoom(room), userID, roomID)
	if err != nil {
		return false, fmt.Errorf("verify notification room membership: %w", err)
	}
	return member, nil
}

func (m *NotificationMaterializer) activeRecipient(ctx context.Context, userID string) (bool, error) {
	_, err := m.core.GetUser(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}
