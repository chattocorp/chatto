package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	clientsyncv1 "hmans.de/chatto/internal/pb/chatto/clientsync/v1"
)

func TestClientSyncPreferencesRoundTrip(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createClientSyncTestUser(t, chatto, ctx, "preferences")

	empty, err := chatto.ClientSync.GetPreferences(ctx, userID)
	if err != nil {
		t.Fatalf("GetPreferences empty: %v", err)
	}
	if empty.Locale != nil || empty.Timezone != nil || empty.TimeFormat != nil {
		t.Fatalf("empty preferences = %v", empty)
	}

	locale := "en-GB"
	timezone := "Europe/Berlin"
	format := clientsyncv1.TimeFormat_TIME_FORMAT_24_HOUR
	updated, err := chatto.ClientSync.UpdatePreferences(ctx, userID, func(preferences *clientsyncv1.Preferences) error {
		preferences.Locale = &locale
		preferences.Timezone = &timezone
		preferences.TimeFormat = &format
		return nil
	})
	if err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	if updated.GetLocale() != locale || updated.GetTimezone() != timezone || updated.GetTimeFormat() != format {
		t.Fatalf("updated preferences = %v", updated)
	}

	stored, err := chatto.ClientSync.GetPreferences(ctx, userID)
	if err != nil {
		t.Fatalf("GetPreferences stored: %v", err)
	}
	if stored.GetLocale() != locale || stored.GetTimezone() != timezone || stored.GetTimeFormat() != format {
		t.Fatalf("stored preferences = %v", stored)
	}
}

func TestClientSyncServerDirectoryProtectsHomeServer(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createClientSyncTestUser(t, chatto, ctx, "directory")
	serverA := &clientsyncv1.KnownServer{Id: "a", Url: "https://a.example", Name: "A", AddedAt: timestamppb.Now()}
	serverB := &clientsyncv1.KnownServer{Id: "b", Url: "https://b.example", Name: "B", AddedAt: timestamppb.Now()}

	if _, err := chatto.ClientSync.CreateServer(ctx, userID, serverA); err != nil {
		t.Fatalf("CreateServer a: %v", err)
	}
	directory, err := chatto.ClientSync.ListServers(ctx, userID)
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if directory.GetHomeServerId() != "a" {
		t.Fatalf("first server home = %q, want a", directory.GetHomeServerId())
	}
	if err := chatto.ClientSync.DeleteServer(ctx, userID, "a"); !errors.Is(err, ErrCannotDeleteHomeServer) {
		t.Fatalf("DeleteServer home err = %v, want ErrCannotDeleteHomeServer", err)
	}

	if _, err := chatto.ClientSync.CreateServer(ctx, userID, serverB); err != nil {
		t.Fatalf("CreateServer b: %v", err)
	}
	if _, err := chatto.ClientSync.SetHomeServer(ctx, userID, "b"); err != nil {
		t.Fatalf("SetHomeServer b: %v", err)
	}
	if err := chatto.ClientSync.DeleteServer(ctx, userID, "a"); err != nil {
		t.Fatalf("DeleteServer a after move: %v", err)
	}
	directory, err = chatto.ClientSync.ListServers(ctx, userID)
	if err != nil {
		t.Fatalf("ListServers final: %v", err)
	}
	if directory.GetHomeServerId() != "b" || len(directory.GetServers()) != 1 || directory.GetServers()[0].GetId() != "b" {
		t.Fatalf("final directory = %v", directory)
	}
}

func TestClientSyncRejectsDuplicateServerID(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createClientSyncTestUser(t, chatto, ctx, "duplicate-id")
	server := &clientsyncv1.KnownServer{Id: "same", Url: "https://one.example", Name: "One"}
	if _, err := chatto.ClientSync.CreateServer(ctx, userID, server); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if _, err := chatto.ClientSync.CreateServer(ctx, userID, server); !errors.Is(err, ErrKnownServerAlreadyExists) {
		t.Fatalf("duplicate err = %v, want ErrKnownServerAlreadyExists", err)
	}
}

func TestClientSyncRejectsDuplicateServerURL(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createClientSyncTestUser(t, chatto, ctx, "duplicate-url")
	first := &clientsyncv1.KnownServer{Id: "first", Url: "https://same.example", Name: "First"}
	duplicate := &clientsyncv1.KnownServer{Id: "second", Url: first.Url, Name: "Second"}
	if _, err := chatto.ClientSync.CreateServer(ctx, userID, first); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if _, err := chatto.ClientSync.CreateServer(ctx, userID, duplicate); !errors.Is(err, ErrKnownServerAlreadyExists) {
		t.Fatalf("duplicate URL err = %v, want ErrKnownServerAlreadyExists", err)
	}

	second := &clientsyncv1.KnownServer{Id: "second", Url: "https://other.example", Name: "Second"}
	if _, err := chatto.ClientSync.CreateServer(ctx, userID, second); err != nil {
		t.Fatalf("CreateServer second: %v", err)
	}
	if _, err := chatto.ClientSync.UpdateServer(ctx, userID, second.Id, func(server *clientsyncv1.KnownServer) error {
		server.Url = first.Url
		return nil
	}); !errors.Is(err, ErrKnownServerAlreadyExists) {
		t.Fatalf("duplicate URL update err = %v, want ErrKnownServerAlreadyExists", err)
	}
}

func TestClientSyncServerDirectoryLimit(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createClientSyncTestUser(t, chatto, ctx, "limit")
	directory := &clientsyncv1.ServerDirectory{Servers: make([]*clientsyncv1.KnownServer, 0, MaxClientSyncKnownServers)}
	for i := range MaxClientSyncKnownServers {
		directory.Servers = append(directory.Servers, &clientsyncv1.KnownServer{
			Id:  fmt.Sprintf("server-%d", i),
			Url: fmt.Sprintf("https://server-%d.example", i),
		})
	}
	data, err := proto.Marshal(directory)
	if err != nil {
		t.Fatalf("Marshal directory: %v", err)
	}
	if _, err := chatto.storage.runtimeStateKV.Put(ctx, clientSyncServerDirectoryKey(userID), data); err != nil {
		t.Fatalf("Put directory: %v", err)
	}

	_, err = chatto.ClientSync.CreateServer(ctx, userID, &clientsyncv1.KnownServer{Id: "overflow", Url: "https://overflow.example"})
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("CreateServer overflow error = %v, want ErrLimitExceeded", err)
	}
}

func TestClientSyncServerDirectoryLimitUnderConcurrentCreates(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createClientSyncTestUser(t, chatto, ctx, "concurrent-limit")
	directory := &clientsyncv1.ServerDirectory{Servers: make([]*clientsyncv1.KnownServer, 0, MaxClientSyncKnownServers-1)}
	for i := range MaxClientSyncKnownServers - 1 {
		directory.Servers = append(directory.Servers, &clientsyncv1.KnownServer{
			Id:  fmt.Sprintf("server-%d", i),
			Url: fmt.Sprintf("https://server-%d.example", i),
		})
	}
	data, err := proto.Marshal(directory)
	if err != nil {
		t.Fatalf("Marshal directory: %v", err)
	}
	if _, err := chatto.storage.runtimeStateKV.Put(ctx, clientSyncServerDirectoryKey(userID), data); err != nil {
		t.Fatalf("Put directory: %v", err)
	}

	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := chatto.ClientSync.CreateServer(ctx, userID, &clientsyncv1.KnownServer{
				Id:  fmt.Sprintf("concurrent-%d", i),
				Url: fmt.Sprintf("https://concurrent-%d.example", i),
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	var succeeded, limited int
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrLimitExceeded):
			limited++
		default:
			t.Fatalf("CreateServer error = %v", err)
		}
	}
	if succeeded != 1 || limited != 1 {
		t.Fatalf("concurrent results: succeeded=%d limited=%d, want 1 each", succeeded, limited)
	}
}

func TestClientSyncRejectsMutationAfterAccountDeletion(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createClientSyncTestUser(t, chatto, ctx, "deleted")
	if err := chatto.DeleteUser(ctx, SystemActorID, userID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	_, err := chatto.ClientSync.UpdatePreferences(ctx, userID, func(preferences *clientsyncv1.Preferences) error {
		locale := "en-GB"
		preferences.Locale = &locale
		return nil
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdatePreferences after deletion error = %v, want ErrNotFound", err)
	}
	for _, key := range []string{clientSyncPreferencesKey(userID), clientSyncServerDirectoryKey(userID)} {
		if _, err := chatto.storage.runtimeStateKV.Get(ctx, key); !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
			t.Fatalf("Get(%q) after deletion error = %v, want absent", key, err)
		}
	}
}

func TestClientSyncDeleteUserAttemptsEveryRecord(t *testing.T) {
	kv := &failingClientSyncKV{purgeErrors: map[string]error{
		clientSyncPreferencesKey("user"):     errors.New("preferences unavailable"),
		clientSyncServerDirectoryKey("user"): errors.New("directory unavailable"),
	}}
	service := NewClientSyncService(kv, nil)

	err := service.DeleteUser(context.Background(), "user")
	if err == nil || !stringsContainAll(err.Error(), "preferences unavailable", "directory unavailable") {
		t.Fatalf("DeleteUser error = %v, want both purge failures", err)
	}
	if !slices.Contains(kv.purged, clientSyncPreferencesKey("user")) || !slices.Contains(kv.purged, clientSyncServerDirectoryKey("user")) {
		t.Fatalf("purged keys = %v, want both user records attempted", kv.purged)
	}
}

func TestClientSyncDeletionMarkerWinsAfterMutationValidation(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createClientSyncTestUser(t, chatto, ctx, "deletion-race")
	beforeCreate := make(chan struct{})
	resumeCreate := make(chan struct{})
	kv := &pausingClientSyncKV{
		KeyValue:     chatto.storage.runtimeStateKV,
		key:          clientSyncPreferencesKey(userID),
		beforeCreate: beforeCreate,
		resumeCreate: resumeCreate,
	}
	service := NewClientSyncService(kv, nil)

	mutationDone := make(chan error, 1)
	go func() {
		_, err := service.UpdatePreferences(ctx, userID, func(preferences *clientsyncv1.Preferences) error {
			locale := "en-GB"
			preferences.Locale = &locale
			return nil
		})
		mutationDone <- err
	}()
	<-beforeCreate
	if err := service.DeleteUser(ctx, userID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	close(resumeCreate)
	if err := <-mutationDone; !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdatePreferences error = %v, want ErrNotFound", err)
	}
	if _, err := chatto.storage.runtimeStateKV.Get(ctx, clientSyncPreferencesKey(userID)); !isClientSyncKeyAbsent(err) {
		t.Fatalf("preferences after deletion race error = %v, want absent", err)
	}
}

func TestClientSyncRecoversTransientDeletionFailure(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createClientSyncTestUser(t, chatto, ctx, "deletion-recovery")
	if _, err := chatto.ClientSync.UpdatePreferences(ctx, userID, func(preferences *clientsyncv1.Preferences) error {
		locale := "en-GB"
		preferences.Locale = &locale
		return nil
	}); err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	kv := &transientPurgeClientSyncKV{
		KeyValue: chatto.storage.runtimeStateKV,
		failKey:  clientSyncPreferencesKey(userID),
	}
	service := NewClientSyncService(kv, nil)
	if err := service.DeleteUser(ctx, userID); err == nil {
		t.Fatal("DeleteUser succeeded, want transient purge failure")
	}
	if err := service.RecoverPendingDeletions(ctx); err != nil {
		t.Fatalf("RecoverPendingDeletions: %v", err)
	}
	if _, err := chatto.storage.runtimeStateKV.Get(ctx, clientSyncPreferencesKey(userID)); !isClientSyncKeyAbsent(err) {
		t.Fatalf("preferences after recovery error = %v, want absent", err)
	}
}

func TestClientSyncRecoveryDoesNotPurgePreparedActiveAccount(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createClientSyncTestUser(t, chatto, ctx, "prepared-recovery")
	if _, err := chatto.ClientSync.UpdatePreferences(ctx, userID, func(preferences *clientsyncv1.Preferences) error {
		locale := "en-GB"
		preferences.Locale = &locale
		return nil
	}); err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	fence, err := chatto.ClientSync.BeginDeleteUser(ctx, userID)
	if err != nil {
		t.Fatalf("BeginDeleteUser: %v", err)
	}
	if err := chatto.ClientSync.RecoverPendingDeletions(ctx); err != nil {
		t.Fatalf("RecoverPendingDeletions: %v", err)
	}
	if _, err := chatto.storage.runtimeStateKV.Get(ctx, clientSyncPreferencesKey(userID)); err != nil {
		t.Fatalf("prepared active preferences were purged: %v", err)
	}
	if err := chatto.ClientSync.CancelDeleteUser(context.WithoutCancel(ctx), fence); err != nil {
		t.Fatalf("CancelDeleteUser: %v", err)
	}
}

func TestClientSyncConcurrentDeletionCannotCancelOwnerFence(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createClientSyncTestUser(t, chatto, ctx, "concurrent-delete")
	ownerFence, err := chatto.ClientSync.BeginDeleteUser(ctx, userID)
	if err != nil {
		t.Fatalf("first BeginDeleteUser: %v", err)
	}
	loserFence, err := chatto.ClientSync.BeginDeleteUser(ctx, userID)
	if !errors.Is(err, errClientSyncDeletionInProgress) || loserFence != nil {
		t.Fatalf("second BeginDeleteUser = (%v, %v), want owned conflict", loserFence, err)
	}
	if _, err := chatto.storage.runtimeStateKV.Get(ctx, clientSyncDeletionPreparingKey(userID)); err != nil {
		t.Fatalf("owner fence missing after concurrent attempt: %v", err)
	}
	if err := chatto.ClientSync.CancelDeleteUser(ctx, ownerFence); err != nil {
		t.Fatalf("owner CancelDeleteUser: %v", err)
	}
}

func TestClientSyncRecoveryRollsBackStalePreparationForActiveAccount(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createClientSyncTestUser(t, chatto, ctx, "stale-preparation")
	if _, err := chatto.ClientSync.BeginDeleteUser(ctx, userID); err != nil {
		t.Fatalf("BeginDeleteUser: %v", err)
	}
	key := clientSyncDeletionPreparingKey(userID)
	entry, err := chatto.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get pending marker: %v", err)
	}
	staleData, err := proto.Marshal(timestamppb.New(time.Now().Add(-2 * clientSyncDeletionPreparationTTL)))
	if err != nil {
		t.Fatalf("marshal stale marker: %v", err)
	}
	if _, err := chatto.storage.runtimeStateKV.Update(ctx, key, staleData, entry.Revision()); err != nil {
		t.Fatalf("age pending marker: %v", err)
	}
	if err := chatto.ClientSync.RecoverPendingDeletions(ctx); err != nil {
		t.Fatalf("RecoverPendingDeletions: %v", err)
	}
	if _, err := chatto.storage.runtimeStateKV.Get(ctx, key); !isClientSyncKeyAbsent(err) {
		t.Fatalf("stale preparation error = %v, want absent", err)
	}
}

func TestClientSyncCommittedCleanupDoesNotDependOnPreparation(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createClientSyncTestUser(t, chatto, ctx, "expired-commit")
	kv := &transientPurgeClientSyncKV{
		KeyValue: chatto.storage.runtimeStateKV,
		failKey:  clientSyncPreferencesKey(userID),
	}
	service := NewClientSyncService(kv, nil)
	fence, err := service.BeginDeleteUser(ctx, userID)
	if err != nil {
		t.Fatalf("BeginDeleteUser: %v", err)
	}
	if err := kv.Purge(ctx, clientSyncDeletionPreparingKey(userID)); err != nil {
		t.Fatalf("expire preparation: %v", err)
	}
	if err := service.completeUserDeletion(ctx, userID, fence); err == nil {
		t.Fatal("completeUserDeletion succeeded, want transient cleanup failure")
	}
	if _, err := chatto.storage.runtimeStateKV.Get(ctx, clientSyncDeletionPendingKey(userID)); err != nil {
		t.Fatalf("post-commit cleanup marker missing after preparation expiry: %v", err)
	}
}

func TestClientSyncMutationRechecksAccountBeforeWrite(t *testing.T) {
	kv := &failingClientSyncKV{}
	validations := 0
	service := NewClientSyncService(kv, func(context.Context, string) error {
		validations++
		if validations > 1 {
			return ErrNotFound
		}
		return nil
	})

	_, err := service.UpdatePreferences(context.Background(), "user", func(preferences *clientsyncv1.Preferences) error {
		locale := "en-GB"
		preferences.Locale = &locale
		return nil
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdatePreferences error = %v, want ErrNotFound", err)
	}
	if kv.creates != 0 {
		t.Fatalf("Create calls = %d, want 0 after account became unavailable", kv.creates)
	}
}

func TestDeleteUserSurfacesClientSyncCleanupFailure(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createClientSyncTestUser(t, chatto, ctx, "cleanup-failure")
	kv := &failingClientSyncKV{purgeErrors: map[string]error{
		clientSyncPreferencesKey(userID): errors.New("storage unavailable"),
	}}
	chatto.ClientSync = NewClientSyncService(kv, nil)

	err := chatto.DeleteUser(ctx, SystemActorID, userID)
	if err == nil || !strings.Contains(err.Error(), "client sync cleanup failed") {
		t.Fatalf("DeleteUser error = %v, want surfaced client sync cleanup failure", err)
	}
	if _, getErr := chatto.GetUser(ctx, userID); !errors.Is(getErr, ErrNotFound) {
		t.Fatalf("GetUser after deletion error = %v, want ErrNotFound", getErr)
	}
}

func createClientSyncTestUser(t *testing.T, chatto *ChattoCore, ctx context.Context, suffix string) string {
	t.Helper()
	user, err := chatto.CreateUser(ctx, SystemActorID, "client-sync-"+suffix, "Client Sync", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return user.GetId()
}

type failingClientSyncKV struct {
	mu          sync.Mutex
	purged      []string
	purgeErrors map[string]error
	creates     int
}

func (*failingClientSyncKV) Get(context.Context, string) (jetstream.KeyValueEntry, error) {
	return nil, jetstream.ErrKeyNotFound
}

func (kv *failingClientSyncKV) Create(_ context.Context, key string, _ []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	if strings.HasSuffix(key, ".deleted") || strings.HasSuffix(key, ".deletion_pending") || strings.HasSuffix(key, ".deletion_preparing") {
		return 1, nil
	}
	kv.creates++
	return 0, errors.New("unexpected Create")
}

func (*failingClientSyncKV) ListKeysFiltered(context.Context, ...string) (jetstream.KeyLister, error) {
	return nil, jetstream.ErrNoKeysFound
}

func (*failingClientSyncKV) Update(context.Context, string, []byte, uint64) (uint64, error) {
	return 0, errors.New("unexpected Update")
}

func (kv *failingClientSyncKV) Purge(_ context.Context, key string, _ ...jetstream.KVDeleteOpt) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.purged = append(kv.purged, key)
	return kv.purgeErrors[key]
}

type pausingClientSyncKV struct {
	jetstream.KeyValue
	key          string
	beforeCreate chan struct{}
	resumeCreate chan struct{}
}

func (kv *pausingClientSyncKV) Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	if key == kv.key {
		close(kv.beforeCreate)
		<-kv.resumeCreate
	}
	return kv.KeyValue.Create(ctx, key, value, opts...)
}

type transientPurgeClientSyncKV struct {
	jetstream.KeyValue
	mu      sync.Mutex
	failKey string
	failed  bool
}

func (kv *transientPurgeClientSyncKV) Purge(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	kv.mu.Lock()
	if key == kv.failKey && !kv.failed {
		kv.failed = true
		kv.mu.Unlock()
		return errors.New("transient storage failure")
	}
	kv.mu.Unlock()
	return kv.KeyValue.Purge(ctx, key, opts...)
}

func stringsContainAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
