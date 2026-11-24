package connectapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/config"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestHandlerOptionsForWebserver_ResponseCompression(t *testing.T) {
	tests := []struct {
		name                string
		compression         *bool
		minBytes            *int
		payloadBytes        int
		compressedRequest   bool
		wantContentEncoding string
	}{
		{
			name:         "default leaves small response uncompressed",
			payloadBytes: 512,
		},
		{
			name:                "default compresses large response",
			payloadBytes:        2048,
			wantContentEncoding: "gzip",
		},
		{
			name:                "compresses response above threshold",
			compression:         new(true),
			minBytes:            new(1024),
			payloadBytes:        2048,
			wantContentEncoding: "gzip",
		},
		{
			name:         "leaves response below threshold uncompressed",
			compression:  new(true),
			minBytes:     new(4096),
			payloadBytes: 2048,
		},
		{
			name:              "accepts compressed request but leaves response uncompressed when disabled",
			compression:       new(false),
			minBytes:          new(0),
			payloadBytes:      2048,
			compressedRequest: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			procedure := "/chatto.test.v1.CompressionService/GetPayload"
			handler := connect.NewUnaryHandler(
				procedure,
				func(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[wrapperspb.StringValue], error) {
					return connect.NewResponse(wrapperspb.String(strings.Repeat("a", tt.payloadBytes))), nil
				},
				HandlerOptionsForWebserver(config.WebserverConfig{
					APICompression:         tt.compression,
					APICompressionMinBytes: tt.minBytes,
				})...,
			)
			server := httptest.NewServer(handler)
			t.Cleanup(server.Close)

			var body bytes.Buffer
			if tt.compressedRequest {
				writer := gzip.NewWriter(&body)
				if _, err := writer.Write([]byte("{}")); err != nil {
					t.Fatalf("compressing request: %v", err)
				}
				if err := writer.Close(); err != nil {
					t.Fatalf("closing request compressor: %v", err)
				}
			} else {
				body.WriteString("{}")
			}
			request, err := http.NewRequest(http.MethodPost, server.URL+procedure, &body)
			if err != nil {
				t.Fatalf("NewRequest() error = %v", err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Connect-Protocol-Version", "1")
			request.Header.Set("Accept-Encoding", "gzip")
			if tt.compressedRequest {
				request.Header.Set("Content-Encoding", "gzip")
			}

			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatalf("Do() error = %v", err)
			}
			t.Cleanup(func() { response.Body.Close() })
			if _, err := io.Copy(io.Discard, response.Body); err != nil {
				t.Fatalf("reading response body: %v", err)
			}
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
			}
			if got := response.Header.Get("Content-Encoding"); got != tt.wantContentEncoding {
				t.Errorf("Content-Encoding = %q, want %q", got, tt.wantContentEncoding)
			}
		})
	}
}

// newPayloadHandlers returns one unary handler for each of count procedures.
// Each handler gets its own options from newOptions, as Chatto's generated
// service constructors do for each procedure.
func newPayloadHandlers(count, payloadBytes int, newOptions func() []connect.HandlerOption) ([]string, []http.Handler) {
	payload := strings.Repeat("chatto room timeline entry ", payloadBytes/27+1)[:payloadBytes]
	procedures := make([]string, count)
	handlers := make([]http.Handler, count)
	for i := range count {
		procedures[i] = fmt.Sprintf("/chatto.test.v1.CompressionService/GetPayload%d", i)
		handlers[i] = connect.NewUnaryHandler(
			procedures[i],
			func(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[wrapperspb.StringValue], error) {
				return connect.NewResponse(wrapperspb.String(payload)), nil
			},
			newOptions()...,
		)
	}
	return procedures, handlers
}

// serveGzipAcceptingRequest sends one Connect JSON request that accepts gzip
// and returns the Content-Encoding of the response.
func serveGzipAcceptingRequest(tb testing.TB, handler http.Handler, procedure string) string {
	tb.Helper()
	request := httptest.NewRequest(http.MethodPost, procedure, strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	request.Header.Set("Accept-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		tb.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	return recorder.Header().Get("Content-Encoding")
}

func TestHandlerOptionsForWebserver_ReusesGzipWritersAcrossProceduresAndCollections(t *testing.T) {
	const procedureCount = 32
	procedures, handlers := newPayloadHandlers(procedureCount, 2048, func() []connect.HandlerOption {
		return HandlerOptionsForWebserver(config.WebserverConfig{})
	})
	// Make sure that the free list has an idle writer.
	if got := serveGzipAcceptingRequest(t, handlers[0], procedures[0]); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := range procedureCount {
		// A sync.Pool drops its entries after two collections.
		runtime.GC()
		runtime.GC()
		if got := serveGzipAcceptingRequest(t, handlers[i], procedures[i]); got != "gzip" {
			t.Fatalf("procedure %d: Content-Encoding = %q, want gzip", i, got)
		}
	}
	runtime.ReadMemStats(&after)

	// A new gzip writer allocates about 1 MiB. With Connect's pool for each
	// procedure, these calls allocate one writer for each procedure (about
	// 32 MiB). With the shared free list, they reuse an idle writer.
	const maxBytes = 2 << 20
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > maxBytes {
		t.Fatalf("compressed responses from %d procedures allocated %d bytes, want at most %d", procedureCount, allocated, maxBytes)
	}
}

func TestPooledGzipWriter_ConcurrentStreamsDecode(t *testing.T) {
	const streams = 3 * idleGzipWriterLimit
	var wg sync.WaitGroup
	errs := make(chan error, streams)
	for i := range streams {
		wg.Go(func() {
			want := strings.Repeat(fmt.Sprintf("stream %d ", i), 500)
			for range 20 {
				var dst bytes.Buffer
				writer := &pooledGzipWriter{dst: io.Discard}
				writer.Reset(&dst)
				if _, err := writer.Write([]byte(want)); err != nil {
					errs <- err
					return
				}
				if err := writer.Close(); err != nil {
					errs <- err
					return
				}
				writer.Reset(io.Discard)
				reader, err := gzip.NewReader(&dst)
				if err != nil {
					errs <- err
					return
				}
				got, err := io.ReadAll(reader)
				if err != nil {
					errs <- err
					return
				}
				if string(got) != want {
					errs <- fmt.Errorf("stream %d decoded %d bytes, want %d", i, len(got), len(want))
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if idle := len(idleGzipWriters); idle > idleGzipWriterLimit {
		t.Errorf("idle gzip writers = %d, want at most %d", idle, idleGzipWriterLimit)
	}
}

func TestPooledGzipWriter_CloseWithoutWriteWritesEmptyStream(t *testing.T) {
	var dst bytes.Buffer
	writer := &pooledGzipWriter{dst: io.Discard}
	writer.Reset(&dst)
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	reader, err := gzip.NewReader(&dst)
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	got, err := io.ReadAll(reader)
	if err != nil || len(got) != 0 {
		t.Fatalf("ReadAll() = %q, %v; want empty stream", got, err)
	}
}

// BenchmarkCompressedResponseAcrossProcedures measures one gzip-accepting
// request to each of 32 procedures, with a garbage collection between rounds
// (outside the timer), as on a server where most procedures are called less
// often than the collector runs. "connect-default" is Connect's gzip pool for
// each procedure, which Chatto used before the shared free list.
func BenchmarkCompressedResponseAcrossProcedures(b *testing.B) {
	const procedureCount = 32
	pools := []struct {
		name       string
		newOptions func() []connect.HandlerOption
	}{
		{"shared-free-list", func() []connect.HandlerOption {
			return HandlerOptionsForWebserver(config.WebserverConfig{})
		}},
		{"connect-default", func() []connect.HandlerOption {
			// A later WithCompression replaces the shared gzip registration. A
			// new option for each handler gives each procedure its own pool, as
			// Connect's default gzip support does.
			return append(HandlerOptionsForWebserver(config.WebserverConfig{}), connect.WithCompression(
				"gzip",
				func() connect.Decompressor { return &gzip.Reader{} },
				func() connect.Compressor { return gzip.NewWriter(io.Discard) },
			))
		}},
	}
	for _, payloadBytes := range []int{512, 2048, 16384} {
		for _, pool := range pools {
			b.Run(fmt.Sprintf("%s/payload=%d", pool.name, payloadBytes), func(b *testing.B) {
				procedures, handlers := newPayloadHandlers(procedureCount, payloadBytes, pool.newOptions)
				b.ReportAllocs()
				for b.Loop() {
					for i := range procedureCount {
						serveGzipAcceptingRequest(b, handlers[i], procedures[i])
					}
					b.StopTimer()
					runtime.GC()
					b.StartTimer()
				}
			})
		}
	}
}
