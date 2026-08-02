// Package accountdata owns encrypted account-scoped user data persistence.
package accountdata

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/keyvault"
	"hmans.de/authling/internal/tinybasesync"
	"hmans.de/chatto/pkg/datacrypto"
)

const (
	// MaxPlaintextSize is the complete-state limit for one account data space.
	MaxPlaintextSize = 256 << 10
	dataKeyPurpose   = "account-data"
)

// AccountKeys resolves the user-key reference owned by one active account.
type AccountKeys interface {
	UserKeyRef(accountID string) (string, bool)
}

// Service creates durable TinyBase stores selected by authenticated account.
type Service struct {
	kv       jetstream.KeyValue
	vault    *keyvault.Vault
	accounts AccountKeys
	indexKey []byte
}

// New constructs the account-data persistence boundary.
func New(kv jetstream.KeyValue, vault *keyvault.Vault, accounts AccountKeys, indexKey []byte) *Service {
	return &Service{kv: kv, vault: vault, accounts: accounts, indexKey: append([]byte(nil), indexKey...)}
}

// Store returns the data space for accountID. Accounts without user-key
// material cannot use protected account data.
func (service *Service) Store(_ context.Context, accountID string) (tinybasesync.Store, error) {
	if accountID == "" || service.kv == nil || service.vault == nil || service.accounts == nil || len(service.indexKey) == 0 {
		return nil, errors.New("account data unavailable")
	}
	userKeyRef, ok := service.accounts.UserKeyRef(accountID)
	if !ok {
		return nil, errors.New("account data unavailable")
	}
	return &store{
		kv: service.kv, vault: service.vault, userKeyRef: userKeyRef,
		stateKey:   service.opaqueRef("state", accountID),
		dataKeyRef: service.opaqueRef("key", accountID),
	}, nil
}

func (service *Service) opaqueRef(kind, accountID string) string {
	digest := hmac.New(sha256.New, service.indexKey)
	_, _ = digest.Write([]byte("account-data\x00" + kind + "\x00" + accountID))
	return "data." + base64.RawURLEncoding.EncodeToString(digest.Sum(nil))
}

type store struct {
	mu             sync.Mutex
	kv             jetstream.KeyValue
	vault          *keyvault.Vault
	userKeyRef     string
	stateKey       string
	dataKeyRef     string
	cachedRevision uint64
	cachedContent  []byte
}

type sealedState struct {
	Version    int    `json:"version"`
	DataKeyRef string `json:"data_key_ref"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

func (store *store) Load(ctx context.Context) ([]byte, uint64, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	entry, err := store.kv.Get(ctx, store.stateKey)
	if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
		store.cachedRevision = 0
		clear(store.cachedContent)
		store.cachedContent = nil
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("read account data: %w", err)
	}
	if entry.Revision() == store.cachedRevision {
		return append([]byte(nil), store.cachedContent...), entry.Revision(), nil
	}
	var sealed sealedState
	if err := json.Unmarshal(entry.Value(), &sealed); err != nil || sealed.Version != 1 || sealed.DataKeyRef != store.dataKeyRef || len(sealed.Nonce) == 0 || len(sealed.Ciphertext) == 0 {
		return nil, 0, errors.New("decode account-data envelope")
	}
	key, err := store.vault.ResolveDataKeyForPurpose(ctx, sealed.DataKeyRef, store.userKeyRef, dataKeyPurpose)
	if err != nil {
		return nil, 0, fmt.Errorf("resolve account-data key: %w", err)
	}
	defer clear(key)
	plain, err := datacrypto.Open(key, sealed.Ciphertext, sealed.Nonce, store.aad())
	if err != nil {
		return nil, 0, fmt.Errorf("decrypt account data: %w", err)
	}
	if len(plain) > MaxPlaintextSize {
		clear(plain)
		return nil, 0, errors.New("account data exceeds size limit")
	}
	clear(store.cachedContent)
	store.cachedContent = append([]byte(nil), plain...)
	store.cachedRevision = entry.Revision()
	return plain, entry.Revision(), nil
}

func (store *store) Save(ctx context.Context, content []byte, expectedRevision uint64) (uint64, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(content) > MaxPlaintextSize {
		return 0, errors.New("account data exceeds size limit")
	}
	key, err := store.vault.EnsureDataKey(ctx, store.dataKeyRef, store.userKeyRef, dataKeyPurpose)
	if err != nil {
		return 0, fmt.Errorf("ensure account-data key: %w", err)
	}
	sealed, err := datacrypto.Seal(key, content, store.aad())
	clear(key)
	if err != nil {
		return 0, fmt.Errorf("encrypt account data: %w", err)
	}
	encoded, err := json.Marshal(sealedState{Version: 1, DataKeyRef: store.dataKeyRef, Nonce: sealed.Nonce, Ciphertext: sealed.Ciphertext})
	if err != nil {
		return 0, fmt.Errorf("encode account-data envelope: %w", err)
	}
	var revision uint64
	if expectedRevision == 0 {
		revision, err = store.kv.Create(ctx, store.stateKey, encoded)
	} else {
		revision, err = store.kv.Update(ctx, store.stateKey, encoded, expectedRevision)
	}
	if errors.Is(err, jetstream.ErrKeyExists) {
		return 0, tinybasesync.ErrConflict
	}
	if err != nil {
		return 0, fmt.Errorf("write account data: %w", err)
	}
	clear(store.cachedContent)
	store.cachedContent = append([]byte(nil), content...)
	store.cachedRevision = revision
	return revision, nil
}

func (store *store) aad() []byte {
	return []byte("authling:account-data:v1\x00" + store.stateKey + "\x00" + store.dataKeyRef + "\x00" + dataKeyPurpose)
}
