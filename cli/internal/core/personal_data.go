package core

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/jetstreamutil"
	personaldatav1 "hmans.de/chatto/internal/pb/chatto/personaldata/v1"
)

const personalDataMutationRetries = 5

// PersonalDataService owns the authenticated user's portable latest-value
// records. It uses KV revisions so concurrent clients and replicas cannot
// silently overwrite one another.
type PersonalDataService struct {
	kv jetstream.KeyValue
}

func NewPersonalDataService(kv jetstream.KeyValue) *PersonalDataService {
	return &PersonalDataService{kv: kv}
}

func personalPreferencesKey(userID string) string {
	return fmt.Sprintf("personal_data.%s.preferences", userID)
}

func personalServerDirectoryKey(userID string) string {
	return fmt.Sprintf("personal_data.%s.servers", userID)
}

func (s *PersonalDataService) GetPreferences(ctx context.Context, userID string) (*personaldatav1.Preferences, error) {
	if err := requireAuthenticatedActor(userID); err != nil {
		return nil, err
	}
	preferences := &personaldatav1.Preferences{}
	if err := s.get(ctx, personalPreferencesKey(userID), preferences); err != nil {
		if isPersonalDataKeyAbsent(err) {
			return preferences, nil
		}
		return nil, fmt.Errorf("get personal preferences: %w", err)
	}
	return preferences, nil
}

// UpdatePreferences atomically mutates the user's single preferences document.
// The callback receives an isolated copy and may clear fields as well as set them.
func (s *PersonalDataService) UpdatePreferences(ctx context.Context, userID string, mutate func(*personaldatav1.Preferences) error) (*personaldatav1.Preferences, error) {
	if err := requireAuthenticatedActor(userID); err != nil {
		return nil, err
	}
	next := &personaldatav1.Preferences{}
	err := s.mutate(ctx, personalPreferencesKey(userID), next, func() proto.Message {
		return &personaldatav1.Preferences{}
	}, func(current proto.Message) error {
		return mutate(current.(*personaldatav1.Preferences))
	})
	if err != nil {
		return nil, fmt.Errorf("update personal preferences: %w", err)
	}
	return next, nil
}

func (s *PersonalDataService) ListServers(ctx context.Context, userID string) (*personaldatav1.ServerDirectory, error) {
	if err := requireAuthenticatedActor(userID); err != nil {
		return nil, err
	}
	directory := &personaldatav1.ServerDirectory{}
	if err := s.get(ctx, personalServerDirectoryKey(userID), directory); err != nil {
		if isPersonalDataKeyAbsent(err) {
			return directory, nil
		}
		return nil, fmt.Errorf("get personal server directory: %w", err)
	}
	return directory, nil
}

func (s *PersonalDataService) CreateServer(ctx context.Context, userID string, server *personaldatav1.KnownServer) (*personaldatav1.KnownServer, error) {
	if server == nil || server.GetId() == "" {
		return nil, ErrInvalidArgument
	}
	created := proto.Clone(server).(*personaldatav1.KnownServer)
	_, err := s.updateDirectory(ctx, userID, func(directory *personaldatav1.ServerDirectory) error {
		if slices.ContainsFunc(directory.GetServers(), func(existing *personaldatav1.KnownServer) bool {
			return existing.GetId() == created.GetId() || existing.GetUrl() == created.GetUrl()
		}) {
			return ErrPersonalServerAlreadyExists
		}
		directory.Servers = append(directory.Servers, proto.Clone(created).(*personaldatav1.KnownServer))
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

func (s *PersonalDataService) UpdateServer(ctx context.Context, userID, serverID string, mutate func(*personaldatav1.KnownServer) error) (*personaldatav1.KnownServer, error) {
	var updated *personaldatav1.KnownServer
	_, err := s.updateDirectory(ctx, userID, func(directory *personaldatav1.ServerDirectory) error {
		for _, server := range directory.GetServers() {
			if server.GetId() != serverID {
				continue
			}
			if err := mutate(server); err != nil {
				return err
			}
			if slices.ContainsFunc(directory.GetServers(), func(other *personaldatav1.KnownServer) bool {
				return other.GetId() != serverID && other.GetUrl() == server.GetUrl()
			}) {
				return ErrPersonalServerAlreadyExists
			}
			updated = proto.Clone(server).(*personaldatav1.KnownServer)
			return nil
		}
		return ErrNotFound
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *PersonalDataService) DeleteServer(ctx context.Context, userID, serverID string) error {
	_, err := s.updateDirectory(ctx, userID, func(directory *personaldatav1.ServerDirectory) error {
		if directory.GetHomeServerId() == serverID {
			return ErrCannotDeleteHomeServer
		}
		before := len(directory.GetServers())
		directory.Servers = slices.DeleteFunc(directory.Servers, func(server *personaldatav1.KnownServer) bool {
			return server.GetId() == serverID
		})
		if len(directory.GetServers()) == before {
			return nil
		}
		return nil
	})
	return err
}

func (s *PersonalDataService) SetHomeServer(ctx context.Context, userID, serverID string) (*personaldatav1.KnownServer, error) {
	var home *personaldatav1.KnownServer
	_, err := s.updateDirectory(ctx, userID, func(directory *personaldatav1.ServerDirectory) error {
		for _, server := range directory.GetServers() {
			if server.GetId() == serverID {
				home = proto.Clone(server).(*personaldatav1.KnownServer)
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

func (s *PersonalDataService) DeleteUser(ctx context.Context, userID string) error {
	for _, key := range []string{personalPreferencesKey(userID), personalServerDirectoryKey(userID)} {
		if err := s.kv.Delete(ctx, key); err != nil && !isPersonalDataKeyAbsent(err) {
			return fmt.Errorf("delete %s: %w", key, err)
		}
	}
	return nil
}

func (s *PersonalDataService) updateDirectory(ctx context.Context, userID string, mutate func(*personaldatav1.ServerDirectory) error) (*personaldatav1.ServerDirectory, error) {
	if err := requireAuthenticatedActor(userID); err != nil {
		return nil, err
	}
	next := &personaldatav1.ServerDirectory{}
	err := s.mutate(ctx, personalServerDirectoryKey(userID), next, func() proto.Message {
		return &personaldatav1.ServerDirectory{}
	}, func(current proto.Message) error {
		return mutate(current.(*personaldatav1.ServerDirectory))
	})
	if err != nil {
		return nil, fmt.Errorf("update personal server directory: %w", err)
	}
	return next, nil
}

func (s *PersonalDataService) get(ctx context.Context, key string, target proto.Message) error {
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		return err
	}
	if err := proto.Unmarshal(entry.Value(), target); err != nil {
		return fmt.Errorf("decode stored protobuf: %w", err)
	}
	return nil
}

func (s *PersonalDataService) mutate(ctx context.Context, key string, result proto.Message, empty func() proto.Message, mutate func(proto.Message) error) error {
	for attempt := 0; attempt < personalDataMutationRetries; attempt++ {
		current := empty()
		entry, err := s.kv.Get(ctx, key)
		exists := err == nil
		if err != nil && !isPersonalDataKeyAbsent(err) {
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
		proto.Reset(result)
		proto.Merge(result, current)
		return nil
	}
	return fmt.Errorf("personal data changed concurrently too many times")
}

func isPersonalDataKeyAbsent(err error) bool {
	return errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted)
}
