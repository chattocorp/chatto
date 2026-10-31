package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestPutAttachmentObjectRetries(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		reader := bytes.NewReader([]byte("skip-payload"))
		_, err := reader.Seek(5, io.SeekStart)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		info, err := putAttachmentObject(t.Context(), func(_ context.Context, meta jetstream.ObjectMeta, r io.Reader) (*jetstream.ObjectInfo, error) {
			calls++
			body, err := io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != "payload" || meta.Name != "stable-key" {
				t.Fatal("retry changed payload or key")
			}
			if calls < 3 {
				return nil, fmt.Errorf("store: %w", &jetstream.APIError{Code: 429, ErrorCode: 10167})
			}
			return &jetstream.ObjectInfo{}, nil
		}, jetstream.ObjectMeta{Name: "stable-key"}, reader)
		if err != nil || info == nil || calls != 3 {
			t.Fatalf("calls=%d info=%v err=%v", calls, info, err)
		}
	})
}

func TestPutAttachmentObjectRetryLimits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		err      error
		seekable bool
		want     int
	}{
		{"exhausted", &jetstream.APIError{Code: 429, ErrorCode: 10167}, true, 5},
		{"other rejection", &jetstream.APIError{Code: 429, ErrorCode: 10000}, true, 1},
		{"unknown outcome", context.DeadlineExceeded, true, 1},
		{"nonseekable", &jetstream.APIError{Code: 429, ErrorCode: 10167}, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var reader io.Reader = bytes.NewReader([]byte("payload"))
				if !tc.seekable {
					reader = struct{ io.Reader }{reader}
				}
				calls := 0
				_, err := putAttachmentObject(t.Context(), func(context.Context, jetstream.ObjectMeta, io.Reader) (*jetstream.ObjectInfo, error) {
					calls++
					return nil, tc.err
				}, jetstream.ObjectMeta{}, reader)
				if !errors.Is(err, tc.err) || calls != tc.want {
					t.Fatalf("calls=%d err=%v", calls, err)
				}
			})
		})
	}
}

func TestPutAttachmentObjectCancellation(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()
		calls := 0
		_, err := putAttachmentObject(ctx, func(context.Context, jetstream.ObjectMeta, io.Reader) (*jetstream.ObjectInfo, error) {
			calls++
			return nil, &jetstream.APIError{Code: 429, ErrorCode: 10167}
		}, jetstream.ObjectMeta{}, bytes.NewReader(nil))
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
	})
}
