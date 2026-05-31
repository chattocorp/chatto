package migrations

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func TestMigratePushSubscriptionsToRuntimeState(t *testing.T) {
	ctx, legacy, runtimeState := setupPushSubscriptionMigrationKV(t)
	subscription := &corev1.PushSubscription{
		Endpoint:  "https://push.example.com/device1",
		P256Dh:    "p256dh-key",
		Auth:      "auth-secret",
		CreatedAt: timestamppb.Now(),
		UserAgent: "TestBrowser/1.0",
	}
	data, err := proto.Marshal(subscription)
	if err != nil {
		t.Fatalf("marshal subscription: %v", err)
	}

	if _, err := legacy.Put(ctx, "push_subscription.U1.abc123", data); err != nil {
		t.Fatalf("put legacy subscription: %v", err)
	}

	if err := MigratePushSubscriptionsToRuntimeState(ctx, legacy, runtimeState, testLogger()); err != nil {
		t.Fatalf("migrate push subscriptions: %v", err)
	}

	entry, err := runtimeState.Get(ctx, "push_subscription.U1.abc123")
	if err != nil {
		t.Fatalf("get migrated subscription: %v", err)
	}
	var migrated corev1.PushSubscription
	if err := proto.Unmarshal(entry.Value(), &migrated); err != nil {
		t.Fatalf("unmarshal migrated subscription: %v", err)
	}
	if migrated.GetEndpoint() != subscription.GetEndpoint() ||
		migrated.GetP256Dh() != subscription.GetP256Dh() ||
		migrated.GetAuth() != subscription.GetAuth() ||
		migrated.GetUserAgent() != subscription.GetUserAgent() {
		t.Fatalf("migrated subscription = %+v, want %+v", &migrated, subscription)
	}

	if _, err := legacy.Get(ctx, "push_subscription.U1.abc123"); err != nil {
		t.Fatalf("legacy subscription should be retained for rollback: %v", err)
	}
}

func TestMigratePushSubscriptionsToRuntimeState_DoesNotOverwriteRuntimeState(t *testing.T) {
	ctx, legacy, runtimeState := setupPushSubscriptionMigrationKV(t)

	legacySub := &corev1.PushSubscription{Endpoint: "https://push.example.com/legacy", P256Dh: "legacy-key", Auth: "legacy-auth"}
	runtimeSub := &corev1.PushSubscription{Endpoint: "https://push.example.com/runtime", P256Dh: "runtime-key", Auth: "runtime-auth"}
	legacyData, err := proto.Marshal(legacySub)
	if err != nil {
		t.Fatalf("marshal legacy subscription: %v", err)
	}
	runtimeData, err := proto.Marshal(runtimeSub)
	if err != nil {
		t.Fatalf("marshal runtime subscription: %v", err)
	}

	if _, err := legacy.Put(ctx, "push_subscription.U1.abc123", legacyData); err != nil {
		t.Fatalf("put legacy subscription: %v", err)
	}
	if _, err := runtimeState.Put(ctx, "push_subscription.U1.abc123", runtimeData); err != nil {
		t.Fatalf("put runtime subscription: %v", err)
	}

	if err := MigratePushSubscriptionsToRuntimeState(ctx, legacy, runtimeState, testLogger()); err != nil {
		t.Fatalf("migrate push subscriptions: %v", err)
	}

	entry, err := runtimeState.Get(ctx, "push_subscription.U1.abc123")
	if err != nil {
		t.Fatalf("get runtime subscription: %v", err)
	}
	var got corev1.PushSubscription
	if err := proto.Unmarshal(entry.Value(), &got); err != nil {
		t.Fatalf("unmarshal runtime subscription: %v", err)
	}
	if got.GetEndpoint() != runtimeSub.GetEndpoint() {
		t.Fatalf("runtime endpoint = %q, want %q", got.GetEndpoint(), runtimeSub.GetEndpoint())
	}
}

func setupPushSubscriptionMigrationKV(t *testing.T) (context.Context, jetstream.KeyValue, jetstream.KeyValue) {
	t.Helper()

	_, nc := testutil.StartNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}

	legacy, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "INSTANCE",
		Storage: jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatalf("create legacy KV: %v", err)
	}
	runtimeState, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "RUNTIME_STATE",
		Storage: jetstream.MemoryStorage,
		History: 1,
	})
	if err != nil {
		t.Fatalf("create runtime state KV: %v", err)
	}
	return ctx, legacy, runtimeState
}
