package http_server

import (
	"os"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.ShutdownSharedNATS()
	os.Exit(code)
}
