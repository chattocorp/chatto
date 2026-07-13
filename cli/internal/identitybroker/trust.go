package identitybroker

import (
	"crypto/ed25519"
	"fmt"
	"sync"
)

// TrustStore contains public keys discovered for exact server origins. The PoC
// pins keys for the verifier's lifetime; production rotation remains a design
// question.
type TrustStore struct {
	mu   sync.RWMutex
	keys map[string]map[string]ed25519.PublicKey
}

// NewTrustStore creates an empty origin-key pin set.
func NewTrustStore() *TrustStore {
	return &TrustStore{keys: map[string]map[string]ed25519.PublicKey{}}
}

// Add pins one discovered public key to its exact origin and key ID.
func (s *TrustStore) Add(discovery DiscoveryKey) error {
	origin, err := NormalizeOrigin(discovery.Origin)
	if err != nil {
		return err
	}
	if discovery.Protocol != ProtocolVersion || discovery.KeyID == "" || len(discovery.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: invalid discovery key", ErrInvalidArtifact)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keys[origin] == nil {
		s.keys[origin] = map[string]ed25519.PublicKey{}
	}
	if existing := s.keys[origin][discovery.KeyID]; existing != nil && !existing.Equal(ed25519.PublicKey(discovery.PublicKey)) {
		return fmt.Errorf("%w: key id %q changed for %s", ErrInvalidArtifact, discovery.KeyID, origin)
	}
	s.keys[origin][discovery.KeyID] = append(ed25519.PublicKey(nil), discovery.PublicKey...)
	return nil
}

func (s *TrustStore) key(origin, keyID string) (ed25519.PublicKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := s.keys[origin]
	key, ok := keys[keyID]
	return append(ed25519.PublicKey(nil), key...), ok
}
