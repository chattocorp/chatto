package core

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

func TestSharedRealtimeMappingDoesNotStoreErrors(t *testing.T) {
	envelope := NewEVTEventEnvelopeWithDeliverySeq(&evtv1.Event{Id: "E1"}, 1)
	failure := errors.New("lookup failed")
	if _, err := envelope.SharedRealtimeMapping(func() (any, error) { return nil, failure }); !errors.Is(err, failure) {
		t.Fatalf("first call error = %v, want %v", err, failure)
	}
	value, err := envelope.SharedRealtimeMapping(func() (any, error) { return "mapped", nil })
	if err != nil || value != "mapped" {
		t.Fatalf("second call = %v, %v; want computed value after a failure", value, err)
	}
	value, err = envelope.SharedRealtimeMapping(func() (any, error) {
		t.Fatal("stored value computed again")
		return nil, nil
	})
	if err != nil || value != "mapped" {
		t.Fatalf("third call = %v, %v; want stored value", value, err)
	}
	other := NewEVTEventEnvelopeWithDeliverySeq(&evtv1.Event{Id: "E1"}, 1)
	value, _ = other.SharedRealtimeMapping(func() (any, error) { return "other", nil })
	if value != "other" {
		t.Fatalf("new envelope = %v, want its own value", value)
	}
}

func TestSharedRealtimeMappingComputesOnceForConcurrentCallers(t *testing.T) {
	envelope := NewEVTEventEnvelopeWithDeliverySeq(&evtv1.Event{Id: "E1"}, 1)
	var computed atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			value, err := envelope.SharedRealtimeMapping(func() (any, error) {
				computed.Add(1)
				return "mapped", nil
			})
			if err != nil || value != "mapped" {
				t.Errorf("SharedRealtimeMapping = %v, %v", value, err)
			}
		})
	}
	wg.Wait()
	if got := computed.Load(); got != 1 {
		t.Fatalf("computed %d times, want 1", got)
	}
}
