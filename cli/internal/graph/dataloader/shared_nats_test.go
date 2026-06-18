package dataloader_test

import (
	"os"
	"testing"

	"hmans.de/chatto/internal/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.ShutdownSharedNATS()
	os.Exit(code)
}
