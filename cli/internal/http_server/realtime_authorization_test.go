package http_server

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/core"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

func TestRealtimeAuthorityGuardChecksInitialAndQueuedDelivery(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "guard-owner", "Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := env.core.CreateCookieSession(env.ctx, user.Id, "test")
	if err != nil {
		t.Fatal(err)
	}
	deadline, err := env.core.SetCookiePrivilegedMode(env.ctx, session, true)
	if err != nil {
		t.Fatal(err)
	}
	credential := authctx.RuntimeCredential{Kind: authctx.RuntimeCredentialKindCookieSession, Handle: session, UserID: user.Id, PrivilegedModeExpiresAt: deadline}
	ctx := authctx.WithCredential(env.ctx, credential)
	changed := make(chan struct{}, 1)
	changed <- struct{}{}
	if err := env.httpServer.requireRealtimeAuthority(ctx, changed); err != nil {
		t.Fatalf("unchanged metadata notification rejected authority: %v", err)
	}
	if _, err := env.core.SetCookiePrivilegedMode(env.ctx, session, false); err != nil {
		t.Fatal(err)
	}
	changed <- struct{}{}
	if err := env.httpServer.requireRealtimeAuthority(ctx, changed); !errors.Is(err, errRealtimeAuthorityChanged) {
		t.Fatalf("queued revocation error = %v", err)
	}
	credential.PrivilegedModeExpiresAt = time.Now().Add(-time.Second)
	if err := env.httpServer.requireRealtimeAuthority(authctx.WithCredential(env.ctx, credential), nil); !errors.Is(err, errRealtimeAuthorityChanged) {
		t.Fatalf("deadline error = %v", err)
	}
	credential.PrivilegedModeExpiresAt = time.Time{}
	if err := env.core.RevokeCookieSession(env.ctx, session); err != nil {
		t.Fatal(err)
	}
	changed <- struct{}{}
	if err := env.httpServer.requireRealtimeAuthority(authctx.WithCredential(env.ctx, credential), changed); !errors.Is(err, core.ErrNotAuthenticated) {
		t.Fatalf("session revocation error = %v", err)
	}
}

func TestRealtimeAuthorityRefreshesRenewableBearerOnTheSameSocket(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "bearer-mode-owner", "Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	session, err := env.core.CreateBearerSessionWithSource(env.ctx, user.Id, "test")
	if err != nil {
		t.Fatal(err)
	}
	conn := env.dialRealtime(t)
	subscribeRealtime(t, conn, session.AccessToken, realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
	readRealtimeCaughtUp(t, conn)
	for _, active := range []bool{true, false, true} {
		deadline, err := env.core.SetBearerPrivilegedMode(env.ctx, session.AccessToken, active)
		if err != nil {
			t.Fatal(err)
		}
		until := time.Now().Add(2 * time.Second)
		for {
			frame, ok := readRealtimeServerFrame(t, conn, time.Until(until))
			if !ok || frame.GetClose() != nil {
				t.Fatalf("bearer mode change lost socket: %v", frame)
			}
			hint := frame.GetEvent().GetViewerPermissionsChanged()
			if hint == nil || !hint.GetPrivilegedModeChanged() {
				continue
			}
			if active {
				if hint.GetPrivilegedModeExpiresAt() == nil || !hint.PrivilegedModeExpiresAt.AsTime().Equal(deadline) {
					t.Fatal("bearer mode acknowledgement has the wrong deadline")
				}
			} else if hint.GetPrivilegedModeExpiresAt() != nil {
				t.Fatal("inactive bearer mode retained its deadline")
			}
			break
		}
	}
}

// A revocation during handoff must stop already-planned privileged content.
// A second handoff must still deliver ordinary events committed in the gap.
func TestRealtimeAuthorityHandoffStopsQueuedContentAndRecoversGap(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "handoff-owner", "Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, owner.Id, core.RoleOwner); err != nil {
		t.Fatal(err)
	}
	rooms := make([]string, 2)
	for index, name := range []string{"restricted", "ordinary"} {
		room, err := env.core.CreateRoom(env.ctx, owner.Id, core.KindChannel, "", name, "")
		if err != nil {
			t.Fatal(err)
		}
		rooms[index] = room.Id
		if _, err := env.core.JoinRoom(env.ctx, owner.Id, core.KindChannel, owner.Id, room.Id); err != nil {
			t.Fatal(err)
		}
	}
	for _, permission := range []core.Permission{core.PermMessageRead, core.PermMessageReadInteractions} {
		if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, rooms[0], core.RoleEveryone, permission); err != nil {
			t.Fatal(err)
		}
	}
	sessionID, _, err := env.core.CreateCookieSession(env.ctx, owner.Id, "test")
	if err != nil {
		t.Fatal(err)
	}
	deadline, err := env.core.SetCookiePrivilegedMode(env.ctx, sessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	credential := authctx.RuntimeCredential{Kind: authctx.RuntimeCredentialKindCookieSession, UserID: owner.Id, Handle: sessionID, PrivilegedModeExpiresAt: deadline}
	ctx := authctx.WithCredential(env.ctx, credential)
	boundary, _, err := env.httpServer.realtimeSnapshotFrame(ctx, owner.Id)
	if err != nil {
		t.Fatal(err)
	}
	post := func(roomID, body string) string {
		event, err := env.core.PostMessage(env.ctx, core.KindChannel, roomID, owner.Id, body, nil, "", "", nil, false)
		if err != nil {
			t.Fatal(err)
		}
		return event.Id
	}
	first := post(rooms[0], "Authorized before revocation")
	second := post(rooms[0], "Must not leave the queued plan")
	changed := make(chan struct{}, 1)
	var ordinary string
	var delivered []string
	_, stop, lastWritten, err := env.httpServer.refreshRealtimeAuthorization(ctx, owner.Id, boundary, changed, func(frame *realtimev1.RealtimeServerFrame) error {
		if event := frame.GetEvent(); event.GetMessagePosted() != nil {
			delivered = append(delivered, event.Id)
			if event.Id == first {
				if _, err := env.core.SetCookiePrivilegedMode(env.ctx, sessionID, false); err != nil {
					t.Fatal(err)
				}
				ordinary = post(rooms[1], "Committed during the authority handoff")
				changed <- struct{}{}
			}
		}
		return nil
	})
	stop()
	if !errors.Is(err, errRealtimeAuthorityChanged) {
		t.Fatalf("handoff error = %v", err)
	}
	if len(delivered) != 1 || delivered[0] != first {
		t.Fatalf("queued plan disclosed %d messages", len(delivered))
	}
	credential.PrivilegedModeExpiresAt = time.Time{}
	ctx = authctx.WithCredential(env.ctx, credential)
	acknowledged := false
	events, stop, _, err := env.httpServer.refreshRealtimeAuthorization(ctx, owner.Id, lastWritten, changed, func(frame *realtimev1.RealtimeServerFrame) error {
		if event := frame.GetEvent(); event.GetMessagePosted() != nil {
			delivered = append(delivered, event.Id)
		}
		if hint := frame.GetEvent().GetViewerPermissionsChanged(); hint != nil {
			acknowledged = hint.GetPrivilegedModeChanged() && hint.GetPrivilegedModeExpiresAt() == nil
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if !acknowledged || len(delivered) != 2 || delivered[1] != ordinary {
		t.Fatalf("handoff did not recover ordinary delivery: acknowledged=%v count=%d", acknowledged, len(delivered))
	}
	for _, id := range delivered {
		if id == second {
			t.Fatal("revoked queued message was disclosed")
		}
	}
	post(rooms[0], "Must remain private on the refreshed stream")
	sentinel := post(rooms[1], "Ordinary live stream stays connected")
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("refreshed stream closed")
			}
			if event.ID() == sentinel {
				return
			}
			if message := core.EventMessagePosted(event); message != nil && message.RoomId == rooms[0] {
				t.Fatal("refreshed stream retained privileged read access")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("refreshed stream lost live delivery")
		}
	}
}

func TestRealtimeAuthorityExpiresOnTheSameSocket(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "deadline-owner", "Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	env.login(t, "deadline-owner", "password123")
	var sessionID string
	for _, cookie := range env.cookieJar.Cookies(mustParseURL(env.server.URL)) {
		if isBrowserSessionCookieName(cookie.Name) {
			sessionID = cookie.Value
		}
	}
	if sessionID == "" {
		t.Fatal("missing cookie session")
	}
	if _, err := env.core.SetCookiePrivilegedMode(env.ctx, sessionID, true); err != nil {
		t.Fatal(err)
	}
	conn := env.dialRealtime(t)
	subscribeRealtime(t, conn, "", realtimev1.RealtimeInitialState_REALTIME_INITIAL_STATE_LIVE_ONLY, "")
	readRealtimeCaughtUp(t, conn)
	entry, err := env.core.LoadCookieSessionValue(env.ctx, sessionID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var record core.AuthTokenData
	if err := json.Unmarshal(entry.Value, &record); err != nil {
		t.Fatal(err)
	}
	record.PrivilegedModeExpiresAt = time.Now().Add(1500 * time.Millisecond)
	value, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.core.UpdateCookieSessionValue(env.ctx, sessionID, value, entry.Revision, time.Now()); err != nil {
		t.Fatal(err)
	}
	for {
		frame, ok := readRealtimeServerFrame(t, conn, 2*time.Second)
		if !ok || frame.GetClose() != nil {
			t.Fatalf("expiry closed or stalled socket: %+v", frame)
		}
		if hint := frame.GetEvent().GetViewerPermissionsChanged(); hint.GetPrivilegedModeChanged() && hint.GetPrivilegedModeExpiresAt() == nil {
			break
		}
	}
	if err := env.core.SetPresence(env.ctx, user.Id, core.PresenceStatusAway); err != nil {
		t.Fatal(err)
	}
	for {
		frame, ok := readRealtimeServerFrame(t, conn, 2*time.Second)
		if !ok || frame.GetClose() != nil {
			t.Fatalf("delivery after expiry = %+v", frame)
		}
		if frame.GetEvent().GetPresenceChanged() != nil {
			return
		}
	}
}
