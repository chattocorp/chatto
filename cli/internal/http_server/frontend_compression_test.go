package http_server

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func brotliFrontendTestData(t testing.TB, data []byte) []byte {
	t.Helper()
	var encoded bytes.Buffer
	writer := brotli.NewWriter(&encoded)
	_, err := writer.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return encoded.Bytes()
}

func TestFrontendOnDemandGzipFallback(t *testing.T) {
	original := []byte("console.log('fallback');\n")
	source := fstest.MapFS{"app.js.br": {Data: brotliFrontendTestData(t, original)}}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			router := gin.New()
			// Static assets use middleware on unmatched URLs. Gin sets a pending
			// 404 before this middleware runs, which the asset handler must replace.
			router.Use(func(c *gin.Context) {
				require.NoError(t, serveFrontendFile(c, source, "app.js"))
				c.Abort()
			})
			router.NoRoute(func(c *gin.Context) {
				t.Fatal("asset middleware must stop the fallback chain")
			})
			request := httptest.NewRequest(method, "/app.js", nil)
			request.Header.Set("Accept-Encoding", "gzip")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
			require.Equal(t, "Accept-Encoding", response.Header().Get("Vary"))
			require.Contains(t, response.Header().Get("Content-Type"), "javascript")
			if method == http.MethodHead {
				require.Empty(t, response.Body.Bytes())
				return
			}
			reader, err := gzip.NewReader(response.Body)
			require.NoError(t, err)
			defer reader.Close()
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, original, body)
		})
	}
}

func TestFrontendOnDemandCompression(t *testing.T) {
	original := bytes.Repeat([]byte("console.log('on demand');\n"), 4096)
	encoded := brotliFrontendTestData(t, original)
	source := fstest.MapFS{"nested/app.js.br": {Data: encoded}}
	require.True(t, frontendFileExists(source, "nested/app.js"))
	for _, accept := range []string{"br, gzip", "gzip", "identity", "gzip;q=0", "br;q=0, gzip", ""} {
		t.Run(accept, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequest(http.MethodGet, "/nested/app.js", nil)
			context.Request.Header.Set("Accept-Encoding", accept)
			require.NoError(t, serveFrontendFile(context, source, "nested/app.js"))
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, "Accept-Encoding", response.Header().Get("Vary"))
			require.Contains(t, response.Header().Get("Content-Type"), "javascript")
			var reader io.Reader = response.Body
			switch accept {
			case "br, gzip":
				require.Equal(t, "br", response.Header().Get("Content-Encoding"))
				require.Equal(t, encoded, response.Body.Bytes())
				reader = brotli.NewReader(reader)
			case "gzip", "br;q=0, gzip":
				require.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
				gz, err := gzip.NewReader(reader)
				require.NoError(t, err)
				defer gz.Close()
				reader = gz
			default:
				require.Empty(t, response.Header().Get("Content-Encoding"))
			}
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, original, body)
		})
	}
	t.Run("HEAD does not decode or compress", func(t *testing.T) {
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Request = httptest.NewRequest(http.MethodHead, "/app.js", nil)
		context.Request.Header.Set("Accept-Encoding", "gzip")
		// Invalid data proves HEAD did not invoke the decoder.
		require.NoError(t, serveFrontendFile(context, fstest.MapFS{"app.js.br": {Data: []byte("invalid")}}, "app.js"))
		require.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
		require.Empty(t, response.Body.Bytes())
	})
	t.Run("corrupt Brotli returns an error", func(t *testing.T) {
		bad := fstest.MapFS{"app.js.br": {Data: []byte("invalid brotli")}}
		_, err := readFrontendIdentityFile(bad, "app.js")
		require.Error(t, err)
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Request = httptest.NewRequest(http.MethodGet, "/app.js", nil)
		context.Request.Header.Set("Accept-Encoding", "gzip")
		require.Error(t, serveFrontendFile(context, bad, "app.js"))
	})
	t.Run("response write failure is returned", func(t *testing.T) {
		context, _ := gin.CreateTestContext(failingFrontendResponse{header: make(http.Header)})
		context.Request = httptest.NewRequest(http.MethodGet, "/nested/app.js", nil)
		context.Request.Header.Set("Accept-Encoding", "gzip")
		require.ErrorIs(t, serveFrontendFile(context, source, "nested/app.js"), io.ErrClosedPipe)
	})
	t.Run("truncated Brotli cannot produce a complete gzip response", func(t *testing.T) {
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Request = httptest.NewRequest(http.MethodGet, "/app.js", nil)
		context.Request.Header.Set("Accept-Encoding", "gzip")
		bad := fstest.MapFS{"app.js.br": {Data: encoded[:len(encoded)-1]}}
		require.Error(t, serveFrontendFile(context, bad, "app.js"))
		reader, err := gzip.NewReader(response.Body)
		if err == nil {
			defer reader.Close()
			_, err = io.ReadAll(reader)
		}
		require.True(t, errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF))
	})
}

// BenchmarkFrontendOnDemandCompression compares request costs for the same
// synthetic asset. Response bytes are discarded, so allocations exclude
// httptest's full-response buffer. There is no retained asset cache.
func BenchmarkFrontendOnDemandCompression(b *testing.B) {
	original := bytes.Repeat([]byte("console.log('on demand');\n"), 4096)
	source := fstest.MapFS{"app.js.br": {Data: brotliFrontendTestData(b, original)}}
	for _, encoding := range []string{"br", "gzip", "identity"} {
		b.Run(encoding, func(b *testing.B) {
			context, _ := gin.CreateTestContext(discardFrontendResponse{header: make(http.Header)})
			context.Request = httptest.NewRequest(http.MethodGet, "/app.js", nil)
			context.Request.Header.Set("Accept-Encoding", encoding)
			b.ReportAllocs()
			for b.Loop() {
				if err := serveFrontendFile(context, source, "app.js"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

type discardFrontendResponse struct{ header http.Header }

func (w discardFrontendResponse) Header() http.Header          { return w.header }
func (discardFrontendResponse) WriteHeader(int)                {}
func (discardFrontendResponse) Write(data []byte) (int, error) { return len(data), nil }

type failingFrontendResponse struct{ header http.Header }

func (w failingFrontendResponse) Header() http.Header     { return w.header }
func (failingFrontendResponse) WriteHeader(int)           {}
func (failingFrontendResponse) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
