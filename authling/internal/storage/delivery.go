package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/pkg/jetstreamutil"
)

// ErrDeliveryLimited means a delivery budget has no remaining allowance.
var ErrDeliveryLimited = errors.New("email delivery limit reached")

// DeliveryPolicy defines one workflow's refundable delivery budget. GlobalKey
// and the recipient keys passed to Reserve and Rollback must be product-owned
// constants or opaque digests, never email addresses or other PII.
type DeliveryPolicy struct {
	GlobalKey      string
	GlobalLimit    int
	RecipientLimit int
	// Window is the quiet period after the most recent counter write.
	Window time.Duration
}

// DeliveryBudget bounds delivery across replicas using revision-checked
// runtime-state counters. It is separate from non-refundable request admission.
// A lost write acknowledgement fails closed and can leave an allowance consumed
// until expiry. No uncertain write is retried or automatically refunded.
type DeliveryBudget struct {
	kv     KeyValue
	policy DeliveryPolicy
}

// NewDeliveryBudget constructs a budget over Authling's runtime-state bucket.
func NewDeliveryBudget(kv KeyValue, policy DeliveryPolicy) *DeliveryBudget {
	return &DeliveryBudget{kv: kv, policy: policy}
}

// Reserve consumes the global allowance before the recipient allowance.
// Only a successful call grants permission to deliver. If the recipient write
// fails, the confirmed global reservation is refunded on a best-effort basis.
func (b *DeliveryBudget) Reserve(ctx context.Context, recipientKey string) error {
	if b.policy.GlobalKey == "" || recipientKey == "" || recipientKey == b.policy.GlobalKey || b.policy.GlobalLimit < 1 || b.policy.RecipientLimit < 1 || b.policy.Window <= 0 {
		return fmt.Errorf("invalid email delivery policy")
	}
	if err := b.reserveCounter(ctx, b.policy.GlobalKey, b.policy.GlobalLimit); err != nil {
		return err
	}
	if err := b.reserveCounter(ctx, recipientKey, b.policy.RecipientLimit); err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return errors.Join(err, b.rollbackCounter(cleanup, b.policy.GlobalKey))
	}
	return nil
}

// Rollback refunds a successful Reserve after work or delivery fails. Call it
// at most once per reservation, with a live cleanup context. Both counters are
// attempted even if one fails. Refunds are best effort, not durable work.
func (b *DeliveryBudget) Rollback(ctx context.Context, recipientKey string) error {
	return errors.Join(b.rollbackCounter(ctx, recipientKey), b.rollbackCounter(ctx, b.policy.GlobalKey))
}

func (b *DeliveryBudget) reserveCounter(ctx context.Context, key string, limit int) error {
	limited, err := IncrementCounter(ctx, b.kv, key, limit, b.policy.Window)
	if err != nil {
		return fmt.Errorf("reserve email delivery: %w", err)
	}
	if limited {
		return ErrDeliveryLimited
	}
	return nil
}

func (b *DeliveryBudget) rollbackCounter(ctx context.Context, key string) error {
	for range maxCounterAttempts {
		entry, err := b.kv.Get(ctx, key)
		if IsKeyAbsent(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read email delivery rollback: %w", err)
		}
		counter, err := decodeCounter(entry.Value())
		if err != nil {
			return fmt.Errorf("rollback email delivery: %w", err)
		}
		if counter.Count == 1 {
			err = b.kv.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
		} else {
			counter.Count--
			data, _ := json.Marshal(counter)
			_, err = b.kv.UpdateWithTTL(ctx, key, data, entry.Revision(), b.policy.Window)
		}
		if err == nil {
			return nil
		}
		if !jetstreamutil.IsSequenceConflict(err) {
			return fmt.Errorf("rollback email delivery: %w", err)
		}
	}
	return fmt.Errorf("email delivery rollback conflicted repeatedly")
}
