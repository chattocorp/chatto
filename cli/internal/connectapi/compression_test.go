package connectapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"hmans.de/chatto/internal/config"
)

func TestHandlerOptionsForWebserver_ResponseCompression(t *testing.T) {
	tests := []struct {
		name                string
		compression         bool
		minBytes            int
		compressedRequest   bool
		wantContentEncoding string
	}{
		{
			name:                "compresses response above threshold",
			compression:         true,
			minBytes:            1024,
			wantContentEncoding: "gzip",
		},
		{
			name:        "leaves response below threshold uncompressed",
			compression: true,
			minBytes:    4096,
		},
		{
			name:              "accepts compressed request but leaves response uncompressed when disabled",
			minBytes:          0,
			compressedRequest: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			procedure := "/chatto.test.v1.CompressionService/GetPayload"
			handler := connect.NewUnaryHandler(
				procedure,
				func(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[wrapperspb.StringValue], error) {
					return connect.NewResponse(wrapperspb.String(strings.Repeat("a", 2048))), nil
				},
				HandlerOptionsForWebserver(config.WebserverConfig{
					APICompression:         &tt.compression,
					APICompressionMinBytes: &tt.minBytes,
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
