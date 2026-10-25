package http_server

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/assets"
)

// testPNGHeader returns a PNG signature and IHDR chunk for the dimensions.
// InspectImage reads only these bytes.
func testPNGHeader(width, height uint32) []byte {
	var out bytes.Buffer
	out.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	data := make([]byte, 13)
	binary.BigEndian.PutUint32(data[0:4], width)
	binary.BigEndian.PutUint32(data[4:8], height)
	data[8], data[9] = 8, 6
	_ = binary.Write(&out, binary.BigEndian, uint32(len(data)))
	out.WriteString("IHDR")
	out.Write(data)
	_ = binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(append([]byte("IHDR"), data...)))
	return out.Bytes()
}

func TestTransformPixelWeight(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data []byte
		want int64
	}{
		{"static image", testPNGHeader(20, 10), 200},
		{"largest image", testPNGHeader(8000, 5000), assets.MaxDecodedImagePixels},
		{"rejected image", []byte("not an image"), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := transformPixelWeight(tt.data); got != tt.want {
				t.Fatalf("transformPixelWeight() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestImageTransformsShareConcurrentWork(t *testing.T) {
	t.Parallel()
	var transforms imageTransforms
	ctx := t.Context()
	release := make(chan struct{})
	var reads, runs atomic.Int32

	const callers = 5
	var wg sync.WaitGroup
	results := make([]*transformedImage, callers)
	errs := make([]error, callers)
	for i := range callers {
		wg.Go(func() {
			results[i], errs[i] = transforms.do(ctx, "key",
				func() ([]byte, error) {
					reads.Add(1)
					return testPNGHeader(1, 1), nil
				},
				func([]byte) (*transformedImage, error) {
					runs.Add(1)
					<-release
					return &transformedImage{Data: []byte("out"), ContentType: "image/jpeg"}, nil
				})
		})
	}
	// Wait until the first transform runs. singleflight does not show when
	// the other callers join, so give them time before the transform ends.
	waitFor(t, func() bool { return runs.Load() == 1 })
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()

	if reads.Load() != 1 || runs.Load() != 1 {
		t.Fatalf("reads = %d, runs = %d, want one shared transform", reads.Load(), runs.Load())
	}
	for i := range callers {
		if errs[i] != nil || string(results[i].Data) != "out" {
			t.Fatalf("caller %d got %v, %v", i, results[i], errs[i])
		}
	}
}

func TestImageTransformsRetryAfterCancelledFirstRequest(t *testing.T) {
	t.Parallel()
	var transforms imageTransforms
	firstCtx, cancelFirst := context.WithCancel(t.Context())
	firstStarted := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := transforms.do(firstCtx, "key",
			func() ([]byte, error) {
				close(firstStarted)
				<-firstCtx.Done()
				return nil, firstCtx.Err()
			},
			func([]byte) (*transformedImage, error) { panic("first request must not transform") })
		firstDone <- err
	}()
	<-firstStarted

	secondDone := make(chan error, 1)
	var secondResult *transformedImage
	go func() {
		var err error
		secondResult, err = transforms.do(t.Context(), "key",
			func() ([]byte, error) { return testPNGHeader(1, 1), nil },
			func([]byte) (*transformedImage, error) {
				return &transformedImage{Data: []byte("second")}, nil
			})
		secondDone <- err
	}()
	// Let the second request join the first before the first fails.
	time.Sleep(100 * time.Millisecond)
	cancelFirst()

	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("first request error = %v, want context.Canceled", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second request error = %v", err)
	}
	if string(secondResult.Data) != "second" {
		t.Fatalf("second request result = %q", secondResult.Data)
	}
}

func TestImageTransformsLimitDecodedPixels(t *testing.T) {
	t.Parallel()
	var transforms imageTransforms
	ctx := t.Context()
	large := testPNGHeader(8000, 5000)
	var running, peak atomic.Int32
	release := make(chan struct{})

	var wg sync.WaitGroup
	for _, key := range []string{"a", "b"} {
		wg.Go(func() {
			_, err := transforms.do(ctx, key,
				func() ([]byte, error) { return large, nil },
				func([]byte) (*transformedImage, error) {
					n := running.Add(1)
					for {
						p := peak.Load()
						if n <= p || peak.CompareAndSwap(p, n) {
							break
						}
					}
					<-release
					running.Add(-1)
					return &transformedImage{}, nil
				})
			if err != nil {
				t.Error(err)
			}
		})
	}
	waitFor(t, func() bool { return running.Load() == 1 })
	// The other transform must not start while the first holds all pixels.
	time.Sleep(100 * time.Millisecond)
	if running.Load() != 1 {
		t.Fatalf("%d largest-image transforms run together, want 1", running.Load())
	}
	close(release)
	wg.Wait()
	if peak.Load() != 1 {
		t.Fatalf("peak concurrent largest-image transforms = %d, want 1", peak.Load())
	}

	// A cancelled wait for pixels returns the context error.
	held := make(chan struct{})
	releaseHolder := make(chan struct{})
	defer close(releaseHolder)
	go func() {
		_, _ = transforms.do(ctx, "holder",
			func() ([]byte, error) { return large, nil },
			func([]byte) (*transformedImage, error) {
				close(held)
				<-releaseHolder
				return &transformedImage{}, nil
			})
	}()
	<-held
	waitCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	_, err := transforms.do(waitCtx, "waiter",
		func() ([]byte, error) { return testPNGHeader(1, 1), nil },
		func([]byte) (*transformedImage, error) { return &transformedImage{}, nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting transform error = %v, want context.DeadlineExceeded", err)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met before deadline")
		}
		time.Sleep(time.Millisecond)
	}
}
