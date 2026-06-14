package core

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	callE2EEKeyPrefix       = "voice.e2ee."
	callReconcileInterval   = 30 * time.Second
	callReconcileAPITimeout = 10 * time.Second
	callReconcileMaxRetries = 5
)

type liveKitParticipantSnapshot struct {
	RoomID  string
	UserIDs []string
}

type liveKitParticipantLister interface {
	ListCallParticipants(ctx context.Context) ([]liveKitParticipantSnapshot, error)
}

type CallService struct {
	publisher     *events.Publisher
	projection    *CallStateProjection
	projector     *events.Projector
	memoryCacheKV jetstream.KeyValue
	livekit       liveKitParticipantLister
	logger        events.Logger
}

func NewCallService(
	publisher *events.Publisher,
	projection *CallStateProjection,
	projector *events.Projector,
	memoryCacheKV jetstream.KeyValue,
	livekit liveKitParticipantLister,
	logger events.Logger,
) *CallService {
	return &CallService{
		publisher:     publisher,
		projection:    projection,
		projector:     projector,
		memoryCacheKV: memoryCacheKV,
		livekit:       livekit,
		logger:        logger,
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
	service   livekit.RoomService
	apiKey    string
	apiSecret string
	serverID  string
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
		_, roomID := ParseLiveKitRoomName(room.GetName())
		if roomID == "" {
			continue
		}
		participantsResp, err := c.service.ListParticipants(
			c.withVideoGrant(ctx, &lkauth.VideoGrant{RoomAdmin: true, Room: room.GetName()}),
			&livekit.ListParticipantsRequest{Room: room.GetName()},
		)
		if err != nil {
			return nil, err
		}
		userIDs := make([]string, 0, len(participantsResp.GetParticipants()))
		for _, participant := range participantsResp.GetParticipants() {
			if participant.GetIdentity() != "" {
				userIDs = append(userIDs, participant.GetIdentity())
			}
		}
		sort.Strings(userIDs)
		out = append(out, liveKitParticipantSnapshot{RoomID: roomID, UserIDs: userIDs})
	}
	return out, nil
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
	key := callE2EEKeyPrefix + roomID
	entry, err := s.memoryCacheKV.Get(ctx, key)
	if err == nil {
		return string(entry.Value()), nil
	}
	if !errors.Is(err, jetstream.ErrKeyNotFound) {
		return "", fmt.Errorf("read call E2EE key: %w", err)
	}

	generated, err := generateVoiceCallE2EEKey()
	if err != nil {
		return "", err
	}
	if _, err := s.memoryCacheKV.Create(ctx, key, []byte(generated)); err == nil {
		return generated, nil
	} else if !errors.Is(err, jetstream.ErrKeyExists) {
		return "", fmt.Errorf("write call E2EE key: %w", err)
	}

	entry, err = s.memoryCacheKV.Get(ctx, key)
	if err != nil {
		return "", fmt.Errorf("read raced call E2EE key: %w", err)
	}
	return string(entry.Value()), nil
}

func generateVoiceCallE2EEKey() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate call E2EE key: %w", err)
	}
	return base64.RawStdEncoding.EncodeToString(raw[:]), nil
}

func (s *CallService) AppendJoined(ctx context.Context, roomID, userID string, source corev1.CallParticipantEventSource) error {
	return s.appendParticipantTransition(ctx, roomID, userID, true, source)
}

func (s *CallService) AppendLeft(ctx context.Context, roomID, userID string, source corev1.CallParticipantEventSource) error {
	return s.appendParticipantTransition(ctx, roomID, userID, false, source)
}

func (s *CallService) appendParticipantTransition(ctx context.Context, roomID, userID string, joined bool, source corev1.CallParticipantEventSource) error {
	aggregate := events.RoomAggregate(roomID)
	filter := aggregate.AllEventsFilter()
	for attempt := 0; attempt < callReconcileMaxRetries; attempt++ {
		snapshot := s.projection.RoomSnapshot(roomID)
		if callParticipantTransitionAlreadyApplied(snapshot.Participants, userID, joined) {
			return nil
		}

		event := newCallParticipantEvent(roomID, userID, joined, source)
		subject := aggregate.SubjectFor(event)
		seq, err := s.publisher.AppendAtFilter(ctx, subject, event, filter, snapshot.Seq)
		if err == nil {
			return s.projector.WaitFor(ctx, events.SubjectPosition(filter, seq))
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
	return s.appendParticipantTransition(ctx, roomID, userID, joined, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_RECONCILIATION)
}

func newCallParticipantEvent(roomID, userID string, joined bool, source corev1.CallParticipantEventSource) *corev1.Event {
	if joined {
		return newEvent(userID, &corev1.Event{
			Event: &corev1.Event_VoiceCallParticipantJoined{
				VoiceCallParticipantJoined: &corev1.CallParticipantJoinedEvent{
					RoomId: roomID,
					Source: source,
				},
			},
		})
	}
	return newEvent(userID, &corev1.Event{
		Event: &corev1.Event_VoiceCallParticipantLeft{
			VoiceCallParticipantLeft: &corev1.CallParticipantLeftEvent{
				RoomId: roomID,
				Source: source,
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
	if s.livekit == nil {
		return nil
	}
	snapshots, err := s.livekit.ListCallParticipants(ctx)
	if err != nil {
		return err
	}
	observedRooms := make(map[string]struct{}, len(snapshots))
	for _, snapshot := range snapshots {
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

func (s *CallService) Run(ctx context.Context) error {
	if s.livekit == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	s.reconcileBestEffort(ctx)

	ticker := time.NewTicker(callReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.reconcileBestEffort(ctx)
		}
	}
}

func (s *CallService) reconcileBestEffort(ctx context.Context) {
	reconcileCtx, cancel := context.WithTimeout(ctx, callReconcileAPITimeout)
	defer cancel()
	if err := s.ReconcileWithLiveKit(reconcileCtx); err != nil && s.logger != nil && !strings.Contains(err.Error(), context.Canceled.Error()) {
		s.logger.Warn("LiveKit call-state reconciliation failed", "error", err)
	}
}
