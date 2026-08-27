package sessions

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/storage"
)

var (
	// ErrInventoryUnavailable means the process-wide session inventory has not
	// completed startup or stopped consuming its authoritative KV source.
	ErrInventoryUnavailable = errors.New("session inventory unavailable")
	// ErrCurrentSession prevents the session-management surface from revoking
	// the browser that is issuing the request.
	ErrCurrentSession = errors.New("cannot revoke current session")
)

// BrowserSession is the non-sensitive account-facing view of an active
// first-party browser session. ID is opaque and is neither a bearer credential
// nor a JetStream coordinate.
type BrowserSession struct {
	ID         string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	Current    bool
}

type inventoryEntry struct {
	ID      string
	Key     string
	Session Session
}

// RunInventory maintains the one process-wide in-memory index of encrypted
// browser-session KV records. Runtime KV remains authoritative for every
// mutation; this model exists only to make account-scoped enumeration cheap.
func (s *Service) RunInventory(ctx context.Context) (runErr error) {
	s.inventoryMu.Lock()
	if s.inventoryStarted {
		s.inventoryMu.Unlock()
		return fmt.Errorf("session inventory already started")
	}
	s.inventoryStarted = true
	s.inventoryMu.Unlock()

	defer func() {
		if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, context.DeadlineExceeded) {
			s.failInventory(runErr)
		}
	}()

	watcher, err := s.kv.Watch(ctx, "session.*")
	if err != nil {
		return fmt.Errorf("watch browser sessions: %w", err)
	}
	defer watcher.Stop()

	ready := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case entry, ok := <-watcher.Updates():
			if !ok {
				if err := ctx.Err(); err != nil {
					return err
				}
				return fmt.Errorf("browser session watcher stopped: %w", ErrInventoryUnavailable)
			}
			if entry == nil {
				if !ready {
					s.markInventoryReady()
					ready = true
				}
				continue
			}
			s.applyInventoryEntry(entry)
		}
	}
}

// WaitForInventoryStartup blocks until the watcher has received the complete
// initial KV snapshot or reports a fatal startup failure.
func (s *Service) WaitForInventoryStartup(ctx context.Context) error {
	select {
	case <-s.inventoryReady:
		return nil
	default:
	}
	select {
	case <-s.inventoryReady:
		return nil
	case <-s.inventoryFailed:
		s.inventoryMu.RLock()
		err := s.inventoryErr
		s.inventoryMu.RUnlock()
		if err == nil {
			return ErrInventoryUnavailable
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// List returns the account's currently valid browser sessions. The current
// bearer is used only to identify this browser and is never retained.
func (s *Service) List(ctx context.Context, accountID, currentToken string) ([]BrowserSession, error) {
	if strings.TrimSpace(accountID) == "" || !validToken(currentToken) {
		return nil, ErrNotFound
	}
	if err := s.WaitForInventoryStartup(ctx); err != nil {
		return nil, fmt.Errorf("list browser sessions: %w", err)
	}
	currentKey := s.sessionKey(currentToken)
	now := s.now().UTC()

	s.inventoryMu.RLock()
	current, currentOK := s.inventoryByKey[currentKey]
	if !currentOK || current.Session.AccountID != accountID || !s.sessionActive(current.Session, now) {
		s.inventoryMu.RUnlock()
		return nil, ErrNotFound
	}
	keys := s.inventoryByAccount[accountID]
	views := make([]BrowserSession, 0, len(keys))
	for key := range keys {
		entry, ok := s.inventoryByKey[key]
		if !ok || !s.sessionActive(entry.Session, now) {
			continue
		}
		views = append(views, BrowserSession{
			ID: entry.ID, CreatedAt: entry.Session.CreatedAt,
			LastSeenAt: entry.Session.LastSeenAt, ExpiresAt: entry.Session.ExpiresAt,
			Current: key == currentKey,
		})
	}
	s.inventoryMu.RUnlock()

	sort.Slice(views, func(i, j int) bool {
		if views[i].Current != views[j].Current {
			return views[i].Current
		}
		return views[i].LastSeenAt.After(views[j].LastSeenAt)
	})
	return views, nil
}

// RevokeSession removes one other browser session after authoritatively
// re-reading and re-authorizing its encrypted KV record.
func (s *Service) RevokeSession(ctx context.Context, accountID, id, currentToken string) error {
	if strings.TrimSpace(accountID) == "" || !validSessionID(id) || !validToken(currentToken) {
		return ErrNotFound
	}
	if err := s.WaitForInventoryStartup(ctx); err != nil {
		return fmt.Errorf("revoke browser session: %w", err)
	}

	s.inventoryMu.RLock()
	entry, ok := s.inventoryEntryByIDLocked(accountID, id)
	s.inventoryMu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	if entry.Key == s.sessionKey(currentToken) {
		return ErrCurrentSession
	}
	return s.revokeInventoryEntry(ctx, accountID, entry)
}

// RevokeOtherSessions removes every currently inventoried session belonging
// to the account except the browser issuing the request.
func (s *Service) RevokeOtherSessions(ctx context.Context, accountID, currentToken string) (int, error) {
	if strings.TrimSpace(accountID) == "" || !validToken(currentToken) {
		return 0, ErrNotFound
	}
	if err := s.WaitForInventoryStartup(ctx); err != nil {
		return 0, fmt.Errorf("revoke other browser sessions: %w", err)
	}
	currentKey := s.sessionKey(currentToken)

	s.inventoryMu.RLock()
	entries := make([]inventoryEntry, 0, len(s.inventoryByAccount[accountID]))
	for key := range s.inventoryByAccount[accountID] {
		if key != currentKey {
			entries = append(entries, s.inventoryByKey[key])
		}
	}
	s.inventoryMu.RUnlock()

	revoked := 0
	for _, entry := range entries {
		if err := s.revokeInventoryEntry(ctx, accountID, entry); errors.Is(err, ErrNotFound) {
			continue
		} else if err != nil {
			return revoked, err
		}
		revoked++
	}
	return revoked, nil
}

func (s *Service) revokeInventoryEntry(ctx context.Context, accountID string, projected inventoryEntry) error {
	for range 4 {
		entry, state, err := s.read(ctx, projected.Key)
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read browser session for revocation: %w", err)
		}
		if state.AccountID != accountID || s.publicSessionID(projected.Key) != projected.ID {
			return ErrNotFound
		}
		revision, err := storage.DeleteKey(ctx, s.js, storage.RuntimeStateBucket, projected.Key, entry.Revision())
		if err != nil {
			continue
		}
		if err := s.waitForInventoryRevision(ctx, revision); err != nil {
			return fmt.Errorf("wait for session inventory: %w", err)
		}
		return nil
	}
	return fmt.Errorf("revoke browser session after repeated conflicts")
}

func (s *Service) applyInventoryEntry(entry jetstream.KeyValueEntry) {
	s.inventoryMu.Lock()
	defer s.inventoryMu.Unlock()

	s.removeInventoryKeyLocked(entry.Key())
	if entry.Operation() == jetstream.KeyValuePut {
		if state, err := s.open(entry.Key(), entry.Value()); err == nil {
			indexed := inventoryEntry{ID: s.publicSessionID(entry.Key()), Key: entry.Key(), Session: state}
			s.inventoryByKey[entry.Key()] = indexed
			keys := s.inventoryByAccount[state.AccountID]
			if keys == nil {
				keys = make(map[string]struct{})
				s.inventoryByAccount[state.AccountID] = keys
			}
			keys[entry.Key()] = struct{}{}
		}
	}
	if entry.Revision() > s.inventoryRevision {
		s.inventoryRevision = entry.Revision()
	}
	s.signalInventoryChangeLocked()
}

func (s *Service) removeInventoryKeyLocked(key string) {
	previous, ok := s.inventoryByKey[key]
	if !ok {
		return
	}
	delete(s.inventoryByKey, key)
	keys := s.inventoryByAccount[previous.Session.AccountID]
	delete(keys, key)
	if len(keys) == 0 {
		delete(s.inventoryByAccount, previous.Session.AccountID)
	}
}

func (s *Service) inventoryEntryByIDLocked(accountID, id string) (inventoryEntry, bool) {
	for key := range s.inventoryByAccount[accountID] {
		entry, ok := s.inventoryByKey[key]
		if ok && hmac.Equal([]byte(entry.ID), []byte(id)) {
			return entry, true
		}
	}
	return inventoryEntry{}, false
}

func (s *Service) sessionActive(state Session, now time.Time) bool {
	if !now.Before(state.ExpiresAt) || now.Sub(state.LastSeenAt) >= InactivityLifetime {
		return false
	}
	if s.authenticationVersion != nil {
		version, ok := s.authenticationVersion(state.AccountID)
		return ok && version == state.AuthenticationVersion
	}
	return true
}

func (s *Service) publicSessionID(key string) string {
	digest := hmac.New(sha256.New, s.key)
	_, _ = digest.Write([]byte("session-id\x00" + key))
	return "ses_" + base64.RawURLEncoding.EncodeToString(digest.Sum(nil))
}

func validSessionID(id string) bool {
	if !strings.HasPrefix(id, "ses_") {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(id, "ses_"))
	return err == nil && len(decoded) == sha256.Size
}

func (s *Service) waitForInventoryRevision(ctx context.Context, revision uint64) error {
	for {
		s.inventoryMu.RLock()
		started := s.inventoryStarted
		current := s.inventoryRevision
		changed := s.inventoryChanged
		err := s.inventoryErr
		s.inventoryMu.RUnlock()
		if !started {
			return nil
		}
		if err != nil {
			return errors.Join(ErrInventoryUnavailable, err)
		}
		if current >= revision {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *Service) markInventoryReady() {
	s.inventoryMu.Lock()
	defer s.inventoryMu.Unlock()
	select {
	case <-s.inventoryReady:
	default:
		close(s.inventoryReady)
	}
	s.signalInventoryChangeLocked()
}

func (s *Service) failInventory(err error) {
	s.inventoryMu.Lock()
	defer s.inventoryMu.Unlock()
	if s.inventoryErr == nil {
		s.inventoryErr = err
		close(s.inventoryFailed)
		s.signalInventoryChangeLocked()
	}
}

func (s *Service) signalInventoryChangeLocked() {
	close(s.inventoryChanged)
	s.inventoryChanged = make(chan struct{})
}
