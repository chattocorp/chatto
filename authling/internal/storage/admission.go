package storage

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrAdmissionLimited means the shared request budget is exhausted.
var ErrAdmissionLimited = errors.New("request admission limit reached")

// AdmitRequest consumes a non-refundable request allowance in runtime state.
// key must be a product-owned constant or an opaque keyed digest, never PII.
// Each admission resets the quiet window. OCC bounds admissions across replicas;
// an OCC conflict reads the counter again. Storage failures and unknown
// acknowledgements fail closed without refunds.
func AdmitRequest(ctx context.Context, kv KeyValue, key string, limit int, window time.Duration) error {
	if limit < 1 || window <= 0 {
		return fmt.Errorf("invalid request admission policy")
	}
	limited, err := IncrementCounter(ctx, kv, key, limit, window)
	if err != nil {
		return fmt.Errorf("request admission: %w", err)
	}
	if limited {
		return ErrAdmissionLimited
	}
	return nil
}
