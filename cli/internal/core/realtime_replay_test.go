package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/internal/publiccursor"
)

func TestWaitForRealtimeCursorBringsAnotherReplicaToTheResourceBoundary(t *testing.T) {
	primary, nc := setupTestCore(t)
	ctx := testContext(t)
	viewer, err := primary.CreateUser(ctx, SystemActorID, "cursor-replica-viewer", "Cursor Replica Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := primary.UpdateUserBio(ctx, viewer.GetId(), "visible at boundary E"); err != nil {
		t.Fatalf("UpdateUserBio: %v", err)
	}
	plan, err := primary.PlanRealtimeReplay(ctx, viewer.GetId(), "")
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}

	replica, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore replica: %v", err)
	}
	waited := make(chan error, 1)
	go func() {
		waited <- replica.WaitForRealtimeCursor(ctx, viewer.GetId(), plan.BoundaryCursor)
	}()
	select {
	case err := <-waited:
		t.Fatalf("WaitForRealtimeCursor returned before replica projections started: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	startCoreServices(t, replica)
	if err := <-waited; err != nil {
		t.Fatalf("WaitForRealtimeCursor: %v", err)
	}
	user, err := replica.GetUser(ctx, viewer.GetId())
	if err != nil {
		t.Fatalf("GetUser on serving replica: %v", err)
	}
	if user.GetBio() != "visible at boundary E" {
		t.Fatalf("replica bio = %q, want state through E", user.GetBio())
	}
}

func TestRealtimeCursorRoundTrip(t *testing.T) {
	chatto, _ := setupTestCore(t)
	identity := "evt-incarnation-v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	userID := "cursor-viewer"
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	cursor, err := chatto.encodeRealtimeCursorAt(userID, identity, 42, now)
	if err != nil {
		t.Fatalf("encodeRealtimeCursor: %v", err)
	}
	if len(cursor) != 99 {
		t.Fatalf("cursor length = %d, want 99 bytes", len(cursor))
	}
	decoded, err := chatto.parseRealtimeCursorAt(userID, cursor, now)
	if err != nil {
		t.Fatalf("decodeRealtimeCursor: %v", err)
	}
	if decoded.Version != realtimeCursorVersion || decoded.StreamIdentity != realtimeStreamIdentity(identity) || decoded.Sequence != 42 || decoded.IssuedAt != now.Unix() {
		t.Fatalf("decoded cursor = %+v", decoded)
	}
	sequence, err := chatto.resolveRealtimeCursorAt(userID, cursor, identity, 0, 100, now)
	if err != nil || sequence != 42 {
		t.Fatalf("resolved cursor = %d, %v; want 42", sequence, err)
	}
	if _, err := chatto.parseRealtimeCursorAt(userID, "not-a-cursor", now); !errors.Is(err, ErrRealtimeCursorInvalid) {
		t.Fatalf("invalid cursor error = %v, want ErrRealtimeCursorInvalid", err)
	}
	if _, err := chatto.parseRealtimeCursorAt("another-user", cursor, now); !errors.Is(err, ErrRealtimeCursorInvalid) {
		t.Fatalf("cross-user cursor error = %v, want ErrRealtimeCursorInvalid", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatalf("decode cursor envelope: %v", err)
	}
	for _, secret := range []string{identity, userID, `"s":42`, `"sequence":42`} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("cursor envelope exposes internal payload %q", secret)
		}
	}
	second, err := chatto.encodeRealtimeCursorAt(userID, identity, 42, now)
	if err != nil {
		t.Fatalf("encode second cursor: %v", err)
	}
	if second == cursor {
		t.Fatal("sealed cursor reused a nonce")
	}
	raw[len(raw)-1] ^= 1
	tampered := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := chatto.parseRealtimeCursorAt(userID, tampered, now); !errors.Is(err, ErrRealtimeCursorInvalid) {
		t.Fatalf("tampered cursor error = %v, want ErrRealtimeCursorInvalid", err)
	}
}

func TestRealtimeCursorRejectsMalformedSealedPayload(t *testing.T) {
	chatto := &ChattoCore{config: config.CoreConfig{SecretKey: "cursor-test-secret"}}
	now := time.Unix(1_788_610_000, 0)
	payload := make([]byte, realtimeCursorPayloadSize)
	payload[0] = realtimeCursorVersion
	binary.BigEndian.PutUint64(payload[9:17], uint64(now.Unix()))
	for _, test := range []struct {
		name   string
		change func([]byte) []byte
	}{
		{"empty", func(p []byte) []byte { return nil }},
		{"truncated", func(p []byte) []byte { return p[:len(p)-1] }},
		{"trailing bytes", func(p []byte) []byte { return append(p, 0) }},
		{"wrong version", func(p []byte) []byte { p[0]++; return p }},
		{"negative time", func(p []byte) []byte { binary.BigEndian.PutUint64(p[9:17], ^uint64(0)); return p }},
		{"overflow time", func(p []byte) []byte { binary.BigEndian.PutUint64(p[9:17], uint64(1)<<63-1); return p }},
		{"old JSON", func(p []byte) []byte { return []byte(`{"s":42,"v":4}`) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			malformed := test.change(bytes.Clone(payload))
			cursor, err := publiccursor.Seal(chatto.config.SecretKey, realtimeCursorPurpose, realtimeCursorScope+"\x00viewer", malformed)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := chatto.parseRealtimeCursorAt("viewer", cursor, now); !errors.Is(err, ErrRealtimeCursorInvalid) {
				t.Fatalf("parse error = %v, want invalid cursor", err)
			}
		})
	}
}

func TestRealtimeCursorPreservesFullSequenceRange(t *testing.T) {
	chatto := &ChattoCore{config: config.CoreConfig{SecretKey: "cursor-test-secret"}}
	now := time.Now()
	for _, sequence := range []uint64{0, 1, 1<<53 + 1, ^uint64(0)} {
		cursor, err := chatto.encodeRealtimeCursorAt("viewer", "evt-incarnation-v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", sequence, now)
		if err != nil {
			t.Fatal(err)
		}
		claims, err := chatto.parseRealtimeCursorAt("viewer", cursor, now)
		if err != nil || claims.Sequence != sequence || len(cursor) != 99 {
			t.Fatalf("sequence %d: round trip = %d, error %v, size %d", sequence, claims.Sequence, err, len(cursor))
		}
	}
}

func TestWaitForRealtimeCursorWaitsOnlyForRequestedContentBoundary(t *testing.T) {
	primary, nc := setupTestCore(t)
	ctx := testContext(t)
	viewer, err := primary.CreateUser(ctx, SystemActorID, "exact-cursor-viewer", "Exact Cursor Viewer", "password123")
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := primary.PlanRealtimeReplay(ctx, viewer.GetId(), "")
	if err != nil {
		t.Fatal(err)
	}
	replica, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// A cold replica must respect the caller's shorter deadline.
	waitCtx, cancelWait := context.WithTimeout(ctx, 25*time.Millisecond)
	defer cancelWait()
	if err := replica.WaitForRealtimeCursor(waitCtx, viewer.GetId(), boundary.BoundaryCursor); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cold replica wait = %v, want deadline exceeded", err)
	}

	// Start only the content view. In particular, UserAuth still has not applied
	// the account-created fact. It must not delay this content resource barrier.
	runCtx, cancelRun := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- replica.contentView.projector.Run(runCtx) }()
	t.Cleanup(func() {
		cancelRun()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("content projector did not stop")
		}
	})
	if err := replica.WaitForRealtimeCursor(ctx, viewer.GetId(), boundary.BoundaryCursor); err != nil {
		t.Fatalf("ready content view waited for unrelated projections: %v", err)
	}

	// Test-only apply pause: advance the shared stream on the other replica
	// while this replica remains at E. Waiting for E must not chase E+1.
	err = replica.contentView.projector.WithReadBarrier(func(sequence uint64) error {
		if sequence < boundary.BoundarySequence {
			t.Fatalf("view sequence %d is before requested E", sequence)
		}
		if _, err := primary.UpdateUserBio(ctx, viewer.GetId(), "after requested boundary"); err != nil {
			return err
		}
		bounded, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		defer cancel()
		return replica.WaitForRealtimeCursor(bounded, viewer.GetId(), boundary.BoundaryCursor)
	})
	if err != nil {
		t.Fatalf("wait chased a later stream boundary: %v", err)
	}
}

func TestRealtimeCursorCoordinatesAndReplayBudgetAreIndependent(t *testing.T) {
	chatto, _ := setupTestCore(t)
	identity := "evt-incarnation-v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	now := time.Now()
	cursor, err := chatto.encodeRealtimeCursorAt("viewer", identity, 42, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, identity string
		first, last    uint64
		want           error
	}{
		{"beyond replay work cap", identity, 1, 42 + realtimeReplayMaxSequenceSpan + 1, nil},
		{"wrong incarnation", "evt-incarnation-v1:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, 100, ErrRealtimeCursorInvalid},
		{"future sequence", identity, 1, 41, ErrRealtimeCursorInvalid},
		{"retention gap", identity, 44, 100, ErrRealtimeCursorExpired},
		{"immediately before retention", identity, 43, 100, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sequence, err := chatto.resolveRealtimeCursorAt("viewer", cursor, tt.identity, tt.first, tt.last, now)
			if !errors.Is(err, tt.want) || (err == nil && sequence != 42) {
				t.Fatalf("resolve = %d, %v; want 42 or %v", sequence, err, tt.want)
			}
		})
	}
}

func TestRealtimeCursorOutsideReplayWorkBudgetStillBoundsRPC(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	const viewerID = "busy-stream-viewer"
	before, err := chatto.PlanRealtimeReplay(ctx, viewerID, "")
	if err != nil {
		t.Fatal(err)
	}
	// These internal configuration facts do not generate public events. This
	// gap must hit the sequence-work cap, not the delivered-event cap.
	for index := uint64(0); index <= realtimeReplayMaxSequenceSpan; index++ {
		event := &evtv1.Event{
			Id:    fmt.Sprintf("budget-event-%d", index),
			Event: &evtv1.Event_ServerWelcomeMessageChanged{ServerWelcomeMessageChanged: &evtv1.ServerWelcomeMessageChangedEvent{}},
		}
		if _, err := chatto.EventPublisher.AppendEventually(ctx, evtstream.ConfigAggregate().SubjectFor(event), event); err != nil {
			t.Fatal(err)
		}
	}
	if err := chatto.WaitForRealtimeCursor(ctx, viewerID, before.BoundaryCursor); err != nil {
		t.Fatalf("valid minimum cursor outside replay window failed: %v", err)
	}
	replay, err := chatto.PlanRealtimeReplay(ctx, viewerID, before.BoundaryCursor)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Reset || !replay.HadSequenceGap || len(replay.Events) != 0 {
		t.Fatalf("oversized replay did not select a complete fallback: %v", replay)
	}
}

func TestRealtimeCursorExpiresAfterPublicLifetime(t *testing.T) {
	chatto, _ := setupTestCore(t)
	identity := "evt-incarnation-v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	userID := "cursor-viewer"
	issuedAt := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	cursor, err := chatto.encodeRealtimeCursorAt(userID, identity, 42, issuedAt)
	if err != nil {
		t.Fatalf("encodeRealtimeCursorAt: %v", err)
	}
	if _, err := chatto.parseRealtimeCursorAt(userID, cursor, issuedAt.Add(realtimeCursorLifetime-time.Second)); err != nil {
		t.Fatalf("cursor expired before its lifetime: %v", err)
	}
	if _, err := chatto.parseRealtimeCursorAt(userID, cursor, issuedAt.Add(realtimeCursorLifetime)); !errors.Is(err, ErrRealtimeCursorExpired) {
		t.Fatalf("expired cursor error = %v, want ErrRealtimeCursorExpired", err)
	}
}

func TestRealtimeCursorRejectsImplausibleFutureIssueTime(t *testing.T) {
	chatto, _ := setupTestCore(t)
	identity := "evt-incarnation-v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	userID := "cursor-viewer"
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	cursor, err := chatto.encodeRealtimeCursorAt(userID, identity, 42, now.Add(realtimeCursorFutureSkew+time.Second))
	if err != nil {
		t.Fatalf("encodeRealtimeCursorAt: %v", err)
	}
	if _, err := chatto.parseRealtimeCursorAt(userID, cursor, now); !errors.Is(err, ErrRealtimeCursorInvalid) {
		t.Fatalf("future cursor error = %v, want ErrRealtimeCursorInvalid", err)
	}
}

func TestRealtimeCursorAtCurrentBoundary(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	const userID = "cursor-boundary-viewer"

	plan, err := chatto.PlanRealtimeReplay(ctx, userID, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	current, err := chatto.RealtimeCursorAtCurrentBoundary(ctx, userID, plan.BoundaryCursor)
	if err != nil {
		t.Fatalf("RealtimeCursorAtCurrentBoundary: %v", err)
	}
	if !current {
		t.Fatal("boundary cursor reported stale")
	}

	if _, err := chatto.CreateUser(ctx, SystemActorID, "cursor-boundary-new-user", "Cursor Boundary User", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	current, err = chatto.RealtimeCursorAtCurrentBoundary(ctx, userID, plan.BoundaryCursor)
	if err != nil {
		t.Fatalf("RealtimeCursorAtCurrentBoundary after event: %v", err)
	}
	if current {
		t.Fatal("cursor before a durable event reported current")
	}
	replay, err := chatto.PlanRealtimeReplay(ctx, userID, plan.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay after boundary advanced: %v", err)
	}
	if !replay.HadSequenceGap {
		t.Fatal("replay did not report the gap that appeared after boundary classification")
	}

	for name, cursor := range map[string]string{
		"empty":      "",
		"invalid":    "not-a-cursor",
		"cross-user": plan.BoundaryCursor,
	} {
		viewer := userID
		if name == "cross-user" {
			viewer = "different-viewer"
		}
		current, err := chatto.RealtimeCursorAtCurrentBoundary(ctx, viewer, cursor)
		if err != nil {
			t.Fatalf("%s cursor classification: %v", name, err)
		}
		if current {
			t.Fatalf("%s cursor reported current", name)
		}
	}
}

func TestPlanRealtimeReplayReportsRetentionResetGap(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	const userID = "cursor-retention-viewer"

	before, err := chatto.PlanRealtimeReplay(ctx, userID, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	for index := 0; index < 2; index++ {
		if _, err := chatto.CreateUser(ctx, SystemActorID, fmt.Sprintf("cursor-retention-%d", index), "Cursor Retention", "password123"); err != nil {
			t.Fatalf("CreateUser %d: %v", index, err)
		}
	}
	info, err := chatto.storage.serverEvtStream.Info(ctx)
	if err != nil {
		t.Fatalf("read EVT info: %v", err)
	}
	if err := chatto.storage.serverEvtStream.Purge(ctx, jetstream.WithPurgeSequence(info.State.LastSeq)); err != nil {
		t.Fatalf("purge retained EVT prefix: %v", err)
	}

	replay, err := chatto.PlanRealtimeReplay(ctx, userID, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay after retention truncation: %v", err)
	}
	if !replay.Reset || !replay.HadSequenceGap {
		t.Fatalf("retention replay = %+v, want reset with reported sequence gap", replay)
	}
}

func TestPlanRealtimeReplayResetsForExpiredPublicCursor(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	identity, err := evtstream.Identity(chatto.storage.serverEvtStream)
	if err != nil {
		t.Fatalf("StreamIdentity: %v", err)
	}
	cursor, err := chatto.encodeRealtimeCursorAt("cursor-viewer", identity, 0, time.Now().Add(-realtimeCursorLifetime-time.Second))
	if err != nil {
		t.Fatalf("encodeRealtimeCursorAt: %v", err)
	}
	plan, err := chatto.PlanRealtimeReplay(ctx, "cursor-viewer", cursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if !plan.Reset || len(plan.Events) != 0 || plan.BoundaryCursor == "" {
		t.Fatalf("expired cursor plan = %+v, want snapshot fallback", plan)
	}
}

func TestPlanRealtimeReplayReplaysAuthorizedReactionGap(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, messageEventID := setupReactionTest(t, chatto, ctx)

	before, err := chatto.PlanRealtimeReplay(ctx, user.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	if len(before.Events) != 0 || before.BoundaryCursor == "" {
		t.Fatalf("initial replay plan = %+v", before)
	}

	if added, err := chatto.ReactionModel().addReaction(ctx, KindChannel, room.Id, messageEventID, "thumbsup", user.Id); err != nil || !added {
		t.Fatalf("AddReaction = %v, %v", added, err)
	}
	if removed, err := chatto.ReactionModel().removeReaction(ctx, KindChannel, room.Id, messageEventID, "thumbsup", user.Id); err != nil || !removed {
		t.Fatalf("RemoveReaction = %v, %v", removed, err)
	}

	replay, err := chatto.PlanRealtimeReplay(ctx, user.Id, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if len(replay.Events) != 2 {
		t.Fatalf("replayed events = %d, want 2", len(replay.Events))
	}
	if got := replay.Events[0].EVTEvent().GetReactionAdded(); got == nil || got.GetMessageEventId() != messageEventID {
		t.Fatalf("first replay event = %T, want reaction_added for %q", replay.Events[0].EVTEvent().GetEvent(), messageEventID)
	}
	if got := replay.Events[1].EVTEvent().GetReactionRemoved(); got == nil || got.GetMessageEventId() != messageEventID {
		t.Fatalf("second replay event = %T, want reaction_removed for %q", replay.Events[1].EVTEvent().GetEvent(), messageEventID)
	}
	if replay.Events[0].DeliverySeq() >= replay.Events[1].DeliverySeq() || replay.Events[1].DeliverySeq() > replay.BoundarySequence {
		t.Fatalf("replay sequences = %d, %d through %d", replay.Events[0].DeliverySeq(), replay.Events[1].DeliverySeq(), replay.BoundarySequence)
	}

	outsider, err := chatto.CreateUser(ctx, SystemActorID, "replay-outsider", "Replay Outsider", "password123")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	outsiderReplay, err := chatto.PlanRealtimeReplay(ctx, outsider.Id, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("outsider PlanRealtimeReplay: %v", err)
	}
	if !outsiderReplay.Reset || outsiderReplay.BoundaryCursor == "" {
		t.Fatalf("cross-user cursor plan = %+v, want snapshot fallback", outsiderReplay)
	}
	for _, event := range outsiderReplay.Events {
		if event.EVTEvent().GetReactionAdded() != nil || event.EVTEvent().GetReactionRemoved() != nil {
			t.Fatalf("outsider replayed unauthorized reaction event: %T", event.EVTEvent().GetEvent())
		}
	}
}

func TestPlanRealtimeReplayReplaysAuthorizedAssetLifecycleGap(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, _ := setupReactionTest(t, chatto, ctx)
	attachment, err := chatto.UploadAttachment(ctx, user.Id, room.Id, "replay-asset.txt", "text/plain", bytes.NewReader([]byte("asset")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	message, err := chatto.PostMessage(ctx, KindChannel, room.Id, user.Id, "asset lifecycle", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	before, err := chatto.PlanRealtimeReplay(ctx, user.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	if err := chatto.RecordAssetProcessingStarted(ctx, SystemActorID, room.Id, message.Id, attachment.Id); err != nil {
		t.Fatalf("RecordAssetProcessingStarted: %v", err)
	}
	if err := chatto.RecordAssetProcessingFailed(ctx, SystemActorID, room.Id, message.Id, attachment.Id, evtv1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED); err != nil {
		t.Fatalf("RecordAssetProcessingFailed: %v", err)
	}
	if err := chatto.RecordAssetDeleted(ctx, SystemActorID, room.Id, attachment.Id); err != nil {
		t.Fatalf("RecordAssetDeleted: %v", err)
	}

	replay, err := chatto.PlanRealtimeReplay(ctx, user.Id, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if replay.Reset || len(replay.Events) != 3 {
		t.Fatalf("asset replay plan = %+v, want three incremental events", replay)
	}
	if replay.Events[0].EVTEvent().GetAssetProcessingStarted() == nil || replay.Events[1].EVTEvent().GetAssetProcessingFailed() == nil || replay.Events[2].EVTEvent().GetAssetDeleted() == nil {
		t.Fatalf("asset replay events = %T, %T, %T", replay.Events[0].EVTEvent().GetEvent(), replay.Events[1].EVTEvent().GetEvent(), replay.Events[2].EVTEvent().GetEvent())
	}
	for i, event := range replay.Events {
		if event.DeliverySeq() == 0 || event.DeliverySeq() > replay.BoundarySequence {
			t.Fatalf("asset replay event %d sequence = %d through %d", i, event.DeliverySeq(), replay.BoundarySequence)
		}
	}

	outsider, err := chatto.CreateUser(ctx, SystemActorID, "asset-replay-outsider", "Asset Replay Outsider", "password123")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	outsiderReplay, err := chatto.PlanRealtimeReplay(ctx, outsider.Id, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("outsider PlanRealtimeReplay: %v", err)
	}
	for _, event := range outsiderReplay.Events {
		if isAssetLifecycleEvent(event.EVTEvent()) {
			t.Fatalf("outsider replayed unauthorized asset event: %T", event.EVTEvent().GetEvent())
		}
	}
}

func TestPlanRealtimeReplayReplaysLegacyRoomScopedAssetLifecycleGap(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, _ := setupReactionTest(t, chatto, ctx)
	attachment, err := chatto.UploadAttachment(ctx, user.Id, room.Id, "legacy-replay.txt", "text/plain", bytes.NewReader([]byte("asset")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	message, err := chatto.PostMessage(ctx, KindChannel, room.Id, user.Id, "legacy asset lifecycle", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	before, err := chatto.PlanRealtimeReplay(ctx, user.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	legacy := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_AssetProcessingStarted{
		AssetProcessingStarted: &evtv1.AssetProcessingStartedEvent{AssetId: attachment.Id, MessageEventId: message.Id},
	}})
	legacySubject := evtstream.RoomAggregate(room.Id).SubjectFor(legacy)
	if _, err := chatto.EventPublisher.AppendEventually(ctx, legacySubject, legacy); err != nil {
		t.Fatalf("append legacy asset event: %v", err)
	}

	replay, err := chatto.PlanRealtimeReplay(ctx, user.Id, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if replay.Reset || len(replay.Events) != 1 || replay.Events[0].EVTEvent().GetAssetProcessingStarted() == nil {
		t.Fatalf("legacy asset replay plan = %+v, want one processing-started event", replay)
	}
}

func TestAssetEventTimelineTargetResolvesDeletedProcessedDerivative(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, _ := setupReactionTest(t, chatto, ctx)
	original, err := chatto.UploadAttachment(ctx, user.Id, room.Id, "original.mp4", "video/mp4", bytes.NewReader([]byte("original")))
	if err != nil {
		t.Fatalf("UploadAttachment original: %v", err)
	}
	thumbnail, err := chatto.UploadDerivativeAttachment(ctx, original.Id, evtv1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL, room.Id, "thumbnail.bin", "application/octet-stream", bytes.NewReader([]byte("thumbnail")))
	if err != nil {
		t.Fatalf("UploadDerivativeAttachment: %v", err)
	}
	message, err := chatto.PostMessage(ctx, KindChannel, room.Id, user.Id, "processed video", []string{original.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := chatto.assetModel.RecordAssetProcessed(ctx, SystemActorID, room.Id, message.Id, original.Id, 1000, 640, 360, thumbnail, nil); err != nil {
		t.Fatalf("RecordAssetProcessed: %v", err)
	}
	if err := chatto.RecordAssetDeleted(ctx, SystemActorID, room.Id, thumbnail.Id); err != nil {
		t.Fatalf("RecordAssetDeleted thumbnail: %v", err)
	}

	roomID, messageEventID, ok := chatto.AssetEventTimelineTarget(&evtv1.Event{
		Event: &evtv1.Event_AssetDeleted{AssetDeleted: &evtv1.AssetDeletedEvent{AssetId: thumbnail.Id}},
	})
	if !ok || roomID != room.Id || messageEventID != message.Id {
		t.Fatalf("AssetEventTimelineTarget = %q, %q, %v; want %q, %q, true", roomID, messageEventID, ok, room.Id, message.Id)
	}
}

func TestPlanRealtimeReplayResetsForDifferentStreamIncarnation(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	cursor, err := chatto.encodeRealtimeCursor("user", "evt-incarnation-v1:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 0)
	if err != nil {
		t.Fatalf("encodeRealtimeCursor: %v", err)
	}
	plan, err := chatto.PlanRealtimeReplay(ctx, "user", cursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if !plan.Reset || len(plan.Events) != 0 || plan.BoundaryCursor == "" {
		t.Fatalf("PlanRealtimeReplay plan = %+v, want snapshot fallback", plan)
	}
}

func TestPlanRealtimeReplayResetsAfterUserKeyShredding(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	viewer, err := chatto.CreateUser(ctx, SystemActorID, "replay-shred-viewer", "Replay Shred Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	author, err := chatto.CreateUser(ctx, SystemActorID, "replay-shred-author", "Replay Shred Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	room, err := chatto.CreateRoom(ctx, viewer.Id, KindChannel, "", "replay-shred-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{viewer.Id, author.Id} {
		if _, err := chatto.JoinRoom(ctx, viewer.Id, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %q: %v", userID, err)
		}
	}
	if _, err := chatto.PostMessage(ctx, KindChannel, room.Id, author.Id, "must be purged", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	boundary, err := chatto.PlanRealtimeReplay(ctx, viewer.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	if err := chatto.DeleteUser(ctx, viewer.Id, author.Id); err != nil {
		t.Fatalf("DeleteUser author: %v", err)
	}

	plan, err := chatto.PlanRealtimeReplay(ctx, viewer.Id, boundary.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if !plan.Reset || len(plan.Events) != 0 {
		t.Fatalf("PlanRealtimeReplay plan = %+v, want snapshot fallback", plan)
	}
}

func TestPlanRealtimeReplayDeliversViewerLeaveAfterVisibilityCloses(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	viewer, room, _ := setupReactionTest(t, chatto, ctx)

	boundary, err := chatto.PlanRealtimeReplay(ctx, viewer.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	if err := chatto.LeaveRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

	plan, err := chatto.PlanRealtimeReplay(ctx, viewer.Id, boundary.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if plan.Reset || len(plan.Events) != 1 || plan.Events[0].EVTEvent().GetUserLeftRoom() == nil {
		t.Fatalf("PlanRealtimeReplay plan = %+v, want one closing user_left_room event", plan)
	}
}

func TestPlanRealtimeReplayResetsForRoomCreationWithoutCurrentMembership(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	viewer, err := chatto.CreateUser(ctx, SystemActorID, "replay-room-create-viewer", "Replay Room Create Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	owner, err := chatto.CreateUser(ctx, SystemActorID, "replay-room-create-owner", "Replay Room Create Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	boundary, err := chatto.PlanRealtimeReplay(ctx, viewer.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	if _, err := chatto.CreateRoom(ctx, owner.Id, KindChannel, "", "replay-unseen-room", ""); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	plan, err := chatto.PlanRealtimeReplay(ctx, viewer.Id, boundary.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if !plan.Reset || len(plan.Events) != 0 {
		t.Fatalf("PlanRealtimeReplay plan = %+v, want snapshot fallback", plan)
	}
}

func TestPlanRealtimeReplayOmitsMessagesWithoutAReadMode(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	viewer, room, _ := setupReactionTest(t, chatto, ctx)

	if err := chatto.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageRead); err != nil {
		t.Fatalf("DenyRoomPermission message.read: %v", err)
	}
	if err := chatto.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageReadInteractions); err != nil {
		t.Fatalf("DenyRoomPermission message.read-interactions: %v", err)
	}
	boundary, err := chatto.PlanRealtimeReplay(ctx, viewer.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	if _, err := chatto.PostMessage(ctx, KindChannel, room.Id, viewer.Id, "write-only replay message", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage without message.read: %v", err)
	}

	plan, err := chatto.PlanRealtimeReplay(ctx, viewer.Id, boundary.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if plan.Reset {
		t.Fatalf("PlanRealtimeReplay reset = true, want filtered incremental replay")
	}
	for _, event := range plan.Events {
		if event.EVTEvent().GetMessagePosted() != nil {
			t.Fatalf("PlanRealtimeReplay delivered message without message.read: %+v", event.EVTEvent())
		}
	}
}

func TestPlanRealtimeReplayIncludesOnlyRelatedThreadMessages(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	viewer, err := chatto.CreateUser(ctx, SystemActorID, "replay-interaction-viewer", "Replay Interaction Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	author, err := chatto.CreateUser(ctx, SystemActorID, "replay-interaction-author", "Replay Interaction Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "replay-interactions", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{viewer.GetId(), author.GetId()} {
		if _, err := chatto.JoinRoom(ctx, userID, KindChannel, userID, room.GetId()); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	root, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "replay target root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	if _, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "earlier context", nil, root.GetId(), "", nil, false); err != nil {
		t.Fatalf("PostMessage earlier reply: %v", err)
	}
	if err := chatto.DenyUserRoomPermission(ctx, SystemActorID, room.GetId(), viewer.GetId(), PermMessageRead); err != nil {
		t.Fatalf("DenyUserRoomPermission message.read: %v", err)
	}
	if err := chatto.GrantUserRoomPermission(ctx, SystemActorID, room.GetId(), viewer.GetId(), PermMessageReadInteractions); err != nil {
		t.Fatalf("GrantUserRoomPermission message.read-interactions: %v", err)
	}
	boundary, err := chatto.PlanRealtimeReplay(ctx, viewer.GetId(), "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	mention, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "ping @replay-interaction-viewer", nil, root.GetId(), "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage mention: %v", err)
	}
	unrelated, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "unrelated new root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage unrelated: %v", err)
	}
	future, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "future related reply", nil, root.GetId(), "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage future reply: %v", err)
	}
	if added, err := chatto.ReactionModel().AddReaction(ctx, ReactionMutationInput{
		ActorID: author.GetId(), RoomID: room.GetId(), MessageEventID: future.GetId(), Emoji: "heart",
	}); err != nil || !added {
		t.Fatalf("AddReaction related reply = %v, %v", added, err)
	}
	if added, err := chatto.ReactionModel().AddReaction(ctx, ReactionMutationInput{
		ActorID: author.GetId(), RoomID: room.GetId(), MessageEventID: unrelated.GetId(), Emoji: "thumbsup",
	}); err != nil || !added {
		t.Fatalf("AddReaction unrelated root = %v, %v", added, err)
	}

	plan, err := chatto.PlanRealtimeReplay(ctx, viewer.GetId(), boundary.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay interactions: %v", err)
	}
	if plan.Reset {
		t.Fatal("PlanRealtimeReplay reset = true, want incremental replay")
	}
	posted := make(map[string]bool)
	reactions := make(map[string]bool)
	for _, envelope := range plan.Events {
		if event := envelope.EVTEvent(); event != nil && event.GetMessagePosted() != nil {
			posted[event.GetId()] = true
		} else if event != nil && event.GetReactionAdded() != nil {
			reactions[event.GetReactionAdded().GetMessageEventId()] = true
		}
	}
	if !posted[mention.GetId()] || !posted[future.GetId()] || posted[unrelated.GetId()] {
		t.Fatalf("replayed message IDs = %v; want mention and future only", posted)
	}
	if !reactions[future.GetId()] || reactions[unrelated.GetId()] {
		t.Fatalf("replayed reaction targets = %v; want related reply only", reactions)
	}
}

func TestPlanRealtimeReplayOmitsDMMessagesAfterMessageReadDenial(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	admin, err := chatto.CreateUser(ctx, SystemActorID, "replay-dm-admin", "Replay DM Admin", "password123")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := chatto.AssignAdminRole(ctx, admin.GetId()); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	viewer, err := chatto.CreateUser(ctx, SystemActorID, "replay-dm-viewer", "Replay DM Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	author, err := chatto.CreateUser(ctx, SystemActorID, "replay-dm-author", "Replay DM Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	dm, _, err := chatto.FindOrCreateDM(ctx, viewer.GetId(), []string{author.GetId()})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if err := chatto.SetUserPermissionState(ctx, admin.GetId(), viewer.GetId(), PermissionTargetScope{Kind: MatrixScopeDM}, PermMessageRead, PermissionStateDeny); err != nil {
		t.Fatalf("deny DM message.read: %v", err)
	}
	boundary, err := chatto.PlanRealtimeReplay(ctx, viewer.GetId(), "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	message, err := chatto.PostMessage(ctx, KindDM, dm.GetId(), author.GetId(), "replayed DM message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	plan, err := chatto.PlanRealtimeReplay(ctx, viewer.GetId(), boundary.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if plan.Reset {
		t.Fatalf("PlanRealtimeReplay reset = true, want incremental DM replay")
	}
	found := false
	for _, event := range plan.Events {
		if event.EVTEvent().GetId() == message.GetId() {
			found = true
			break
		}
	}
	if found {
		t.Fatalf("PlanRealtimeReplay disclosed DM message %s after message.read denial", message.GetId())
	}
}

func TestRealtimeReplayRequiresResetForServerProjectionAggregates(t *testing.T) {
	for _, subject := range []string{
		"evt.group.G1.room_group_updated",
		"evt.layout.default.room_moved",
	} {
		if !realtimeReplayRequiresReset(subject) {
			t.Fatalf("realtimeReplayRequiresReset(%q) = false", subject)
		}
	}
	if realtimeReplayRequiresReset("evt.config.server.server_name_changed") {
		t.Fatal("public server profile event unexpectedly requires reset")
	}
	if realtimeReplayRequiresReset("evt.room.R1.message_posted") {
		t.Fatal("room message unexpectedly requires reset")
	}
}

func TestPlanRealtimeReplayIncludesDurableUserPreferences(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "replay-preferences", "Replay Preferences", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	before, err := chatto.PlanRealtimeReplay(ctx, user.GetId(), "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	format := evtv1.TimeFormat_TIME_FORMAT_24H
	if _, err := chatto.UpdateUserSettings(ctx, user.GetId(), UserSettingsInput{TimeFormat: &format}); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}

	plan, err := chatto.PlanRealtimeReplay(ctx, user.GetId(), before.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if plan.Reset {
		t.Fatal("durable preference replay requested a snapshot reset")
	}
	found := false
	for _, envelope := range plan.Events {
		if event := envelope.EVTEvent(); event != nil && event.GetUserTimeFormatChanged().GetUserId() == user.GetId() {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("durable preference replay omitted user_time_format_changed")
	}
}

func TestPlanRealtimeReplayResetsForUnknownFutureEvent(t *testing.T) {
	chatto, nc := setupTestCore(t)
	ctx := testContext(t)
	const userID = "future-event-viewer"

	before, err := chatto.PlanRealtimeReplay(ctx, userID, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create JetStream client: %v", err)
	}
	data := protowire.AppendTag(nil, 19_999, protowire.BytesType)
	data = protowire.AppendBytes(data, nil)
	if _, err := js.Publish(ctx, "evt.room.room-1.future_room_fact", data); err != nil {
		t.Fatalf("publish future EVT event: %v", err)
	}

	plan, err := chatto.PlanRealtimeReplay(ctx, userID, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay after future event: %v", err)
	}
	if !plan.Reset || !plan.HadSequenceGap || len(plan.Events) != 0 {
		t.Fatalf("future event replay = %+v, want exact snapshot fallback", plan)
	}
}

func TestPlanRealtimeReplayResetsForUnknownAggregateNamespace(t *testing.T) {
	chatto, nc := setupTestCore(t)
	ctx := testContext(t)
	const userID = "future-aggregate-viewer"

	before, err := chatto.PlanRealtimeReplay(ctx, userID, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	event := &evtv1.Event{
		Id: "future-aggregate-event",
		Event: &evtv1.Event_UserCustomStatusSet{
			UserCustomStatusSet: &evtv1.UserCustomStatusSetEvent{UserId: "user-1", Status: &evtv1.CustomUserStatus{Text: "status"}},
		},
	}
	data, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal known payload on future aggregate: %v", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create JetStream client: %v", err)
	}
	subject := "evt.future.resource-1." + evtstream.EventUserCustomStatusSet
	if _, err := js.Publish(ctx, subject, data); err != nil {
		t.Fatalf("publish future aggregate event: %v", err)
	}

	plan, err := chatto.PlanRealtimeReplay(ctx, userID, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay after future aggregate: %v", err)
	}
	if !plan.Reset || !plan.HadSequenceGap || len(plan.Events) != 0 {
		t.Fatalf("future aggregate replay = %+v, want exact snapshot fallback", plan)
	}
}

func TestPlanRealtimeReplayResetsForMismatchedUserSubject(t *testing.T) {
	chatto, nc := setupTestCore(t)
	ctx := testContext(t)
	const userID = "mismatched-user-subject-viewer"

	before, err := chatto.PlanRealtimeReplay(ctx, userID, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	event := &evtv1.Event{
		Id: "mismatched-user-subject-event",
		Event: &evtv1.Event_UserCustomStatusSet{
			UserCustomStatusSet: &evtv1.UserCustomStatusSetEvent{UserId: "user-2"},
		},
	}
	data, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal user event: %v", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create JetStream client: %v", err)
	}
	subject := evtstream.UserAggregate("user-1").Subject(evtstream.EventUserCustomStatusSet)
	if _, err := js.Publish(ctx, subject, data); err != nil {
		t.Fatalf("publish mismatched user event: %v", err)
	}

	plan, err := chatto.PlanRealtimeReplay(ctx, userID, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay after mismatched user event: %v", err)
	}
	if !plan.Reset || !plan.HadSequenceGap || len(plan.Events) != 0 {
		t.Fatalf("mismatched user replay = %+v, want exact snapshot fallback", plan)
	}
}

func TestRealtimeReplayRoomSubject(t *testing.T) {
	roomID, ok := realtimeReplayRoomSubject(evtstream.RoomAggregate("R1").SubjectFor(&evtv1.Event{
		Event: &evtv1.Event_ReactionAdded{ReactionAdded: &evtv1.ReactionAddedEvent{}},
	}))
	if !ok || roomID != "R1" {
		t.Fatalf("realtimeReplayRoomSubject = %q, %v", roomID, ok)
	}
	if _, ok := realtimeReplayRoomSubject("evt.user.U1.custom_status_set"); ok {
		t.Fatal("user subject accepted as room replay subject")
	}
}
