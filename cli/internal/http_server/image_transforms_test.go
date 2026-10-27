package http_server

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"sync/atomic"
	"testing"
	"testing/synctest"
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
	synctest.Test(t, func(t *testing.T) {
		var transforms imageTransforms
		release := make(chan struct{})
		var reads, runs atomic.Int32

		const callers = 5
		results := make(chan *transformedImage, callers)
		call := func() {
			result, err := transforms.do(t.Context(), "key",
				func() ([]byte, error) {
					reads.Add(1)
					return testPNGHeader(1, 1), nil
				},
				func([]byte) (*transformedImage, error) {
					runs.Add(1)
					<-release
					return &transformedImage{Data: []byte("out"), ContentType: "image/jpeg"}, nil
				})
			if err != nil {
				result = nil
			}
			results <- result
		}
		go call()
		synctest.Wait() // The first call runs its transform.
		for range callers - 1 {
			go call()
		}
		synctest.Wait() // The other calls wait for the first.
		close(release)

		for range callers {
			if result := <-results; result == nil || string(result.Data) != "out" {
				t.Fatalf("caller got %v, want the shared result", result)
			}
		}
		if reads.Load() != 1 || runs.Load() != 1 {
			t.Fatalf("reads = %d, runs = %d, want one shared transform", reads.Load(), runs.Load())
		}
	})
}

func TestImageTransformsFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// cancelFirst ends the first request's work by cancelling its context.
		// Otherwise, the first read fails.
		cancelFirst bool
		// wantRetry is true when a waiting request does the work again.
		wantRetry bool
	}{
		{name: "first request ends", cancelFirst: true, wantRetry: true},
		{name: "first read fails", cancelFirst: false, wantRetry: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var transforms imageTransforms
				firstCtx, cancelFirst := context.WithCancel(t.Context())
				defer cancelFirst()
				failRead := make(chan struct{})
				firstErr := make(chan error, 1)
				go func() {
					_, err := transforms.do(firstCtx, "key",
						func() ([]byte, error) {
							select {
							case <-firstCtx.Done():
								return nil, firstCtx.Err()
							case <-failRead:
								return nil, errors.New("broken source")
							}
						},
						func([]byte) (*transformedImage, error) { panic("first request must not transform") })
					firstErr <- err
				}()
				synctest.Wait()

				var secondReads atomic.Int32
				secondErr := make(chan error, 1)
				var secondResult *transformedImage
				go func() {
					var err error
					secondResult, err = transforms.do(t.Context(), "key",
						func() ([]byte, error) {
							secondReads.Add(1)
							return testPNGHeader(1, 1), nil
						},
						func([]byte) (*transformedImage, error) {
							return &transformedImage{Data: []byte("second")}, nil
						})
					secondErr <- err
				}()
				synctest.Wait() // The second request waits for the first.
				if tt.cancelFirst {
					cancelFirst()
				} else {
					close(failRead)
				}

				err := <-firstErr
				if tt.cancelFirst && !errors.Is(err, context.Canceled) {
					t.Fatalf("first request error = %v, want context.Canceled", err)
				}
				err = <-secondErr
				if tt.wantRetry {
					if err != nil || secondReads.Load() != 1 || string(secondResult.Data) != "second" {
						t.Fatalf("second request = %v, %v after %d reads, want its own result", secondResult, err, secondReads.Load())
					}
					return
				}
				if err == nil || secondReads.Load() != 0 {
					t.Fatalf("second request error = %v after %d reads, want the shared error without a read", err, secondReads.Load())
				}
			})
		})
	}
}

func TestImageTransformsLimitDecodedPixels(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var transforms imageTransforms
		large := testPNGHeader(8000, 5000)
		var running atomic.Int32
		release := make(chan struct{})
		done := make(chan error, 2)
		for _, key := range []string{"a", "b"} {
			go func() {
				_, err := transforms.do(t.Context(), key,
					func() ([]byte, error) { return large, nil },
					func([]byte) (*transformedImage, error) {
						running.Add(1)
						<-release
						running.Add(-1)
						return &transformedImage{}, nil
					})
				done <- err
			}()
		}
		synctest.Wait()
		if running.Load() != 1 {
			t.Fatalf("%d largest-image transforms run together, want 1", running.Load())
		}

		// A wait for pixels ends with the context of its request.
		waitCtx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		_, err := transforms.do(waitCtx, "waiter",
			func() ([]byte, error) { return testPNGHeader(1, 1), nil },
			func([]byte) (*transformedImage, error) { return &transformedImage{}, nil })
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("waiting transform error = %v, want context.DeadlineExceeded", err)
		}

		close(release)
		for range 2 {
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		}
	})
}

// Waiting for the pixel budget must not let distinct requests accumulate an
// unbounded number of encoded source buffers.
func TestImageTransformsBoundSourcesBeforeReading(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var transforms imageTransforms
		var reads, runs atomic.Int32
		release := make(chan struct{})
		defer close(release)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		const requests = 8
		done := make(chan error, requests)
		for i := range requests {
			go func() {
				_, err := transforms.do(ctx, fmt.Sprintf("source-%d", i),
					func() ([]byte, error) {
						reads.Add(1)
						return testPNGHeader(8000, 5000), nil
					},
					func([]byte) (*transformedImage, error) {
						runs.Add(1)
						<-release
						return &transformedImage{}, nil
					})
				done <- err
			}()
		}
		synctest.Wait()
		// One large transform runs and one buffered source waits for pixels.
		// All other requests must wait before their read callback runs.
		if got := reads.Load(); got != 2 {
			t.Fatalf("read %d sources, want 2", got)
		}
		if got := runs.Load(); got != 1 {
			t.Fatalf("ran %d transforms, want 1", got)
		}
		cancel()
		for range requests - 1 {
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("waiting request error = %v, want context.Canceled", err)
			}
		}
		if got := reads.Load(); got != 2 {
			t.Fatalf("canceled waiters read sources: got %d reads, want 2", got)
		}
		// The canceled pixel waiter released its source slot even though the
		// running transform still holds the entire pixel budget.
		probeErr := errors.New("probe read")
		_, err := transforms.do(t.Context(), "probe", func() ([]byte, error) {
			return nil, probeErr
		}, func([]byte) (*transformedImage, error) { panic("probe must not transform") })
		if !errors.Is(err, probeErr) {
			t.Fatalf("probe error = %v, want released source slot", err)
		}
	})
}

func TestImageTransformsReleaseSourceSlotsOnFailure(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"read", "transform"} {
		t.Run(stage, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var transforms imageTransforms
				failure := errors.New("operation failed")
				// More failures than slots expose a leak at either stage. The
				// virtual deadline makes a leaked slot fail without a real wait.
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				for range maxConcurrentImageSources + 1 {
					_, err := transforms.do(ctx, "key", func() ([]byte, error) {
						if stage == "read" {
							return nil, failure
						}
						return testPNGHeader(1, 1), nil
					}, func([]byte) (*transformedImage, error) { return nil, failure })
					if !errors.Is(err, failure) {
						t.Fatalf("operation error = %v, want %v", err, failure)
					}
				}
				for range maxConcurrentImageSources + 1 {
					_, err := transforms.do(ctx, "success", func() ([]byte, error) {
						return testPNGHeader(1, 1), nil
					}, func([]byte) (*transformedImage, error) { return &transformedImage{}, nil })
					if err != nil {
						t.Fatalf("transform after failures: %v", err)
					}
				}
			})
		})
	}
}

func TestImageTransformsSharedFollowersDoNotHoldSourceSlots(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var transforms imageTransforms
		var reads atomic.Int32
		release := make(chan struct{})
		defer close(release)
		call := func(key string) {
			go func() {
				_, err := transforms.do(t.Context(), key, func() ([]byte, error) {
					reads.Add(1)
					return testPNGHeader(1, 1), nil
				}, func([]byte) (*transformedImage, error) {
					<-release
					return &transformedImage{}, nil
				})
				if err != nil {
					t.Errorf("transform %s: %v", key, err)
				}
			}()
		}
		call("shared")
		synctest.Wait()
		for range 3 {
			call("shared")
		}
		synctest.Wait()
		call("distinct")
		synctest.Wait()
		if got := reads.Load(); got != 2 {
			t.Fatalf("read %d sources, want shared and distinct requests to run together", got)
		}
	})
}
