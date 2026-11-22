package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/nats-io/nats.go/jetstream"
)

// Each test in this file runs an operation that needs fresh JetStream
// information next to a message read through the same shared storage handle.
// nats.go writes the cached information of a handle in Info without a lock,
// and message reads use that cache. Run the tests with -race: a refresh of a
// shared handle shows as a data race.

const sharedHandleRaceRounds = 30

// runConcurrently runs refresh and read in separate goroutines, each for a
// bounded number of rounds, and waits for both.
func runConcurrently(t *testing.T, refresh, read func() error) {
	t.Helper()
	var wg sync.WaitGroup
	for _, op := range []func() error{refresh, read} {
		wg.Go(func() {
			for range sharedHandleRaceRounds {
				if err := op(); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
}

// readSharedEVT reads a message through the shared EVT handle, as the event
// reader and EventPublisher position lookups do.
func readSharedEVT(ctx context.Context, c *ChattoCore) func() error {
	return func() error {
		_, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.SetupAggregate().AllEventsFilter())
		return err
	}
}

// readSharedRuntimeState reads a key through the shared RUNTIME_STATE handle,
// as fast credential reads do.
func readSharedRuntimeState(ctx context.Context, c *ChattoCore) func() error {
	return func() error {
		_, err := c.storage.runtimeStateKV.GetAnyReplica(ctx, "missing_key")
		if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			return err
		}
		return nil
	}
}

func TestProjectionAdminStatesDoesNotRefreshSharedEVTHandle(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	runConcurrently(t, func() error {
		_, err := c.ProjectionAdminStates(ctx)
		return err
	}, readSharedEVT(ctx, c))
}

func TestNATSRecoveryCheckDoesNotRefreshSharedHandles(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	readEVT, readRuntimeState := readSharedEVT(ctx, c), readSharedRuntimeState(ctx, c)
	runConcurrently(t, func() error {
		return c.verifyNATSRecovery(ctx)
	}, func() error {
		if err := readEVT(); err != nil {
			return err
		}
		return readRuntimeState()
	})
}

func TestReadyDoesNotRefreshSharedRuntimeStateHandle(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	runConcurrently(t, func() error {
		return c.Ready(ctx)
	}, readSharedRuntimeState(ctx, c))
}

func TestListEventLogDoesNotRefreshSharedEVTHandle(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	admin, err := c.CreateUser(ctx, SystemActorID, "eventlog-admin", "Admin", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, admin.Id, PermAdminAuditView); err != nil {
		t.Fatalf("GrantUserPermission: %v", err)
	}
	runConcurrently(t, func() error {
		_, err := c.ListEventLog(ctx, admin.Id, EventLogQuery{Limit: 5})
		return err
	}, readSharedEVT(ctx, c))
}

func TestListBotWebhookFailuresDoesNotRefreshSharedLogHandle(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, bot, _ := webhookTestBot(t, c)
	endpoint, _, err := c.CreateBotOutboundWebhook(ctx, owner, bot, "Logs", "https://example.com", "", true)
	if err != nil {
		t.Fatalf("CreateBotOutboundWebhook: %v", err)
	}
	appendTestLog(t, c, bot, endpoint.ID, "entry0")
	// Each call refreshes the LOG information and then reads messages through
	// the same handle, so two concurrent calls overlap a refresh with a read.
	list := func() error {
		_, err := c.ListBotWebhookFailures(ctx, owner, bot, endpoint.ID, 2, "")
		return err
	}
	runConcurrently(t, list, list)
}

func TestAssetCleanupAdminStatusDoesNotRefreshSharedConsumerHandles(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	// Two concurrent admin requests must not both write the cached information
	// of the shared consumer handles.
	status := func() error {
		_, err := c.assetModel.AdminCleanupStatus(ctx)
		return err
	}
	runConcurrently(t, status, status)
}

// sharedStorageRefresh matches a call that refreshes a shared storage handle:
// Stream.Info, Consumer.Info, KeyValue.Status, and ObjectStore.Status or Seal.
var sharedStorageRefresh = regexp.MustCompile(`\bstorage\.\w+\.(Info|Status|Seal)\(`)

// TestNoRefreshOfSharedStorageHandles keeps production code from refreshing a
// shared storage handle. Use currentStreamInfo or a private handle instead.
// The check is narrow: it finds only direct calls on a storage field, such as
// c.storage.serverEvtStream.Info(ctx). It does not find calls through a local
// variable or a handle in another struct. The race tests above cover those
// callers.
func TestNoRefreshOfSharedStorageHandles(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(source), "\n") {
			if sharedStorageRefresh.MatchString(line) {
				t.Errorf("%s:%d refreshes a shared storage handle: %s", file, i+1, strings.TrimSpace(line))
			}
		}
	}
}
