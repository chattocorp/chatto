package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/jetstreamutil"
	clientsyncv1 "hmans.de/chatto/internal/pb/chatto/clientsync/v1"
)

const clientSyncMutationRetries = 5

// MaxClientSyncKnownServers bounds one user's portable directory so a single
// latest-value KV record cannot grow without limit.
const MaxClientSyncKnownServers = 100

type clientSyncKV interface {
	Get(context.Context, string) (jetstream.KeyValueEntry, error)
	Create(context.Context, string, []byte, ...jetstream.KVCreateOpt) (uint64, error)
	Update(context.Context, string, []byte, uint64) (uint64, error)
	Purge(context.Context, string, ...jetstream.KVDeleteOpt) error
	ListKeysFiltered(context.Context, ...string) (jetstream.KeyLister, error)
}

// ClientSyncService owns the authenticated user's portable latest-value
// records. It uses KV revisions so concurrent clients and replicas cannot
// silently overwrite one another.
type ClientSyncService struct {
	kv           clientSyncKV
	validateUser func(context.Context, string) error
}

func NewClientSyncService(kv clientSyncKV, validateUser func(context.Context, string) error) *ClientSyncService {
	return &ClientSyncService{kv: kv, validateUser: validateUser}
}

func clientSyncPreferencesKey(userID string) string {
	return fmt.Sprintf("client_sync.%s.preferences", userID)
}

func clientSyncServerDirectoryKey(userID string) string {
	return fmt.Sprintf("client_sync.%s.servers", userID)
}

func clientSyncDeletionMarkerKey(userID string) string {
	return fmt.Sprintf("client_sync.%s.deleted", userID)
}

func (s *ClientSyncService) GetPreferences(ctx context.Context, userID string) (*clientsyncv1.Preferences, error) {
	if err := s.requireActiveUser(ctx, userID); err != nil {
		return nil, err
	}
	preferences := &clientsyncv1.Preferences{}
	if err := s.get(ctx, clientSyncPreferencesKey(userID), preferences); err != nil {
		if isClientSyncKeyAbsent(err) {
			return preferences, nil
		}
		return nil, fmt.Errorf("get personal preferences: %w", err)
	}
	return preferences, nil
}

// UpdatePreferences atomically mutates the user's single preferences document.
// The callback receives an isolated copy and may clear fields as well as set them.
func (s *ClientSyncService) UpdatePreferences(ctx context.Context, userID string, mutate func(*clientsyncv1.Preferences) error) (*clientsyncv1.Preferences, error) {
	if err := s.requireActiveUser(ctx, userID); err != nil {
		return nil, err
	}
	next := &clientsyncv1.Preferences{}
	err := s.mutate(ctx, userID, clientSyncPreferencesKey(userID), next, func() proto.Message {
		return &clientsyncv1.Preferences{}
	}, func(current proto.Message) error {
		return mutate(current.(*clientsyncv1.Preferences))
	})
	if err != nil {
		return nil, fmt.Errorf("update personal preferences: %w", err)
	}
	return next, nil
}

func (s *ClientSyncService) ListServers(ctx context.Context, userID string) (*clientsyncv1.ServerDirectory, error) {
	if err := s.requireActiveUser(ctx, userID); err != nil {
		return nil, err
	}
	directory := &clientsyncv1.ServerDirectory{}
	if err := s.get(ctx, clientSyncServerDirectoryKey(userID), directory); err != nil {
		if isClientSyncKeyAbsent(err) {
			return directory, nil
		}
		return nil, fmt.Errorf("get client-sync server directory: %w", err)
	}
	return directory, nil
}

func (s *ClientSyncService) CreateServer(ctx context.Context, userID string, server *clientsyncv1.KnownServer) (*clientsyncv1.KnownServer, error) {
	if server == nil || server.GetId() == "" {
		return nil, ErrInvalidArgument
	}
	created := proto.Clone(server).(*clientsyncv1.KnownServer)
	_, err := s.updateDirectory(ctx, userID, func(directory *clientsyncv1.ServerDirectory) error {
		if slices.ContainsFunc(directory.GetServers(), func(existing *clientsyncv1.KnownServer) bool {
			return existing.GetId() == created.GetId() || existing.GetUrl() == created.GetUrl()
		}) {
			return ErrKnownServerAlreadyExists
		}
		if len(directory.GetServers()) >= MaxClientSyncKnownServers {
			return ErrLimitExceeded
		}
		directory.Servers = append(directory.Servers, proto.Clone(created).(*clientsyncv1.KnownServer))
		if directory.HomeServerId == nil {
			homeID := created.GetId()
			directory.HomeServerId = &homeID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *ClientSyncService) UpdateServer(ctx context.Context, userID, serverID string, mutate func(*clientsyncv1.KnownServer) error) (*clientsyncv1.KnownServer, error) {
	var updated *clientsyncv1.KnownServer
	_, err := s.updateDirectory(ctx, userID, func(directory *clientsyncv1.ServerDirectory) error {
		for _, server := range directory.GetServers() {
			if server.GetId() != serverID {
				continue
			}
			if err := mutate(server); err != nil {
				return err
			}
			if slices.ContainsFunc(directory.GetServers(), func(other *clientsyncv1.KnownServer) bool {
				return other.GetId() != serverID && other.GetUrl() == server.GetUrl()
			}) {
				return ErrKnownServerAlreadyExists
			}
			updated = proto.Clone(server).(*clientsyncv1.KnownServer)
			return nil
		}
		return ErrNotFound
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *ClientSyncService) DeleteServer(ctx context.Context, userID, serverID string) error {
	_, err := s.updateDirectory(ctx, userID, func(directory *clientsyncv1.ServerDirectory) error {
		if directory.GetHomeServerId() == serverID {
			return ErrCannotDeleteHomeServer
		}
		before := len(directory.GetServers())
		directory.Servers = slices.DeleteFunc(directory.Servers, func(server *clientsyncv1.KnownServer) bool {
			return server.GetId() == serverID
		})
		if len(directory.GetServers()) == before {
			return nil
		}
		return nil
	})
	return err
}

func (s *ClientSyncService) SetHomeServer(ctx context.Context, userID, serverID string) (*clientsyncv1.KnownServer, error) {
	var home *clientsyncv1.KnownServer
	_, err := s.updateDirectory(ctx, userID, func(directory *clientsyncv1.ServerDirectory) error {
		for _, server := range directory.GetServers() {
			if server.GetId() == serverID {
				home = proto.Clone(server).(*clientsyncv1.KnownServer)
				directory.HomeServerId = &serverID
				return nil
			}
		}
		return ErrNotFound
	})
	if err != nil {
		return nil, err
	}
	return home, nil
}

// BeginDeleteUser durably fences client-sync mutations before the account
// deletion event is committed. Callers must cancel the fence if that commit
// fails.
func (s *ClientSyncService) BeginDeleteUser(ctx context.Context, userID string) error {
	// The marker is deliberately retained. It closes the race with requests
	// that authenticated before account deletion and gives restart recovery a
	// durable list of privacy cleanups to repeat idempotently.
	if _, err := s.kv.Create(ctx, clientSyncDeletionMarkerKey(userID), []byte{1}); err != nil && !errors.Is(err, jetstream.ErrKeyExists) {
		return fmt.Errorf("mark client sync deleted: %w", err)
	}
	return nil
}

// CancelDeleteUser removes a pre-commit fence after the account deletion event
// failed, allowing the still-active account to continue syncing.
func (s *ClientSyncService) CancelDeleteUser(ctx context.Context, userID string) error {
	if err := s.kv.Purge(ctx, clientSyncDeletionMarkerKey(userID)); err != nil && !isClientSyncKeyAbsent(err) {
		return fmt.Errorf("remove client-sync deletion marker: %w", err)
	}
	return nil
}

func (s *ClientSyncService) DeleteUser(ctx context.Context, userID string) error {
	if err := s.BeginDeleteUser(ctx, userID); err != nil {
		return err
	}
	return s.purgeUserRecords(ctx, userID)
}

func (s *ClientSyncService) purgeUserRecords(ctx context.Context, userID string) error {
	var errs []error
	for _, key := range []string{clientSyncPreferencesKey(userID), clientSyncServerDirectoryKey(userID)} {
		if err := s.kv.Purge(ctx, key); err != nil && !isClientSyncKeyAbsent(err) {
			errs = append(errs, fmt.Errorf("purge %s: %w", key, err))
		}
	}
	return errors.Join(errs...)
}

// RecoverPendingDeletions repeats privacy cleanup for every durable deletion
// marker. It is safe for every replica to run at boot because Purge is
// idempotent and the marker prevents later client-sync mutations.
func (s *ClientSyncService) RecoverPendingDeletions(ctx context.Context) error {
	lister, err := s.kv.ListKeysFiltered(ctx, "client_sync.*.deleted")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil
		}
		return fmt.Errorf("list client-sync deletion markers: %w", err)
	}
	var errs []error
	for key := range lister.Keys() {
		userID := strings.TrimSuffix(strings.TrimPrefix(key, "client_sync."), ".deleted")
		if userID == "" || clientSyncDeletionMarkerKey(userID) != key {
			continue
		}
		if err := s.purgeUserRecords(ctx, userID); err != nil {
			errs = append(errs, fmt.Errorf("recover %s: %w", key, err))
		}
	}
	return errors.Join(errs...)
}

func (s *ClientSyncService) updateDirectory(ctx context.Context, userID string, mutate func(*clientsyncv1.ServerDirectory) error) (*clientsyncv1.ServerDirectory, error) {
	if err := s.requireActiveUser(ctx, userID); err != nil {
		return nil, err
	}
	next := &clientsyncv1.ServerDirectory{}
	err := s.mutate(ctx, userID, clientSyncServerDirectoryKey(userID), next, func() proto.Message {
		return &clientsyncv1.ServerDirectory{}
	}, func(current proto.Message) error {
		return mutate(current.(*clientsyncv1.ServerDirectory))
	})
	if err != nil {
		return nil, fmt.Errorf("update client-sync server directory: %w", err)
	}
	return next, nil
}

func (s *ClientSyncService) get(ctx context.Context, key string, target proto.Message) error {
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		return err
	}
	if err := proto.Unmarshal(entry.Value(), target); err != nil {
		return fmt.Errorf("decode stored protobuf: %w", err)
	}
	return nil
}

func (s *ClientSyncService) mutate(ctx context.Context, userID, key string, result proto.Message, empty func() proto.Message, mutate func(proto.Message) error) error {
	for attempt := 0; attempt < clientSyncMutationRetries; attempt++ {
		current := empty()
		entry, err := s.kv.Get(ctx, key)
		exists := err == nil
		if err != nil && !isClientSyncKeyAbsent(err) {
			return err
		}
		if exists {
			if err := proto.Unmarshal(entry.Value(), current); err != nil {
				return fmt.Errorf("decode stored protobuf: %w", err)
			}
		}
		if err := mutate(current); err != nil {
			return err
		}
		data, err := proto.Marshal(current)
		if err != nil {
			return fmt.Errorf("encode stored protobuf: %w", err)
		}
		// Re-check immediately before the OCC write. Account deletion can race
		// an already-authenticated request; no plaintext record may be created
		// or restored after the durable user tombstone becomes visible.
		if err := s.requireActiveUser(ctx, userID); err != nil {
			return err
		}
		if exists {
			_, err = s.kv.Update(ctx, key, data, entry.Revision())
		} else {
			_, err = s.kv.Create(ctx, key, data)
		}
		if err != nil {
			if jetstreamutil.IsSequenceConflict(err) {
				continue
			}
			return err
		}
		// A deletion marker can be committed between the pre-write check and
		// this write. In that ordering, remove the new revision before
		// returning so account deletion always wins across replicas.
		if deleted, markerErr := s.userDeleted(ctx, userID); markerErr != nil {
			_ = s.kv.Purge(ctx, key)
			return markerErr
		} else if deleted {
			if purgeErr := s.kv.Purge(ctx, key); purgeErr != nil && !isClientSyncKeyAbsent(purgeErr) {
				return fmt.Errorf("purge client sync written during account deletion: %w", purgeErr)
			}
			return ErrNotFound
		}
		proto.Reset(result)
		proto.Merge(result, current)
		return nil
	}
	return fmt.Errorf("client sync changed concurrently too many times")
}

func (s *ClientSyncService) requireActiveUser(ctx context.Context, userID string) error {
	if err := requireAuthenticatedActor(userID); err != nil {
		return err
	}
	deleted, err := s.userDeleted(ctx, userID)
	if err != nil {
		return err
	}
	if deleted {
		return ErrNotFound
	}
	if s.validateUser != nil {
		return s.validateUser(ctx, userID)
	}
	return nil
}

func (s *ClientSyncService) userDeleted(ctx context.Context, userID string) (bool, error) {
	_, err := s.kv.Get(ctx, clientSyncDeletionMarkerKey(userID))
	if err == nil {
		return true, nil
	}
	if isClientSyncKeyAbsent(err) {
		return false, nil
	}
	return false, fmt.Errorf("read client-sync deletion marker: %w", err)
}

func isClientSyncKeyAbsent(err error) bool {
	return errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted)
}
