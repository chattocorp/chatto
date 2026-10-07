package http_server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/core"
	pubsubv1 "hmans.de/chatto/internal/pb/chatto/core/pubsub/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

var errRealtimeAuthorityChanged = errors.New("realtime authority changed during catch-up")

func activePrivilegedDeadline(deadline time.Time) time.Time {
	if time.Now().Before(deadline) {
		return deadline
	}
	return time.Time{}
}

// requireRealtimeAuthority checks queued session changes and absolute expiry
// immediately before delivery, including initial snapshot/replay delivery.
// Metadata notifications cannot grant authority; validate the stored credential.
func (s *HTTPServer) requireRealtimeAuthority(ctx context.Context, changed <-chan struct{}) error {
	credential, _ := authctx.CredentialForContext(ctx)
	if !credential.PrivilegedModeExpiresAt.IsZero() && !time.Now().Before(credential.PrivilegedModeExpiresAt) {
		return errRealtimeAuthorityChanged
	}
	select {
	case <-changed:
		deadline, err := s.revalidateRealtimeCredential(ctx)
		if err != nil {
			return err
		}
		if !activePrivilegedDeadline(deadline).Equal(credential.PrivilegedModeExpiresAt) {
			return errRealtimeAuthorityChanged
		}
	default:
	}
	return nil
}

// refreshRealtimeAuthorization replaces only the internal event subscription.
// The socket stays open. Subscribe-before-replay recovers handoff events, and
// the caller discards buffered duplicates through the returned boundary.
// A failure requires reconnect recovery; no partially refreshed stream escapes.
func (s *HTTPServer) refreshRealtimeAuthorization(ctx context.Context, userID string, boundary uint64, changed <-chan struct{}, write func(*realtimev1.RealtimeServerFrame) error) (<-chan core.EventEnvelope, context.CancelFunc, uint64, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	success := false
	defer func() {
		if !success {
			cancel()
		}
	}()
	// Bound repeated session changes by the same process-wide concurrency and
	// timeout limits as initial catch-up, without charging stale-cursor retries.
	catchUpCtx, cancelCatchUp := context.WithTimeout(streamCtx, s.realtimeCatchUps.timeout)
	defer cancelCatchUp()
	release, err := s.realtimeCatchUps.acquireAuthority(catchUpCtx)
	if err != nil {
		return nil, func() {}, 0, err
	}
	defer release()
	s.metrics.realtimeCatchUpStarted()
	defer s.metrics.realtimeCatchUpFinished()
	defer func() {
		if errors.Is(catchUpCtx.Err(), context.DeadlineExceeded) {
			s.metrics.realtimeCatchUpTimedOut()
		}
	}()
	events, err := s.core.StreamMyEventsWithOptions(streamCtx, userID, core.StreamMyEventsOptions{TouchPresence: false})
	if err != nil {
		return nil, func() {}, 0, err
	}
	cursor, err := s.core.RealtimeCursorForSequence(userID, boundary)
	if err != nil {
		return nil, func() {}, 0, err
	}
	plan, err := s.core.PlanRealtimeReplay(catchUpCtx, userID, cursor)
	if err != nil {
		return nil, func() {}, 0, err
	}
	if plan.Reset {
		return nil, func() {}, 0, fmt.Errorf("realtime authority handoff exceeded replay limits")
	}
	credential, _ := authctx.CredentialForContext(ctx)
	writeAuthorized := func(frame *realtimev1.RealtimeServerFrame) error {
		if err := catchUpCtx.Err(); err != nil {
			return err
		}
		if err := s.requireRealtimeAuthority(ctx, changed); err != nil {
			return err
		}
		return write(frame)
	}
	for _, event := range plan.Events {
		frame, err := s.realtimeServerFrameForEvent(catchUpCtx, userID, event)
		if errors.Is(err, errRealtimeEventOmitted) {
			continue
		}
		if err != nil {
			return nil, func() {}, 0, err
		}
		if err := writeAuthorized(frame); err != nil {
			return nil, func() {}, boundary, err
		}
		boundary = event.DeliverySeq()
	}
	change := &realtimev1.ViewerPermissionsChangedEvent{PrivilegedModeChanged: new(true)}
	if !credential.PrivilegedModeExpiresAt.IsZero() {
		change.PrivilegedModeExpiresAt = timestamppb.New(credential.PrivilegedModeExpiresAt)
	}
	hint := core.NewPubSubEventEnvelope(&pubsubv1.PubSubEvent{Id: core.NewEventID(), CreatedAt: timestamppb.Now(), Event: &pubsubv1.PubSubEvent_ViewerPermissionsChanged{ViewerPermissionsChanged: change}})
	frame, err := s.realtimeServerFrameForEvent(ctx, userID, hint)
	if err != nil {
		return nil, func() {}, 0, err
	}
	if err := writeAuthorized(frame); err != nil {
		return nil, func() {}, boundary, err
	}
	success = true
	return events, cancel, plan.BoundarySequence, nil
}
