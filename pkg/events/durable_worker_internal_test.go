package events

import (
	"errors"
	"testing"
	"time"
)

func TestDurableWorkerAdaptIdleFetchWait(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want time.Duration
	}{
		{"limit from rejection", errors.New("nats: Exceeded MaxRequestExpires of 2s"), 2 * time.Second},
		{"limit with fraction", errors.New("nats: Exceeded MaxRequestExpires of 1.5s"), 1500 * time.Millisecond},
		{"unreadable limit halves", errors.New("nats: Exceeded MaxRequestExpires of soon"), 15 * time.Second},
		{"missing limit halves", errors.New("nats: Exceeded MaxRequestExpires of "), 15 * time.Second},
		{"limit below FetchMaxWait", errors.New("nats: Exceeded MaxRequestExpires of 10ms"), time.Second},
		{"limit above current wait", errors.New("nats: Exceeded MaxRequestExpires of 1m"), 30 * time.Second},
		{"other failure keeps wait", errors.New("nats: no heartbeat received"), 30 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			worker := &DurableWorker{
				opts:          DurableWorkerOptions{FetchMaxWait: time.Second},
				idleFetchWait: durableWorkerIdleFetchWait,
			}
			worker.adaptIdleFetchWait(test.err)
			if worker.idleFetchWait != test.want {
				t.Fatalf("idle fetch wait = %v, want %v", worker.idleFetchWait, test.want)
			}
		})
	}
}
