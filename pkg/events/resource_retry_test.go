package events_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/pkg/events"
)

func TestCreateJetStreamResourceWithRetryErrors(t *testing.T) {
	permanent := errors.New("invalid configuration")
	for _, tc := range []struct {
		name  string
		err   error
		retry bool
	}{
		{"deadline", context.DeadlineExceeded, true},
		{"wrapped deadline", fmt.Errorf("request: %w", context.DeadlineExceeded), true},
		{"store creation", &jetstream.APIError{Code: 500, ErrorCode: 10049, Description: "error creating store for stream"}, true},
		{"name conflict", fmt.Errorf("create: %w", &jetstream.APIError{Code: 400, ErrorCode: 10058, Description: "stream name already in use"}), true},
		{"permanent", permanent, false},
		{"configuration", &jetstream.APIError{Code: 400, ErrorCode: 10052, Description: "invalid configuration"}, false},
		{"other store failure", &jetstream.APIError{Code: 500, ErrorCode: 10049, Description: "permanent failure"}, false},
		{"canceled", context.Canceled, false},
		{"legacy timeout", nats.ErrTimeout, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				attempts := 0
				start := time.Now()
				got, err := events.CreateJetStreamResourceWithRetry(context.Background(), events.JetStreamResourceRetryPolicy{MaxAttempts: 3, RetryDelay: 25 * time.Millisecond}, func(context.Context) (string, error) {
					attempts++
					if attempts < 3 {
						return "discard", tc.err
					}
					return "resource", nil
				})
				if tc.retry {
					if err != nil || got != "resource" || attempts != 3 {
						t.Fatalf("got (%q, %v), attempts %d", got, err, attempts)
					}
					if elapsed := time.Since(start); elapsed != 75*time.Millisecond {
						t.Fatalf("virtual backoff = %v, want 75ms", elapsed)
					}
				} else if !errors.Is(err, tc.err) || got != "" || attempts != 1 {
					t.Fatalf("got (%q, %v), attempts %d; want permanent error after one call", got, err, attempts)
				}
			})
		})
	}
}

func TestCreateJetStreamResourceWithRetryExhaustion(t *testing.T) {
	attempts := 0
	want := fmt.Errorf("last request: %w", context.DeadlineExceeded)
	got, err := events.CreateJetStreamResourceWithRetry(context.Background(), events.JetStreamResourceRetryPolicy{MaxAttempts: 3}, func(context.Context) (string, error) {
		attempts++
		if attempts == 3 {
			return "discard", want
		}
		return "discard", context.DeadlineExceeded
	})
	if err != want || got != "" || attempts != 3 {
		t.Fatalf("got (%q, %v), attempts %d", got, err, attempts)
	}
}

func TestCreateJetStreamResourceWithRetryInvalidPolicy(t *testing.T) {
	for _, policy := range []events.JetStreamResourceRetryPolicy{
		{}, {MaxAttempts: -1}, {MaxAttempts: 1, RetryDelay: -1}, {MaxAttempts: 3, RetryDelay: time.Duration(1<<63 - 1)},
	} {
		_, err := events.CreateJetStreamResourceWithRetry(context.Background(), policy, func(context.Context) (int, error) {
			t.Fatal("invalid policy called operation")
			return 0, nil
		})
		if err == nil {
			t.Fatalf("policy %+v accepted", policy)
		}
	}
}

func TestCreateJetStreamResourceWithRetryParentContext(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		for _, phase := range []string{"before", "operation", "backoff", "successful operation", "last attempt"} {
			t.Run(fmt.Sprintf("deadline=%t/%s", deadline, phase), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					want := context.Canceled
					if deadline {
						cancel()
						ctx, cancel = context.WithTimeout(context.Background(), time.Second)
						want = context.DeadlineExceeded
					}
					defer cancel()
					endParent := func() {
						if deadline {
							<-ctx.Done()
						} else {
							cancel()
						}
					}
					if phase == "before" {
						endParent()
					}
					attempts := 0
					done := make(chan struct{})
					go func() {
						defer close(done)
						delay := time.Hour
						if phase == "last attempt" {
							delay = 0
						}
						got, err := events.CreateJetStreamResourceWithRetry(ctx, events.JetStreamResourceRetryPolicy{MaxAttempts: 3, RetryDelay: delay}, func(context.Context) (string, error) {
							attempts++
							if phase == "operation" || phase == "successful operation" || (phase == "last attempt" && attempts == 3) {
								endParent()
							}
							if phase == "successful operation" {
								return "discard", nil
							}
							return "discard", context.DeadlineExceeded
						})
						if !errors.Is(err, want) || got != "" {
							t.Errorf("got (%q, %v), want parent %v", got, err, want)
						}
					}()
					if phase == "backoff" {
						synctest.Wait()
						endParent()
					}
					<-done
					wantAttempts := 1
					if phase == "before" {
						wantAttempts = 0
					}
					if phase == "last attempt" {
						wantAttempts = 3
					}
					if attempts != wantAttempts {
						t.Fatalf("attempts = %d, want %d", attempts, wantAttempts)
					}
				})
			})
		}
	}
}

// The first management request has a responder but no reply. This exercises the
// actual client timeout path without depending on the clustered creation race.
func TestCreateJetStreamResourceWithRetryClientTimeout(t *testing.T) {
	nc := startTestNATS(t)
	requests := 0
	sub, err := nc.Subscribe("test.API.STREAM.UPDATE.RESOURCE", func(msg *nats.Msg) {
		requests++
		if requests == 1 {
			return
		}
		if err := msg.Respond([]byte(`{"config":{"name":"RESOURCE"},"created":"2026-01-01T00:00:00Z","state":{}}`)); err != nil {
			t.Errorf("reply: %v", err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	js, err := jetstream.NewWithAPIPrefix(nc, "test.API", jetstream.WithDefaultTimeout(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts := 0
	stream, err := events.CreateJetStreamResourceWithRetry(ctx, events.JetStreamResourceRetryPolicy{MaxAttempts: 3}, func(ctx context.Context) (jetstream.Stream, error) {
		attempts++
		stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{Name: "RESOURCE"})
		if attempts == 1 && (!errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil) {
			t.Errorf("request error = %v, parent error = %v", err, ctx.Err())
		}
		return stream, err
	})
	if err != nil || stream == nil || attempts != 2 {
		t.Fatalf("stream %v, error %v, attempts %d", stream, err, attempts)
	}
}
