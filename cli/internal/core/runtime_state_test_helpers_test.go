package core

import (
	"testing"

	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
	"github.com/nats-io/nats.go/jetstream"
)

// newRuntimeStateTestKV creates the production bucket with leader-routed reads.
// Component tests can use it without starting unrelated core services. Each
// test owns its server, bucket, and cleanup.
func newRuntimeStateTestKV(t *testing.T) *jetstreamutil.KeyValue {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}
	kv, err := createKeyValue(testContext(t), js, runtimeStateConfig(testCoreConfig()))
	if err != nil {
		t.Fatalf("create runtime-state bucket: %v", err)
	}
	return kv
}
