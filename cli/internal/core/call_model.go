package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	lkauth "github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/twitchtv/twirp"
	"golang.org/x/sync/errgroup"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/jetstreamutil"
	"hmans.de/chatto/internal/kms"
	"hmans.de/chatto/internal/lease"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	callReconcileInterval             = 30 * time.Second
	callReconcileAPITimeout           = 10 * time.Second
	callReconcileMaxRetries           = 5
	callReconcileListFailureThreshold = 3
	callReconcileLeaseName            = "livekit_reconciler"
	callReconcileLeaseTTL             = 45 * time.Second
	callReconcileLeaseRenewEvery      = 15 * time.Second
	callReconcileLeaseRetryEvery      = 5 * time.Second
	liveKitReconcileFailureKey        = "livekit.reconciliation.list_failures"
	callKeyCleanupConsumerName        = "chatto-call-key-cleanup-v1"
	callKeyCleanupMaxPending          = 16
	callKeyCleanupAckWait             = 2 * time.Minute
	callKeyCleanupRetryDelay          = 30 * time.Second
	callKeyCleanupHeartbeat           = 30 * time.Second
	callKeyCleanupAckTimeout          = 5 * time.Second
)

type liveKitParticipantSnapshot struct {
	LegacySpaceID string
	RoomID        string
	CallID        string
	UserIDs       []string
}

type liveKitParticipantLister interface {
	ListCallParticipants(ctx context.Context) ([]liveKitParticipantSnapshot, error)
}

type liveKitParticipantRemover interface {
	RemoveCallParticipant(ctx context.Context, legacySpaceID, roomID, callID, userID string) error
}

type CallModel struct {
	publisher          *evtstream.Publisher
	callState          events.ProjectionHandle[*CallStateProjection]
	callKeys           kms.CallKeyStore
	livekit            liveKitParticipantLister
	reconcileLease     *lease.Lease
	memoryCacheKV      jetstream.KeyValue
	logger             events.Logger
	keyCleanupConsumer jetstream.Consumer
	keyCleanupWorker   *events.DurableWorker
}

type liveKitFailureCleanupSummary struct {
	activeRooms   int
	endedRooms    int
	cleanupErrors int
	err           error
}

type liveKitListFailureError struct {
	err                 error
	cleanup             liveKitFailureCleanupSummary
	consecutiveFailures int
	threshold           int
	cleanupAttempted    bool
}

type liveKitListFailureState struct {
	Count     int       `json:"count"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (e *liveKitListFailureError) Error() string {
	if e == nil {
		return ""
	}
	if !e.cleanupAttempted {
		return fmt.Sprintf("list LiveKit call participants: %v; end active calls deferred after %d/%d failures", e.err, e.consecutiveFailures, e.threshold)
	}
	if e.cleanup.err != nil {
		return fmt.Sprintf("list LiveKit call participants: %v; end active calls after LiveKit reconciliation failure: %v", e.err, e.cleanup.err)
	}
	return fmt.Sprintf("list LiveKit call participants: %v; ended active calls after %d/%d failures", e.err, e.consecutiveFailures, e.threshold)
}

func (e *liveKitListFailureError) Unwrap() error {
	if e == nil {
		return nil
	}
	if e.cleanup.err != nil {
		return errors.Join(e.err, e.cleanup.err)
	}
	return e.err
}

func NewCallModel(
	publisher *evtstream.Publisher,
	callState events.ProjectionHandle[*CallStateProjection],
	callKeys kms.CallKeyStore,
	livekit liveKitParticipantLister,
	reconcileLease *lease.Lease,
	memoryCacheKV jetstream.KeyValue,
	logger events.Logger,
) *CallModel {
	model := &CallModel{
		publisher:      publisher,
		callState:      callState,
		callKeys:       callKeys,
		livekit:        livekit,
		reconcileLease: reconcileLease,
		memoryCacheKV:  memoryCacheKV,
		logger:         logger,
	}
	return model
}

func (s *CallModel) configureKeyCleanup(ctx context.Context, stream jetstream.Stream) error {
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:            callKeyCleanupConsumerName,
		Durable:         callKeyCleanupConsumerName,
		Description:     "Shared durable queue for ended Chatto call-key deletion",
		DeliverPolicy:   jetstream.DeliverAllPolicy,
		AckPolicy:       jetstream.AckExplicitPolicy,
		AckWait:         callKeyCleanupAckWait,
		MaxDeliver:      -1,
		FilterSubject:   evtstream.RoomEventTypeFilter(evtstream.EventCallEnded),
		ReplayPolicy:    jetstream.ReplayInstantPolicy,
		MaxAckPending:   callKeyCleanupMaxPending,
		MaxRequestBatch: callKeyCleanupMaxPending,
	})
	if err != nil {
		return fmt.Errorf("create call-key cleanup consumer: %w", err)
	}
	worker, err := events.NewDurableWorker(consumer, s.processKeyCleanupDelivery, events.DurableWorkerOptions{
		MaxConcurrent:     callKeyCleanupMaxPending,
		RetryDelay:        callKeyCleanupRetryDelay,
		AckTimeout:        callKeyCleanupAckTimeout,
		HeartbeatInterval: callKeyCleanupHeartbeat,
		Logger:            s.logger,
	})
	if err != nil {
		return fmt.Errorf("configure call-key cleanup worker: %w", err)
	}
	s.keyCleanupConsumer = consumer
	s.keyCleanupWorker = worker
	return nil
}

func (s *CallModel) waitFor(ctx context.Context, pos events.StreamPosition) error {
	if s == nil || s.callState.Projector() == nil {
		return fmt.Errorf("call state projector is not initialized")
	}
	return s.callState.Projector().WaitFor(ctx, pos)
}

func (s *CallModel) roomSnapshot(roomID string) CallRoomSnapshot {
	return s.callState.Projection().RoomSnapshot(roomID)
}

func (s *CallModel) activeCall(roomID string) (CallSession, bool) {
	return s.callState.Projection().ActiveCall(roomID)
}

func (s *CallModel) participants(roomID string) []CallParticipant {
	return s.callState.Projection().Participants(roomID)
}

func (s *CallModel) activeRoomIDs() []string {
	return s.callState.Projection().ActiveRoomIDs()
}

func (c *ChattoCore) EnableLiveKitCallReconciliation(cfg config.LiveKitConfig) error {
	if c.callModel == nil {
		return fmt.Errorf("call model is not initialized")
	}
	lister, err := newLiveKitParticipantLister(cfg)
	if err != nil {
		return err
	}
	c.callModel.livekit = lister
	return nil
}

func newLiveKitParticipantLister(cfg config.LiveKitConfig) (liveKitParticipantLister, error) {
	if !cfg.IsConfigured() {
		return nil, nil
	}
	httpURL, err := liveKitHTTPURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	return &liveKitRoomClient{
		service:   livekit.NewRoomServiceProtobufClient(httpURL, &http.Client{}),
		apiKey:    cfg.APIKey,
		apiSecret: cfg.APISecret,
		serverID:  cfg.ServerID,
	}, nil
}

func liveKitHTTPURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	case "http", "https":
	default:
		return "", fmt.Errorf("unsupported LiveKit URL scheme %q", u.Scheme)
	}
	return u.String(), nil
}

type liveKitRoomClient struct {
	service   liveKitRoomService
	apiKey    string
	apiSecret string
	serverID  string
}

type liveKitRoomService interface {
	ListRooms(context.Context, *livekit.ListRoomsRequest) (*livekit.ListRoomsResponse, error)
	ListParticipants(context.Context, *livekit.ListParticipantsRequest) (*livekit.ListParticipantsResponse, error)
	RemoveParticipant(context.Context, *livekit.RoomParticipantIdentity) (*livekit.RemoveParticipantResponse, error)
}

func (c *liveKitRoomClient) ListCallParticipants(ctx context.Context) ([]liveKitParticipantSnapshot, error) {
	roomsResp, err := c.service.ListRooms(c.withVideoGrant(ctx, &lkauth.VideoGrant{RoomList: true}), &livekit.ListRoomsRequest{})
	if err != nil {
		return nil, err
	}

	out := make([]liveKitParticipantSnapshot, 0, len(roomsResp.GetRooms()))
	for _, room := range roomsResp.GetRooms() {
		if room == nil || !liveKitRoomBelongsToInstance(room.GetName(), c.serverID) {
			continue
		}
		legacySpaceID, roomID, callID := ParseLiveKitRoomIdentity(room.GetName())
		if roomID == "" {
			continue
		}
		participantsResp, err := c.service.ListParticipants(
			c.withVideoGrant(ctx, &lkauth.VideoGrant{RoomAdmin: true, Room: room.GetName()}),
			&livekit.ListParticipantsRequest{Room: room.GetName()},
		)
		if err != nil {
			if isLiveKitRoomNotFound(err) {
				out = append(out, liveKitParticipantSnapshot{LegacySpaceID: legacySpaceID, RoomID: roomID, CallID: callID})
				continue
			}
			return nil, err
		}
		userIDs := make([]string, 0, len(participantsResp.GetParticipants()))
		for _, participant := range participantsResp.GetParticipants() {
			if participant.GetIdentity() != "" {
				userIDs = append(userIDs, participant.GetIdentity())
			}
		}
		sort.Strings(userIDs)
		out = append(out, liveKitParticipantSnapshot{LegacySpaceID: legacySpaceID, RoomID: roomID, CallID: callID, UserIDs: userIDs})
	}
	return out, nil
}

func (c *liveKitRoomClient) RemoveCallParticipant(ctx context.Context, legacySpaceID, roomID, callID, userID string) error {
	roomName := liveKitRoomName(c.serverID, legacySpaceID, roomID, callID)
	_, err := c.service.RemoveParticipant(
		c.withVideoGrant(ctx, &lkauth.VideoGrant{RoomAdmin: true, Room: roomName}),
		&livekit.RoomParticipantIdentity{Room: roomName, Identity: userID},
	)
	if err != nil && !isLiveKitRoomNotFound(err) {
		return err
	}
	return nil
}

func isLiveKitRoomNotFound(err error) bool {
	var twerr twirp.Error
	return errors.As(err, &twerr) && twerr.Code() == twirp.NotFound
}

func (c *liveKitRoomClient) withVideoGrant(ctx context.Context, grant *lkauth.VideoGrant) context.Context {
	at := lkauth.NewAccessToken(c.apiKey, c.apiSecret)
	token, err := at.SetVideoGrant(grant).SetValidFor(time.Minute).ToJWT()
	if err != nil {
		return ctx
	}
	headers, _ := twirp.HTTPRequestHeaders(ctx)
	if headers != nil {
		headers = headers.Clone()
	} else {
		headers = make(http.Header)
	}
	headers.Set("Authorization", "Bearer "+token)
	nextCtx, err := twirp.WithHTTPRequestHeaders(ctx, headers)
	if err != nil {
		return ctx
	}
	return nextCtx
}

func liveKitRoomBelongsToInstance(roomName, serverID string) bool {
	roomServerID := ParseLiveKitRoomServerID(roomName)
	if serverID == "" {
		return roomServerID == ""
	}
	return roomServerID == serverID
}

// CallAccessMaterial binds the identity and encryption key for one projected
// call generation. Callers must keep these values together when issuing access.
type CallAccessMaterial struct {
	CallID  string
	E2EEKey string
}

func (s *CallModel) GetAccessMaterial(ctx context.Context, roomID string) (CallAccessMaterial, error) {
	if s.callKeys == nil {
		return CallAccessMaterial{}, fmt.Errorf("call key store is not initialized")
	}
	call, ok := s.callState.Projection().ActiveCall(roomID)
	if !ok || call.CallID == "" || call.E2EEKeyRef == "" {
		return CallAccessMaterial{}, fmt.Errorf("no active voice call for room %s: %w", roomID, ErrNotFound)
	}
	key, err := s.callKeys.GetCallKey(ctx, call.E2EEKeyRef)
	if err != nil {
		return CallAccessMaterial{}, fmt.Errorf("read call E2EE key: %w", err)
	}
	current, ok := s.callState.Projection().ActiveCall(roomID)
	if !ok || current.CallID != call.CallID || current.E2EEKeyRef != call.E2EEKeyRef {
		return CallAccessMaterial{}, fmt.Errorf("active voice call changed while resolving access for room %s: %w", roomID, ErrNotFound)
	}
	return CallAccessMaterial{
		CallID:  call.CallID,
		E2EEKey: key,
	}, nil
}

func (s *CallModel) GetE2EEKey(ctx context.Context, roomID string) (string, error) {
	access, err := s.GetAccessMaterial(ctx, roomID)
	if err != nil {
		return "", err
	}
	return access.E2EEKey, nil
}

func (s *CallModel) RemoveLiveKitParticipant(ctx context.Context, kind RoomKind, roomID, callID, userID string) error {
	if s.livekit == nil {
		return nil
	}
	remover, ok := s.livekit.(liveKitParticipantRemover)
	if !ok {
		return nil
	}
	return remover.RemoveCallParticipant(ctx, LegacySpaceIDForRoomKind(kind), roomID, callID, userID)
}

func (s *CallModel) cleanupQueuedCallKey(ctx context.Context, keyRef string) error {
	if keyRef == "" {
		return nil
	}
	if s.callKeys == nil {
		return fmt.Errorf("call key store is not initialized")
	}
	if err := s.callKeys.ShredCallKey(context.WithoutCancel(ctx), keyRef); err != nil {
		return err
	}
	return nil
}

func (s *CallModel) cleanupEndedCallKey(ctx context.Context, event *corev1.Event) error {
	callID := event.GetVoiceCallEnded().GetCallId()
	if callID == "" {
		return nil
	}
	if s.callKeys == nil {
		return fmt.Errorf("call key store is not initialized")
	}
	keyRef := kms.CallKeyRef(callID)
	// Durable handlers must retain the worker context so shutdown can hand the
	// delivery to another replica promptly. Request-path cleanup deliberately
	// uses cleanupQueuedCallKey's detached context instead.
	if err := s.callKeys.ShredCallKey(ctx, keyRef); err != nil {
		return fmt.Errorf("shred ended call key %s: %w", keyRef, err)
	}
	return nil
}

func (s *CallModel) processKeyCleanupDelivery(ctx context.Context, delivery events.DurableDelivery) error {
	event, err := decodeDurableCoreDelivery(delivery)
	if err != nil {
		return err
	}
	ended := event.GetVoiceCallEnded()
	roomID, ok := evtstream.ParseRoomSubject(delivery.Subject)
	if !ok || ended == nil || ended.GetCallId() == "" || ended.GetRoomId() != roomID {
		return events.TerminateDelivery("invalid ended-call key cleanup request", errors.New("call-ended subject and payload do not match"))
	}
	return s.cleanupEndedCallKey(ctx, event)
}

func (s *CallModel) AppendJoined(ctx context.Context, roomID, userID string, source corev1.CallParticipantEventSource) error {
	return s.appendParticipantTransition(ctx, roomID, userID, true, "", source)
}

func (s *CallModel) AppendLeft(ctx context.Context, roomID, userID string, source corev1.CallParticipantEventSource) error {
	return s.appendParticipantTransition(ctx, roomID, userID, false, "", source)
}

func (s *CallModel) AppendJoinedForCall(ctx context.Context, roomID, userID, expectedCallID string, source corev1.CallParticipantEventSource) error {
	return s.appendParticipantTransition(ctx, roomID, userID, true, expectedCallID, source)
}

func (s *CallModel) AppendLeftForCall(ctx context.Context, roomID, userID, expectedCallID string, source corev1.CallParticipantEventSource) error {
	return s.appendParticipantTransition(ctx, roomID, userID, false, expectedCallID, source)
}

func (s *CallModel) appendParticipantTransition(ctx context.Context, roomID, userID string, joined bool, expectedCallID string, source corev1.CallParticipantEventSource) error {
	aggregate := evtstream.RoomAggregate(roomID)
	filter := aggregate.AllEventsFilter()
	for attempt := 0; attempt < callReconcileMaxRetries; attempt++ {
		snapshot := s.callState.Projection().RoomSnapshot(roomID)
		if expectedCallID != "" && snapshot.Call.CallID != expectedCallID {
			return nil
		}
		if callParticipantTransitionAlreadyApplied(snapshot.Participants, userID, joined) {
			return nil
		}

		entries, endedKeyRef, cleanupKeyRef, err := s.callTransitionBatch(ctx, aggregate, snapshot, roomID, userID, joined, source)
		if err != nil {
			return err
		}
		seqs, err := s.publisher.AppendBatch(ctx, entries)
		if err == nil {
			seq := seqs[len(seqs)-1]
			if endedKeyRef != "" {
				if err := s.cleanupQueuedCallKey(ctx, endedKeyRef); err != nil {
					return fmt.Errorf("shred ended call key: %w", err)
				}
			}
			if err := s.callState.Projector().WaitFor(ctx, events.SubjectPosition(filter, seq)); err != nil {
				return err
			}
			return nil
		}
		if cleanupKeyRef != "" {
			if cleanupErr := s.callKeys.ShredCallKey(context.WithoutCancel(ctx), cleanupKeyRef); cleanupErr != nil {
				s.logger.Warn("failed to clean up unused call key after append conflict", "error", cleanupErr, "key_ref", cleanupKeyRef)
			}
		}
		if !errors.Is(err, events.ErrConflict) {
			return err
		}
		if err := s.waitForLatestRoomTransition(ctx, filter); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return fmt.Errorf("append call participant transition after %d attempts: %w", callReconcileMaxRetries, events.ErrConflict)
}

func (s *CallModel) callTransitionBatch(ctx context.Context, aggregate evtstream.Aggregate, snapshot CallRoomSnapshot, roomID, userID string, joined bool, source corev1.CallParticipantEventSource) ([]evtstream.BatchEntry, string, string, error) {
	if joined {
		callID := snapshot.Call.CallID
		if callID == "" {
			if s.callKeys == nil {
				return nil, "", "", fmt.Errorf("call key store is not initialized")
			}
			callID = NewCallID()
			keyRef, _, err := s.callKeys.CreateCallKey(ctx, callID)
			if err != nil {
				return nil, "", "", fmt.Errorf("create call key: %w", err)
			}
			started := newCallStartedEvent(roomID, userID, callID, keyRef, source)
			joinedEvent := newCallParticipantEvent(roomID, userID, callID, true, source)
			return []evtstream.BatchEntry{
				{
					Subject:       aggregate.SubjectFor(started),
					Event:         started,
					ExpectedSeq:   snapshot.Seq,
					FilterSubject: aggregate.AllEventsFilter(),
					HasOCC:        true,
				},
				{
					Subject: aggregate.SubjectFor(joinedEvent),
					Event:   joinedEvent,
				},
			}, "", keyRef, nil
		}

		joinedEvent := newCallParticipantEvent(roomID, userID, callID, true, source)
		return []evtstream.BatchEntry{{
			Subject:       aggregate.SubjectFor(joinedEvent),
			Event:         joinedEvent,
			ExpectedSeq:   snapshot.Seq,
			FilterSubject: aggregate.AllEventsFilter(),
			HasOCC:        true,
		}}, "", "", nil
	}

	participant, ok := callParticipantByUser(snapshot.Participants, userID)
	if !ok {
		return nil, "", "", nil
	}
	callID := participant.CallID
	if callID == "" {
		callID = snapshot.Call.CallID
	}
	leftEvent := newCallParticipantEvent(roomID, userID, callID, false, source)
	entries := []evtstream.BatchEntry{{
		Subject:       aggregate.SubjectFor(leftEvent),
		Event:         leftEvent,
		ExpectedSeq:   snapshot.Seq,
		FilterSubject: aggregate.AllEventsFilter(),
		HasOCC:        true,
	}}
	var endedKeyRef string
	if len(snapshot.Participants) == 1 && snapshot.Call.CallID == callID {
		ended := newCallEndedEvent(roomID, userID, callID, source)
		entries = append(entries, evtstream.BatchEntry{
			Subject: aggregate.SubjectFor(ended),
			Event:   ended,
		})
		endedKeyRef = snapshot.Call.E2EEKeyRef
	}
	return entries, endedKeyRef, "", nil
}

func (s *CallModel) waitForLatestRoomTransition(ctx context.Context, filter string) error {
	tail, err := s.publisher.LastSubjectPosition(ctx, filter)
	if err != nil {
		return err
	}
	return s.callState.Projector().WaitFor(ctx, tail)
}

func callParticipantTransitionAlreadyApplied(active []CallParticipant, userID string, joined bool) bool {
	for _, participant := range active {
		if participant.UserID == userID {
			return joined
		}
	}
	return !joined
}

func callParticipantByUser(active []CallParticipant, userID string) (CallParticipant, bool) {
	for _, participant := range active {
		if participant.UserID == userID {
			return participant, true
		}
	}
	return CallParticipant{}, false
}

func (s *CallModel) ReconcileRoomParticipants(ctx context.Context, roomID string, observedUserIDs []string) error {
	return s.reconcileRoomParticipants(ctx, roomID, observedUserIDs, s.appendReconciliationEvent)
}

type appendReconciliationEventFunc func(context.Context, string, string, bool) error

func (s *CallModel) reconcileRoomParticipants(ctx context.Context, roomID string, observedUserIDs []string, appendEvent appendReconciliationEventFunc) error {
	observed := make(map[string]struct{}, len(observedUserIDs))
	for _, userID := range observedUserIDs {
		if userID != "" {
			observed[userID] = struct{}{}
		}
	}

	active := s.callState.Projection().Participants(roomID)
	activeByUser := make(map[string]struct{}, len(active))
	for _, participant := range active {
		activeByUser[participant.UserID] = struct{}{}
		if _, ok := observed[participant.UserID]; !ok {
			if err := appendEvent(ctx, roomID, participant.UserID, false); err != nil && !s.reconciliationConflictResolved(roomID, participant.UserID, false, err) {
				return err
			}
		}
	}
	for userID := range observed {
		if _, ok := activeByUser[userID]; !ok {
			if err := appendEvent(ctx, roomID, userID, true); err != nil && !s.reconciliationConflictResolved(roomID, userID, true, err) {
				return err
			}
		}
	}
	return nil
}

func (s *CallModel) reconciliationConflictResolved(roomID, userID string, joined bool, err error) bool {
	return errors.Is(err, events.ErrConflict) && s.reconciliationMismatchResolved(roomID, userID, joined)
}

func (s *CallModel) appendReconciliationEvent(ctx context.Context, roomID, userID string, joined bool) error {
	return s.appendParticipantTransition(ctx, roomID, userID, joined, "", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_RECONCILIATION)
}

func newCallStartedEvent(roomID, userID, callID, keyRef string, source corev1.CallParticipantEventSource) *corev1.Event {
	return newEvent(userID, &corev1.Event{
		Event: &corev1.Event_VoiceCallStarted{
			VoiceCallStarted: &corev1.CallStartedEvent{
				RoomId:     roomID,
				CallId:     callID,
				E2EeKeyRef: keyRef,
				Source:     source,
			},
		},
	})
}

func newCallEndedEvent(roomID, userID, callID string, source corev1.CallParticipantEventSource) *corev1.Event {
	return newEvent(userID, &corev1.Event{
		Event: &corev1.Event_VoiceCallEnded{
			VoiceCallEnded: &corev1.CallEndedEvent{
				RoomId: roomID,
				CallId: callID,
				Source: source,
			},
		},
	})
}

func newCallParticipantEvent(roomID, userID, callID string, joined bool, source corev1.CallParticipantEventSource) *corev1.Event {
	if joined {
		return newEvent(userID, &corev1.Event{
			Event: &corev1.Event_VoiceCallParticipantJoined{
				VoiceCallParticipantJoined: &corev1.CallParticipantJoinedEvent{
					RoomId: roomID,
					Source: source,
					CallId: callID,
				},
			},
		})
	}
	return newEvent(userID, &corev1.Event{
		Event: &corev1.Event_VoiceCallParticipantLeft{
			VoiceCallParticipantLeft: &corev1.CallParticipantLeftEvent{
				RoomId: roomID,
				Source: source,
				CallId: callID,
			},
		},
	})
}

func (s *CallModel) reconciliationMismatchResolved(roomID, userID string, joined bool) bool {
	active := s.callState.Projection().Participants(roomID)
	for _, participant := range active {
		if participant.UserID == userID {
			return joined
		}
	}
	return !joined
}

func (s *CallModel) ReconcileWithLiveKit(ctx context.Context) error {
	return s.reconcileWithLiveKit(ctx, func() (context.Context, context.CancelFunc) {
		return context.WithCancel(ctx)
	})
}

func (s *CallModel) reconcileWithLiveKit(ctx context.Context, cleanupContext func() (context.Context, context.CancelFunc)) error {
	if s.livekit == nil {
		return nil
	}
	snapshots, err := s.livekit.ListCallParticipants(ctx)
	if err != nil {
		counterCtx, counterCancel := cleanupContext()
		failures, recordErr := s.recordLiveKitListFailure(counterCtx)
		counterCancel()
		if recordErr != nil {
			return fmt.Errorf("record LiveKit listing failure: %w", recordErr)
		}
		listErr := &liveKitListFailureError{
			err:                 err,
			consecutiveFailures: failures,
			threshold:           callReconcileListFailureThreshold,
		}
		if failures < callReconcileListFailureThreshold {
			return listErr
		}
		cleanupCtx, cancel := cleanupContext()
		defer cancel()
		listErr.cleanup = s.endActiveCallsAfterLiveKitFailure(cleanupCtx)
		listErr.cleanupAttempted = true
		return listErr
	}
	if err := s.resetLiveKitListFailures(ctx); err != nil {
		return fmt.Errorf("reset LiveKit listing failures: %w", err)
	}
	observedRooms := make(map[string]struct{}, len(snapshots))
	for _, snapshot := range snapshots {
		if !s.liveKitSnapshotMatchesActiveCall(snapshot) {
			if err := s.waitForSnapshotRoomTail(ctx, snapshot.RoomID); err != nil {
				return err
			}
			if s.liveKitSnapshotMatchesActiveCall(snapshot) {
				observedRooms[snapshot.RoomID] = struct{}{}
				if err := s.ReconcileRoomParticipants(ctx, snapshot.RoomID, snapshot.UserIDs); err != nil {
					return err
				}
				continue
			}
			if err := s.cleanupUnmatchedLiveKitSnapshot(ctx, snapshot); err != nil {
				return err
			}
			continue
		}
		observedRooms[snapshot.RoomID] = struct{}{}
		if err := s.ReconcileRoomParticipants(ctx, snapshot.RoomID, snapshot.UserIDs); err != nil {
			return err
		}
	}
	for _, roomID := range s.callState.Projection().ActiveRoomIDs() {
		if _, ok := observedRooms[roomID]; !ok {
			if err := s.ReconcileRoomParticipants(ctx, roomID, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *CallModel) waitForSnapshotRoomTail(ctx context.Context, roomID string) error {
	if roomID == "" || s.publisher == nil || s.callState.Projector() == nil {
		return nil
	}
	tail, err := s.publisher.LastSubjectPosition(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		return fmt.Errorf("read unmatched LiveKit room tail: %w", err)
	}
	if tail.Seq == 0 {
		return nil
	}
	if err := s.callState.Projector().WaitFor(ctx, tail); err != nil {
		return fmt.Errorf("wait for unmatched LiveKit room projection: %w", err)
	}
	return nil
}

func (s *CallModel) cleanupUnmatchedLiveKitSnapshot(ctx context.Context, snapshot liveKitParticipantSnapshot) error {
	if snapshot.RoomID == "" || snapshot.CallID == "" {
		return nil
	}
	remover, ok := s.livekit.(liveKitParticipantRemover)
	if !ok {
		return nil
	}
	if err := s.ensureUnmatchedCallEndedFact(ctx, snapshot); err != nil {
		return err
	}
	keyRef := kms.CallKeyRef(snapshot.CallID)
	for _, userID := range snapshot.UserIDs {
		if userID == "" {
			continue
		}
		if err := remover.RemoveCallParticipant(ctx, snapshot.LegacySpaceID, snapshot.RoomID, snapshot.CallID, userID); err != nil {
			return fmt.Errorf("remove participant from unmatched LiveKit call: %w", err)
		}
	}
	if err := s.cleanupQueuedCallKey(ctx, keyRef); err != nil {
		return fmt.Errorf("clean up unmatched LiveKit call key: %w", err)
	}
	return nil
}

func (s *CallModel) ensureUnmatchedCallEndedFact(ctx context.Context, snapshot liveKitParticipantSnapshot) error {
	agg := evtstream.RoomAggregate(snapshot.RoomID)
	subject := agg.Subject(evtstream.EventCallEnded)
	endedEvents, _, err := s.publisher.SubjectEvents(ctx, subject)
	if err != nil {
		return fmt.Errorf("read unmatched LiveKit call endings: %w", err)
	}
	for _, event := range endedEvents {
		if event.GetVoiceCallEnded().GetCallId() == snapshot.CallID {
			return nil
		}
	}
	ended := newCallEndedEvent(snapshot.RoomID, SystemActorID, snapshot.CallID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_RECONCILIATION)
	if _, err := s.publisher.AppendEventually(ctx, subject, ended); err != nil {
		return fmt.Errorf("record unmatched LiveKit call end: %w", err)
	}
	return nil
}

func (s *CallModel) liveKitSnapshotMatchesActiveCall(snapshot liveKitParticipantSnapshot) bool {
	if snapshot.RoomID == "" {
		return false
	}
	active, ok := s.callState.Projection().ActiveCall(snapshot.RoomID)
	if !ok {
		return false
	}
	if snapshot.CallID == "" {
		return false
	}
	return active.CallID == snapshot.CallID
}

func (s *CallModel) recordLiveKitListFailure(ctx context.Context) (int, error) {
	if s.memoryCacheKV == nil {
		return 0, fmt.Errorf("memory cache KV is not configured")
	}
	// The failure threshold is shared across elected leaders, not process-local.
	// A different replica may successfully reconcile and delete this key between
	// failed passes, which makes the counter reflect consecutive failures at the
	// reconciler role level.
	for attempt := 0; attempt < callReconcileMaxRetries; attempt++ {
		entry, err := s.memoryCacheKV.Get(ctx, liveKitReconcileFailureKey)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
				state := liveKitListFailureState{Count: 1, UpdatedAt: time.Now().UTC()}
				data, err := json.Marshal(state)
				if err != nil {
					return 0, err
				}
				if _, err := s.memoryCacheKV.Create(ctx, liveKitReconcileFailureKey, data); err != nil {
					if jetstreamutil.IsSequenceConflict(err) {
						continue
					}
					return 0, err
				}
				return state.Count, nil
			}
			return 0, err
		}

		var state liveKitListFailureState
		if err := json.Unmarshal(entry.Value(), &state); err != nil {
			return 0, err
		}
		state.Count++
		state.UpdatedAt = time.Now().UTC()
		data, err := json.Marshal(state)
		if err != nil {
			return 0, err
		}
		if _, err := s.memoryCacheKV.Update(ctx, liveKitReconcileFailureKey, data, entry.Revision()); err != nil {
			if jetstreamutil.IsSequenceConflict(err) {
				continue
			}
			return 0, err
		}
		return state.Count, nil
	}
	return 0, fmt.Errorf("LiveKit listing failure counter update failed after %d attempts", callReconcileMaxRetries)
}

func (s *CallModel) resetLiveKitListFailures(ctx context.Context) error {
	if s.memoryCacheKV == nil {
		return nil
	}
	if err := s.memoryCacheKV.Delete(ctx, liveKitReconcileFailureKey); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
		return err
	}
	return nil
}

func (s *CallModel) endActiveCallsAfterLiveKitFailure(ctx context.Context) liveKitFailureCleanupSummary {
	summary := liveKitFailureCleanupSummary{}
	var cleanupErr error
	roomIDs := s.callState.Projection().ActiveRoomIDs()
	summary.activeRooms = len(roomIDs)
	for _, roomID := range roomIDs {
		if err := s.ReconcileRoomParticipants(ctx, roomID, nil); err != nil {
			summary.cleanupErrors++
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("room %s: %w", roomID, err))
			continue
		}
		summary.endedRooms++
	}
	summary.err = cleanupErr
	return summary
}

func (s *CallModel) Run(ctx context.Context) error {
	if s == nil || s.keyCleanupWorker == nil {
		return fmt.Errorf("call-key cleanup worker is not configured")
	}
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return s.keyCleanupWorker.Run(gctx) })
	if s.livekit != nil {
		if s.reconcileLease != nil {
			g.Go(func() error { return s.reconcileLease.Run(gctx, s.runReconciliationLoop) })
		} else {
			g.Go(func() error { return s.runReconciliationLoop(gctx) })
		}
	}
	return g.Wait()
}

func (s *CallModel) runReconciliationLoop(ctx context.Context) error {
	if err := s.reconcileBestEffort(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(callReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.reconcileBestEffort(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *CallModel) reconcileBestEffort(ctx context.Context) error {
	reconcileCtx, cancel := context.WithTimeout(ctx, callReconcileAPITimeout)
	defer cancel()
	if err := s.reconcileWithLiveKit(reconcileCtx, func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(ctx, callReconcileAPITimeout)
	}); err != nil && !strings.Contains(err.Error(), context.Canceled.Error()) {
		var listErr *liveKitListFailureError
		if errors.As(err, &listErr) {
			if !listErr.cleanupAttempted {
				if s.logger != nil {
					s.logger.Warn(
						"LiveKit listing failed; active-call cleanup deferred",
						"error", listErr.err,
						"consecutive_failures", listErr.consecutiveFailures,
						"threshold", listErr.threshold,
					)
				}
				return nil
			}
			if s.logger != nil {
				s.logger.Warn(
					"LiveKit listing failed; threshold reached and ended projected active calls",
					"error", listErr.err,
					"consecutive_failures", listErr.consecutiveFailures,
					"threshold", listErr.threshold,
					"active_rooms", listErr.cleanup.activeRooms,
					"ended_rooms", listErr.cleanup.endedRooms,
					"cleanup_errors", listErr.cleanup.cleanupErrors,
				)
			}
			return nil
		}
		if s.logger != nil {
			s.logger.Warn("LiveKit call-state reconciliation failed", "error", err)
		}
	}
	return nil
}
