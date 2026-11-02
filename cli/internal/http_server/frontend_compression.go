package http_server

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"

	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
)

// serveFrontendGzip streams a Brotli asset through a gzip encoder only when
// gzip is accepted and no stored representation was served. It keeps no cache
// and never retains the complete decoded asset. HEAD needs only the headers.
// On a streaming error, do not finish the gzip trailer: clients must detect
// the incomplete response instead of accepting partial content as valid gzip.
func serveFrontendGzip(c *gin.Context, source fs.FS, name string) (bool, error) {
	if !clientAcceptsEncoding(c.GetHeader("Accept-Encoding"), "gzip") {
		return false, nil
	}
	input, err := source.Open(name + ".br")
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer input.Close()
	c.Header("Content-Encoding", "gzip")
	c.Header("Content-Type", frontendContentType(name))
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return true, nil
	}
	writer := gzip.NewWriter(c.Writer)
	if _, err := io.Copy(writer, brotli.NewReader(input)); err != nil {
		return true, fmt.Errorf("transcode frontend asset %q: %w", name, err)
	}
	return true, writer.Close()
}
