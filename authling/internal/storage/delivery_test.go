package storage

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/natsruntime"
)

func openDeliveryStore(t *testing.T, cfg config.NATSConfig) (*natsruntime.Connection, jetstream.JetStream, jetstream.KeyValue) {
	t.Helper()
	connection, err := natsruntime.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	js, err := jetstream.New(connection.NATS)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := OpenStores(t.Context(), js, 1)
	if err != nil {
		t.Fatal(err)
	}
	return connection, js, stores.RuntimeState
}

func deliveryCount(t *testing.T, kv jetstream.KeyValue, key string) int {
	t.Helper()
	entry, err := kv.Get(t.Context(), key)
	if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	var counter deliveryCounter
	if err := json.Unmarshal(entry.Value(), &counter); err != nil {
		t.Fatal(err)
	}
	return counter.Count
}

func TestDeliveryBudgetConcurrentReservationsAndRefunds(t *testing.T) {
	cfg := config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}}
	connection, js, kv := openDeliveryStore(t, cfg)
	other, err := js.KeyValue(t.Context(), RuntimeStateBucket)
	if err != nil {
		t.Fatal(err)
	}
	policy := DeliveryPolicy{GlobalKey: "delivery.global", GlobalLimit: 100, RecipientLimit: 7, Window: time.Minute}
	budgets := []*DeliveryBudget{NewDeliveryBudget(kv, js, policy), NewDeliveryBudget(other, js, policy)}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := range 14 {
		wg.Go(func() {
			err := budgets[i%2].Reserve(t.Context(), "delivery.recipient")
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrDeliveryLimited) {
				t.Errorf("reserve: %v", err)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 7 || deliveryCount(t, kv, policy.GlobalKey) != 7 || deliveryCount(t, kv, "delivery.recipient") != 7 {
		t.Fatal("recipient limit or partial-reservation refund failed")
	}
	for i := range 7 {
		wg.Go(func() {
			if err := budgets[i%2].Rollback(t.Context(), "delivery.recipient"); err != nil {
				t.Errorf("rollback: %v", err)
			}
		})
	}
	wg.Wait()
	if deliveryCount(t, kv, policy.GlobalKey) != 0 || deliveryCount(t, kv, "delivery.recipient") != 0 {
		t.Fatal("concurrent refunds left consumed allowances")
	}
	// A deleted counter can be reused, and persisted counts survive restart.
	if err := budgets[0].Reserve(t.Context(), "delivery.recipient"); err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	_, js, kv = openDeliveryStore(t, cfg)
	if deliveryCount(t, kv, policy.GlobalKey) != 1 || deliveryCount(t, kv, "delivery.recipient") != 1 {
		t.Fatal("restart lost delivery allowances")
	}
	policy.GlobalLimit = 1
	if err := NewDeliveryBudget(kv, js, policy).Reserve(t.Context(), "delivery.other"); !errors.Is(err, ErrDeliveryLimited) {
		t.Fatalf("global cap: %v", err)
	}
	if deliveryCount(t, kv, "delivery.other") != 0 {
		t.Fatal("global rejection created a recipient counter")
	}
}

func TestDeliveryBudgetExpiry(t *testing.T) {
	_, js, kv := openDeliveryStore(t, config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}})
	budget := NewDeliveryBudget(kv, js, DeliveryPolicy{GlobalKey: "expiry.global", GlobalLimit: 1, RecipientLimit: 1, Window: time.Second})
	if err := budget.Reserve(t.Context(), "expiry.recipient"); err != nil {
		t.Fatal(err)
	}
	if err := budget.Reserve(t.Context(), "expiry.recipient"); !errors.Is(err, ErrDeliveryLimited) {
		t.Fatalf("budget not enforced before expiry: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for deliveryCount(t, kv, "expiry.global") != 0 || deliveryCount(t, kv, "expiry.recipient") != 0 {
		if time.Now().After(deadline) {
			t.Fatal("delivery counters did not expire")
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err := budget.Reserve(t.Context(), "expiry.recipient"); err != nil {
		t.Fatal(err)
	}
}

// afterRead forces a real concurrent write between reading a revision and the
// following mutation, or cancels a request after its global reservation.
type deliveryReadHook struct {
	jetstream.KeyValue
	afterRead func(string)
}

func (kv *deliveryReadHook) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	entry, err := kv.KeyValue.Get(ctx, key)
	kv.afterRead(key)
	return entry, err
}

func TestDeliveryRollbackRetriesConfirmedConflicts(t *testing.T) {
	for _, initial := range []string{`{"count":1}`, `{"count":2}`} {
		t.Run(initial, func(t *testing.T) {
			_, js, kv := openDeliveryStore(t, config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}})
			if _, err := kv.Put(t.Context(), "conflict", []byte(initial)); err != nil {
				t.Fatal(err)
			}
			var once sync.Once
			hooked := &deliveryReadHook{KeyValue: kv, afterRead: func(key string) {
				once.Do(func() {
					if _, err := kv.Put(t.Context(), key, []byte(`{"count":3}`)); err != nil {
						t.Fatal(err)
					}
				})
			}}
			budget := NewDeliveryBudget(hooked, js, DeliveryPolicy{Window: time.Minute})
			if err := budget.rollbackCounter(t.Context(), "conflict"); err != nil {
				t.Fatal(err)
			}
			if deliveryCount(t, kv, "conflict") != 2 {
				t.Fatal("refund lost a concurrent reservation")
			}
		})
	}
}

var errDeliveryAckLost = errors.New("injected delivery acknowledgement loss")

type deliveryLostAckKV struct{ jetstream.KeyValue }

func (kv deliveryLostAckKV) Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	if _, err := kv.KeyValue.Create(ctx, key, value, opts...); err != nil {
		return 0, err
	}
	return 0, errDeliveryAckLost
}

func (kv deliveryLostAckKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	if err := kv.KeyValue.Delete(ctx, key, opts...); err != nil {
		return err
	}
	return errDeliveryAckLost
}

type deliveryLostAckJS struct{ jetstream.JetStream }

func (js deliveryLostAckJS) PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	if _, err := js.JetStream.PublishMsg(ctx, msg, opts...); err != nil {
		return nil, err
	}
	return nil, errDeliveryAckLost
}

func TestDeliveryBudgetDoesNotRetryUnknownWrites(t *testing.T) {
	for _, tc := range []struct {
		name          string
		initial, want int
		refund        bool
	}{
		{"create", 0, 1, false},
		{"increment", 2, 3, false},
		{"decrement", 3, 2, true},
		{"delete", 1, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, js, kv := openDeliveryStore(t, config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}})
			if tc.initial > 0 {
				data, _ := json.Marshal(deliveryCounter{Count: tc.initial})
				if _, err := kv.Put(t.Context(), "unknown", data); err != nil {
					t.Fatal(err)
				}
			}
			budget := NewDeliveryBudget(deliveryLostAckKV{kv}, deliveryLostAckJS{js}, DeliveryPolicy{Window: time.Minute})
			var err error
			if tc.refund {
				err = budget.rollbackCounter(t.Context(), "unknown")
			} else {
				err = budget.reserveCounter(t.Context(), "unknown", 10)
			}
			if !errors.Is(err, errDeliveryAckLost) {
				t.Fatalf("unknown write outcome: %v", err)
			}
			if got := deliveryCount(t, kv, "unknown"); got != tc.want {
				t.Fatalf("counter = %d, want %d; uncertain write retried", got, tc.want)
			}
		})
	}
}

func TestDeliveryBudgetPartialFailureCleanup(t *testing.T) {
	_, js, kv := openDeliveryStore(t, config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}})
	policy := DeliveryPolicy{GlobalKey: "partial.global", GlobalLimit: 10, RecipientLimit: 1, Window: time.Minute}
	budget := NewDeliveryBudget(kv, js, policy)
	for _, malformed := range []string{`{"count":0}`, `{"count":-1}`, `invalid`} {
		if _, err := kv.Put(t.Context(), "partial.recipient", []byte(malformed)); err != nil {
			t.Fatal(err)
		}
		if err := budget.Reserve(t.Context(), "partial.recipient"); err == nil {
			t.Fatal("accepted corrupt recipient counter")
		}
		if deliveryCount(t, kv, policy.GlobalKey) != 0 {
			t.Fatal("failed recipient reservation leaked global allowance")
		}
	}
	if _, err := kv.Put(t.Context(), policy.GlobalKey, []byte(`{"count":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := budget.Rollback(t.Context(), "partial.recipient"); err == nil {
		t.Fatal("rollback hid corrupt recipient state")
	}
	if deliveryCount(t, kv, policy.GlobalKey) != 0 {
		t.Fatal("recipient rollback failure prevented global refund")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	hooked := &deliveryReadHook{KeyValue: kv, afterRead: func(key string) {
		if key == "partial.cancel" {
			cancel()
		}
	}}
	if err := NewDeliveryBudget(hooked, js, policy).Reserve(ctx, "partial.cancel"); err == nil {
		t.Fatal("cancelled reservation succeeded")
	}
	if deliveryCount(t, kv, policy.GlobalKey) != 0 {
		t.Fatal("request cancellation prevented global refund")
	}
}

func TestDeliveryGlobalBudgetAcrossRecipients(t *testing.T) {
	_, js, kv := openDeliveryStore(t, config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}})
	policy := DeliveryPolicy{GlobalKey: "global-cap.global", GlobalLimit: 7, RecipientLimit: 10, Window: time.Minute}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := range 14 {
		wg.Go(func() {
			budget := NewDeliveryBudget(kv, js, policy)
			err := budget.Reserve(t.Context(), "global-cap."+string(rune('a'+i)))
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrDeliveryLimited) {
				t.Errorf("reserve: %v", err)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 7 || deliveryCount(t, kv, policy.GlobalKey) != 7 {
		t.Fatal("concurrent recipients exceeded the global cap")
	}
}
