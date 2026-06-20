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

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	"hmans.de/chatto/internal/lease"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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
)

type liveKitParticipantSnapshot struct {
	RoomID  string
	CallID  string
	UserIDs []string
}

type liveKitParticipantLister interface {
	ListCallParticipants(ctx context.Context) ([]liveKitParticipantSnapshot, error)
}

type CallService struct {
	publisher      *events.Publisher
	projection     *CallStateProjection
	projector      *events.Projector
	callKeys       kms.CallKeyStore
	livekit        liveKitParticipantLister
	reconcileLease *lease.Lease
	memoryCacheKV  jetstream.KeyValue
	logger         events.Logger
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

func NewCallService(
	publisher *events.Publisher,
	projection *CallStateProjection,
	projector *events.Projector,
	callKeys kms.CallKeyStore,
	livekit liveKitParticipantLister,
	reconcileLease *lease.Lease,
	memoryCacheKV jetstream.KeyValue,
	logger events.Logger,
) *CallService {
	return &CallService{
		publisher:      publisher,
		projection:     projection,
		projector:      projector,
		callKeys:       callKeys,
		livekit:        livekit,
		reconcileLease: reconcileLease,
		memoryCacheKV:  memoryCacheKV,
		logger:         logger,
	}
}

func (c *ChattoCore) EnableLiveKitCallReconciliation(cfg config.LiveKitConfig) error {
	if c.callService == nil {
		return fmt.Errorf("call service is not initialized")
	}
	lister, err := newLiveKitParticipantLister(cfg)
	if err != nil {
		return err
	}
	c.callService.livekit = lister
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
		_, roomID, callID := ParseLiveKitRoomIdentity(room.GetName())
		if roomID == "" {
			continue
		}
		participantsResp, err := c.service.ListParticipants(
			c.withVideoGrant(ctx, &lkauth.VideoGrant{RoomAdmin: true, Room: room.GetName()}),
			&livekit.ListParticipantsRequest{Room: room.GetName()},
		)
		if err != nil {
			if isLiveKitRoomNotFound(err) {
				out = append(out, liveKitParticipantSnapshot{RoomID: roomID, CallID: callID})
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
		out = append(out, liveKitParticipantSnapshot{RoomID: roomID, CallID: callID, UserIDs: userIDs})
	}
	return out, nil
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

func (s *CallService) GetE2EEKey(ctx context.Context, roomID string) (string, error) {
	if s.callKeys == nil {
		return "", fmt.Errorf("call key store is not initialized")
	}
	call, ok := s.projection.ActiveCall(roomID)
	if !ok || call.CallID == "" || call.E2EEKeyRef == "" {
		return "", fmt.Errorf("no active voice call for room %s", roomID)
	}
	key, err := s.callKeys.GetCallKey(ctx, call.E2EEKeyRef)
	if err != nil {
		return "", fmt.Errorf("read call E2EE key: %w", err)
	}
	return key, nil
}

func (s *CallService) AppendJoined(ctx context.Context, roomID, userID string, source corev1.CallParticipantEventSource) error {
	return s.appendParticipantTransition(ctx, roomID, userID, true, "", source)
}

func (s *CallService) AppendLeft(ctx context.Context, roomID, userID string, source corev1.CallParticipantEventSource) error {
	return s.appendParticipantTransition(ctx, roomID, userID, false, "", source)
}

func (s *CallService) AppendJoinedForCall(ctx context.Context, roomID, userID, expectedCallID string, source corev1.CallParticipantEventSource) error {
	return s.appendParticipantTransition(ctx, roomID, userID, true, expectedCallID, source)
}

func (s *CallService) AppendLeftForCall(ctx context.Context, roomID, userID, expectedCallID string, source corev1.CallParticipantEventSource) error {
	return s.appendParticipantTransition(ctx, roomID, userID, false, expectedCallID, source)
}

func (s *CallService) appendParticipantTransition(ctx context.Context, roomID, userID string, joined bool, expectedCallID string, source corev1.CallParticipantEventSource) error {
	aggregate := events.RoomAggregate(roomID)
	filter := aggregate.AllEventsFilter()
	for attempt := 0; attempt < callReconcileMaxRetries; attempt++ {
		snapshot := s.projection.RoomSnapshot(roomID)
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
				if err := s.callKeys.ShredCallKey(context.WithoutCancel(ctx), endedKeyRef); err != nil {
					return fmt.Errorf("shred ended call key: %w", err)
				}
			}
			if err := s.projector.WaitFor(ctx, events.SubjectPosition(filter, seq)); err != nil {
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

func (s *CallService) callTransitionBatch(ctx context.Context, aggregate events.Aggregate, snapshot CallRoomSnapshot, roomID, userID string, joined bool, source corev1.CallParticipantEventSource) ([]events.BatchEntry, string, string, error) {
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
			return []events.BatchEntry{
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
		return []events.BatchEntry{{
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
	entries := []events.BatchEntry{{
		Subject:       aggregate.SubjectFor(leftEvent),
		Event:         leftEvent,
		ExpectedSeq:   snapshot.Seq,
		FilterSubject: aggregate.AllEventsFilter(),
		HasOCC:        true,
	}}
	var endedKeyRef string
	if len(snapshot.Participants) == 1 && snapshot.Call.CallID == callID {
		ended := newCallEndedEvent(roomID, userID, callID, source)
		entries = append(entries, events.BatchEntry{
			Subject: aggregate.SubjectFor(ended),
			Event:   ended,
		})
		endedKeyRef = snapshot.Call.E2EEKeyRef
	}
	return entries, endedKeyRef, "", nil
}

func (s *CallService) waitForLatestRoomTransition(ctx context.Context, filter string) error {
	tail, err := s.publisher.LastSubjectPosition(ctx, filter)
	if err != nil {
		return err
	}
	return s.projector.WaitFor(ctx, tail)
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

func (s *CallService) ReconcileRoomParticipants(ctx context.Context, roomID string, observedUserIDs []string) error {
	return s.reconcileRoomParticipants(ctx, roomID, observedUserIDs, s.appendReconciliationEvent)
}

type appendReconciliationEventFunc func(context.Context, string, string, bool) error

func (s *CallService) reconcileRoomParticipants(ctx context.Context, roomID string, observedUserIDs []string, appendEvent appendReconciliationEventFunc) error {
	observed := make(map[string]struct{}, len(observedUserIDs))
	for _, userID := range observedUserIDs {
		if userID != "" {
			observed[userID] = struct{}{}
		}
	}

	active := s.projection.Participants(roomID)
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

func (s *CallService) reconciliationConflictResolved(roomID, userID string, joined bool, err error) bool {
	return errors.Is(err, events.ErrConflict) && s.reconciliationMismatchResolved(roomID, userID, joined)
}

func (s *CallService) appendReconciliationEvent(ctx context.Context, roomID, userID string, joined bool) error {
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

func (s *CallService) reconciliationMismatchResolved(roomID, userID string, joined bool) bool {
	active := s.projection.Participants(roomID)
	for _, participant := range active {
		if participant.UserID == userID {
			return joined
		}
	}
	return !joined
}

func (s *CallService) ReconcileWithLiveKit(ctx context.Context) error {
	return s.reconcileWithLiveKit(ctx, func() (context.Context, context.CancelFunc) {
		return context.WithCancel(ctx)
	})
}

func (s *CallService) reconcileWithLiveKit(ctx context.Context, cleanupContext func() (context.Context, context.CancelFunc)) error {
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
			continue
		}
		observedRooms[snapshot.RoomID] = struct{}{}
		if err := s.ReconcileRoomParticipants(ctx, snapshot.RoomID, snapshot.UserIDs); err != nil {
			return err
		}
	}
	for _, roomID := range s.projection.ActiveRoomIDs() {
		if _, ok := observedRooms[roomID]; !ok {
			if err := s.ReconcileRoomParticipants(ctx, roomID, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *CallService) liveKitSnapshotMatchesActiveCall(snapshot liveKitParticipantSnapshot) bool {
	if snapshot.RoomID == "" {
		return false
	}
	active, ok := s.projection.ActiveCall(snapshot.RoomID)
	if !ok {
		return false
	}
	if snapshot.CallID == "" {
		return false
	}
	return active.CallID == snapshot.CallID
}

func (s *CallService) recordLiveKitListFailure(ctx context.Context) (int, error) {
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
					if errors.Is(err, jetstream.ErrKeyExists) {
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
			if errors.Is(err, jetstream.ErrKeyExists) {
				continue
			}
			return 0, err
		}
		return state.Count, nil
	}
	return 0, fmt.Errorf("LiveKit listing failure counter update failed after %d attempts", callReconcileMaxRetries)
}

func (s *CallService) resetLiveKitListFailures(ctx context.Context) error {
	if s.memoryCacheKV == nil {
		return nil
	}
	if err := s.memoryCacheKV.Delete(ctx, liveKitReconcileFailureKey); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
		return err
	}
	return nil
}

func (s *CallService) endActiveCallsAfterLiveKitFailure(ctx context.Context) liveKitFailureCleanupSummary {
	summary := liveKitFailureCleanupSummary{}
	var cleanupErr error
	roomIDs := s.projection.ActiveRoomIDs()
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

func (s *CallService) Run(ctx context.Context) error {
	if s.livekit == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	if s.reconcileLease != nil {
		return s.reconcileLease.Run(ctx, s.runReconciliationLoop)
	}
	return s.runReconciliationLoop(ctx)
}

func (s *CallService) runReconciliationLoop(ctx context.Context) error {
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

func (s *CallService) reconcileBestEffort(ctx context.Context) error {
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
