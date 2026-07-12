package core

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func newTestPresenceModel(t *testing.T) (*PresenceModel, jetstream.KeyValue, *log.Logger) {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream.New: %v", err)
	}
	memoryCacheKV, err := js.CreateOrUpdateKeyValue(testContext(t), jetstream.KeyValueConfig{
		Bucket:         "MEMORY_CACHE",
		Storage:        jetstream.MemoryStorage,
		LimitMarkerTTL: PresenceTTL,
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateKeyValue: %v", err)
	}
	logger := testCoreLogger()
	return NewPresenceModel(js, memoryCacheKV, logger), memoryCacheKV, logger
}

func TestNewPresenceModelWiresDependencies(t *testing.T) {
	service, memoryCacheKV, logger := newTestPresenceModel(t)

	if service.js == nil {
		t.Fatal("JetStream handle was not wired")
	}
	if service.memoryCacheKV != memoryCacheKV {
		t.Fatal("memory cache KV was not wired")
	}
	if service.logger != logger {
		t.Fatal("logger was not wired")
	}
	if service.hub == nil {
		t.Fatal("presence hub was not initialized")
	}
}

func TestPresenceModelSetAndGetPresence(t *testing.T) {
	service, _, _ := newTestPresenceModel(t)
	ctx := testContext(t)

	if got, err := service.GetUserPresence(ctx, "U-service"); err != nil || got != PresenceStatusOffline {
		t.Fatalf("initial GetUserPresence = %q, %v; want %q, nil", got, err, PresenceStatusOffline)
	}
	if err := service.SetPresence(ctx, "U-service", PresenceStatusDoNotDisturb); err != nil {
		t.Fatalf("SetPresence returned error: %v", err)
	}
	if got, err := service.GetUserPresence(ctx, "U-service"); err != nil || got != PresenceStatusDoNotDisturb {
		t.Fatalf("GetUserPresence = %q, %v; want %q, nil", got, err, PresenceStatusDoNotDisturb)
	}
	if err := service.SetPresence(ctx, "U-service", PresenceStatusAway); err != nil {
		t.Fatalf("second SetPresence returned error: %v", err)
	}
	if got, err := service.GetUserPresence(ctx, "U-service"); err != nil || got != PresenceStatusAway {
		t.Fatalf("GetUserPresence after update = %q, %v; want %q, nil", got, err, PresenceStatusAway)
	}
	if err := service.refreshPresence(ctx, "U-service"); err != nil {
		t.Fatalf("refreshPresence returned error: %v", err)
	}
	if got, err := service.GetUserPresence(ctx, "U-service"); err != nil || got != PresenceStatusAway {
		t.Fatalf("GetUserPresence after refresh = %q, %v; want %q, nil", got, err, PresenceStatusAway)
	}
}

func TestPresenceModelSetPresenceStatusMapping(t *testing.T) {
	service, _, _ := newTestPresenceModel(t)
	ctx := testContext(t)

	tests := []struct {
		name   string
		userID string
		status string
		want   string
	}{
		{name: "online", userID: "U-online", status: PresenceStatusOnline, want: PresenceStatusOnline},
		{name: "away", userID: "U-away", status: PresenceStatusAway, want: PresenceStatusAway},
		{name: "do not disturb", userID: "U-dnd", status: PresenceStatusDoNotDisturb, want: PresenceStatusDoNotDisturb},
		{name: "unknown defaults online", userID: "U-unknown", status: "BUSY", want: PresenceStatusOnline},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := service.SetPresence(ctx, tt.userID, tt.status); err != nil {
				t.Fatalf("SetPresence returned error: %v", err)
			}
			got, err := service.GetUserPresence(ctx, tt.userID)
			if err != nil {
				t.Fatalf("GetUserPresence returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("GetUserPresence = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPresenceModelGetUserPresenceTreatsDeletesAndCorruptValuesAsOffline(t *testing.T) {
	service, kv, _ := newTestPresenceModel(t)
	ctx := testContext(t)

	if err := service.SetPresence(ctx, "U-delete", PresenceStatusOnline); err != nil {
		t.Fatalf("SetPresence returned error: %v", err)
	}
	if err := kv.Delete(ctx, presenceKey("U-delete")); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if got, err := service.GetUserPresence(ctx, "U-delete"); err != nil || got != PresenceStatusOffline {
		t.Fatalf("deleted GetUserPresence = %q, %v; want %q, nil", got, err, PresenceStatusOffline)
	}

	if _, err := kv.Put(ctx, presenceKey("U-corrupt"), []byte("not protobuf")); err != nil {
		t.Fatalf("Put corrupt returned error: %v", err)
	}
	if got, err := service.GetUserPresence(ctx, "U-corrupt"); err != nil || got != PresenceStatusOffline {
		t.Fatalf("corrupt GetUserPresence = %q, %v; want %q, nil", got, err, PresenceStatusOffline)
	}
}

func TestPresenceModelGetUserPresenceTreatsInvalidUserIDAsOffline(t *testing.T) {
	service, _, _ := newTestPresenceModel(t)
	ctx := testContext(t)

	for _, userID := range []string{"", "bad>", ".bad", "bad."} {
		t.Run(userID, func(t *testing.T) {
			got, err := service.GetUserPresence(ctx, userID)
			if err != nil || got != PresenceStatusOffline {
				t.Fatalf("GetUserPresence(%q) = %q, %v; want %q, nil", userID, got, err, PresenceStatusOffline)
			}
		})
	}
}

func TestPresenceModelRefreshMissingEntrySetsOnline(t *testing.T) {
	service, _, _ := newTestPresenceModel(t)
	ctx := testContext(t)

	if err := service.refreshPresence(ctx, "U-missing"); err != nil {
		t.Fatalf("refreshPresence returned error: %v", err)
	}
	if got, err := service.GetUserPresence(ctx, "U-missing"); err != nil || got != PresenceStatusOnline {
		t.Fatalf("GetUserPresence = %q, %v; want %q, nil", got, err, PresenceStatusOnline)
	}
}

func TestPresenceModelKeyHelpers(t *testing.T) {
	if got := presenceKey("U-key"); got != "presence.U-key" {
		t.Fatalf("presenceKey = %q, want %q", got, "presence.U-key")
	}

	tests := []struct {
		name       string
		key        string
		wantUserID string
		wantOK     bool
	}{
		{name: "presence key", key: "presence.U-key", wantUserID: "U-key", wantOK: true},
		{name: "empty user", key: "presence.", wantOK: false},
		{name: "wrong prefix", key: "other.U-key", wantOK: false},
		{name: "too short", key: "presence", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUserID, gotOK := parsePresenceKey(tt.key)
			if gotUserID != tt.wantUserID || gotOK != tt.wantOK {
				t.Fatalf("parsePresenceKey(%q) = %q, %v; want %q, %v", tt.key, gotUserID, gotOK, tt.wantUserID, tt.wantOK)
			}
		})
	}
}

func TestPresenceModelChattoCoreFacades(t *testing.T) {
	service, _, _ := newTestPresenceModel(t)
	core := &ChattoCore{presenceModel: service}
	ctx := testContext(t)

	if err := core.SetPresence(ctx, "U-facade", PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence facade returned error: %v", err)
	}
	if got, err := core.GetUserPresence(ctx, "U-facade"); err != nil || got != PresenceStatusAway {
		t.Fatalf("GetUserPresence facade = %q, %v; want %q, nil", got, err, PresenceStatusAway)
	}
	if err := core.refreshPresence(ctx, "U-facade"); err != nil {
		t.Fatalf("refreshPresence facade returned error: %v", err)
	}
	if got, err := service.GetUserPresence(ctx, "U-facade"); err != nil || got != PresenceStatusAway {
		t.Fatalf("service GetUserPresence after facade refresh = %q, %v; want %q, nil", got, err, PresenceStatusAway)
	}
}

func TestPresenceModelLivePresenceCount(t *testing.T) {
	service, kv, _ := newTestPresenceModel(t)
	ctx := testContext(t)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- service.Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	if got, err := service.LivePresenceCount(ctx); err != nil || got != 0 {
		t.Fatalf("initial LivePresenceCount = %d, %v; want 0, nil", got, err)
	}

	if err := service.SetPresence(ctx, "U-online", PresenceStatusOnline); err != nil {
		t.Fatalf("SetPresence online: %v", err)
	}
	if err := service.SetPresence(ctx, "U-away", PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence away: %v", err)
	}
	if err := service.SetPresence(ctx, "U-dnd", PresenceStatusDoNotDisturb); err != nil {
		t.Fatalf("SetPresence dnd: %v", err)
	}
	if _, err := kv.Put(ctx, presenceKey("U-corrupt"), []byte("not protobuf")); err != nil {
		t.Fatalf("Put corrupt returned error: %v", err)
	}

	waitForPresenceCount(t, ctx, service, 3)

	if err := kv.Delete(ctx, presenceKey("U-away")); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	waitForPresenceCount(t, ctx, service, 2)
}

func TestPresenceModelSubscribeAndUnsubscribe(t *testing.T) {
	service, kv, _ := newTestPresenceModel(t)
	ctx := testContext(t)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- service.Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	sub, err := service.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	service.Unsubscribe(sub)

	data, err := proto.Marshal(&corev1.UserPresence{Status: corev1.UserPresenceStatus_USER_PRESENCE_STATUS_ONLINE})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if _, err := kv.Put(ctx, presenceKey("U-sub"), data); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
}

func waitForPresenceCount(t *testing.T, ctx context.Context, service *PresenceModel, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var got int
	var err error
	for time.Now().Before(deadline) {
		got, err = service.LivePresenceCount(ctx)
		if err == nil && got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("LivePresenceCount = %d, %v; want %d", got, err, want)
}
