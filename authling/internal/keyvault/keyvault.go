// Package keyvault owns Authling's application-specific key hierarchy.
package keyvault

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/ids"
	"hmans.de/chatto/pkg/datacrypto"
)

const (
	systemWorkflowKey        = "system.workflow.v1"
	systemDummyUserKey       = "system.authentication-dummy-user.v1"
	systemDummyCredentialKey = "system.authentication-dummy-credential.v1"
	systemOIDCSigningKey     = "system.oidc-signing.v1"
)

type rawKeyRecord struct {
	Version   int       `json:"version"`
	Key       []byte    `json:"key"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

type wrappedKeyRecord struct {
	Version    int       `json:"version"`
	UserKeyRef string    `json:"user_key_ref"`
	Nonce      []byte    `json:"nonce"`
	Ciphertext []byte    `json:"ciphertext"`
	CreatedAt  time.Time `json:"created_at"`
}

type provisioningRecord struct {
	Version                int       `json:"version"`
	CreatedAt              time.Time `json:"created_at"`
	UserKeyRef, DataKeyRef string
}

type signingKeyRecord struct {
	Version    int       `json:"version"`
	Algorithm  string    `json:"algorithm"`
	PrivateDER []byte    `json:"private_der"`
	CreatedAt  time.Time `json:"created_at"`
}

// SigningKey is Authling's current OpenID Connect signing identity. Ref and ID
// are safe to persist and publish; Private must remain inside the key boundary.
type SigningKey struct {
	Ref     string
	ID      string
	Private *rsa.PrivateKey
}

// Vault stores user keys and wrapped data keys outside ordinary event/runtime
// storage. References are random and reveal no account identifier.
type Vault struct{ kv jetstream.KeyValue }

func New(kv jetstream.KeyValue) *Vault { return &Vault{kv: kv} }

// WorkflowKey returns the deployment key used to protect short-lived flows
// and derive non-reversible lookup keys.
func (v *Vault) WorkflowKey(ctx context.Context) ([]byte, error) {
	entry, err := v.kv.Get(ctx, systemWorkflowKey)
	if err == nil {
		return decodeRaw(entry.Value())
	}
	if !errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, fmt.Errorf("read workflow key: %w", err)
	}
	key, err := datacrypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(rawKeyRecord{Version: 1, Key: key})
	if err != nil {
		return nil, fmt.Errorf("encode workflow key: %w", err)
	}
	if _, err := v.kv.Create(ctx, systemWorkflowKey, data); err != nil {
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return nil, fmt.Errorf("create workflow key: %w", err)
		}
		entry, err = v.kv.Get(ctx, systemWorkflowKey)
		if err != nil {
			return nil, fmt.Errorf("read raced workflow key: %w", err)
		}
		return decodeRaw(entry.Value())
	}
	return key, nil
}

// OIDCSigningKey returns the deployment's stable RS256 signing key, creating
// it with JetStream Create semantics when the deployment has none yet.
func (v *Vault) OIDCSigningKey(ctx context.Context) (SigningKey, error) {
	entry, err := v.kv.Get(ctx, systemOIDCSigningKey)
	if err == nil {
		return decodeSigningKey(entry.Value())
	}
	if !errors.Is(err, jetstream.ErrKeyNotFound) {
		return SigningKey{}, fmt.Errorf("read OIDC signing key: %w", err)
	}
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return SigningKey{}, fmt.Errorf("generate OIDC signing key: %w", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return SigningKey{}, fmt.Errorf("encode OIDC signing key: %w", err)
	}
	data, err := json.Marshal(signingKeyRecord{
		Version: 1, Algorithm: "RS256", PrivateDER: privateDER, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		return SigningKey{}, fmt.Errorf("encode OIDC signing-key record: %w", err)
	}
	if _, err := v.kv.Create(ctx, systemOIDCSigningKey, data); err != nil {
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return SigningKey{}, fmt.Errorf("create OIDC signing key: %w", err)
		}
		entry, err = v.kv.Get(ctx, systemOIDCSigningKey)
		if err != nil {
			return SigningKey{}, fmt.Errorf("read raced OIDC signing key: %w", err)
		}
		return decodeSigningKey(entry.Value())
	}
	return signingKey(private)
}

// AuthenticationDummyKey returns a persistent synthetic credential key pair.
// Unknown-account authentication resolves this pair through the same durable
// storage and unwrapping path as a real credential, reducing timing leakage.
func (v *Vault) AuthenticationDummyKey(ctx context.Context) (userRef, dataRef string, dataKey []byte, err error) {
	userKey, err := v.ensureRawKey(ctx, systemDummyUserKey)
	if err != nil {
		return "", "", nil, fmt.Errorf("open authentication dummy user key: %w", err)
	}
	defer clear(userKey)

	_, err = v.kv.Get(ctx, systemDummyCredentialKey)
	if err == nil {
		key, resolveErr := v.ResolveDataKey(ctx, systemDummyCredentialKey, systemDummyUserKey)
		return systemDummyUserKey, systemDummyCredentialKey, key, resolveErr
	}
	if !errors.Is(err, jetstream.ErrKeyNotFound) {
		return "", "", nil, fmt.Errorf("read authentication dummy credential key: %w", err)
	}
	dataKey, err = datacrypto.GenerateKey()
	if err != nil {
		return "", "", nil, err
	}
	wrapped, err := datacrypto.WrapKey(userKey, dataKey, wrapAAD(systemDummyUserKey, systemDummyCredentialKey))
	if err != nil {
		clear(dataKey)
		return "", "", nil, err
	}
	encoded, err := json.Marshal(wrappedKeyRecord{
		Version: 1, UserKeyRef: systemDummyUserKey, Nonce: wrapped.Nonce,
		Ciphertext: wrapped.Ciphertext, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		clear(dataKey)
		return "", "", nil, err
	}
	if _, err := v.kv.Create(ctx, systemDummyCredentialKey, encoded); err != nil {
		clear(dataKey)
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return "", "", nil, fmt.Errorf("store authentication dummy credential key: %w", err)
		}
		dataKey, err = v.ResolveDataKey(ctx, systemDummyCredentialKey, systemDummyUserKey)
		if err != nil {
			return "", "", nil, err
		}
	}
	return systemDummyUserKey, systemDummyCredentialKey, dataKey, nil
}

func (v *Vault) ensureRawKey(ctx context.Context, ref string) ([]byte, error) {
	entry, err := v.kv.Get(ctx, ref)
	if err == nil {
		return decodeRaw(entry.Value())
	}
	if !errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, err
	}
	key, err := datacrypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(rawKeyRecord{Version: 1, Key: key, CreatedAt: time.Now().UTC()})
	if err != nil {
		clear(key)
		return nil, err
	}
	if _, err := v.kv.Create(ctx, ref, encoded); err != nil {
		clear(key)
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return nil, err
		}
		entry, err = v.kv.Get(ctx, ref)
		if err != nil {
			return nil, err
		}
		return decodeRaw(entry.Value())
	}
	return key, nil
}

// ProvisionCredentialKeys creates a user key plus a credential data key
// wrapped beneath it. Both writes complete before an event may reference them.
func (v *Vault) ProvisionCredentialKeys(ctx context.Context) (operationRef, userRef, dataRef string, dataKey []byte, err error) {
	operationRef, err = ids.New("op")
	if err != nil {
		return "", "", "", nil, err
	}
	userRef, err = ids.New("uk")
	if err != nil {
		return "", "", "", nil, err
	}
	dataRef, err = ids.New("dk")
	if err != nil {
		return "", "", "", nil, err
	}
	now := time.Now().UTC()
	operationData, _ := json.Marshal(provisioningRecord{Version: 1, CreatedAt: now, UserKeyRef: userRef, DataKeyRef: dataRef})
	if _, err = v.kv.Create(ctx, operationRef, operationData); err != nil {
		return "", "", "", nil, fmt.Errorf("store provisioning operation: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			cleanupContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = v.RemoveProvisionedCredentialKeys(cleanupContext, operationRef, userRef, dataRef)
		}
	}()
	userKey, err := datacrypto.GenerateKey()
	if err != nil {
		return operationRef, userRef, dataRef, nil, err
	}
	defer clear(userKey)
	dataKey, err = datacrypto.GenerateKey()
	if err != nil {
		return operationRef, userRef, dataRef, nil, err
	}
	userData, err := json.Marshal(rawKeyRecord{Version: 1, Key: userKey, CreatedAt: now})
	if err != nil {
		return operationRef, userRef, dataRef, nil, err
	}
	if _, err = v.kv.Create(ctx, userRef, userData); err != nil {
		return operationRef, userRef, dataRef, nil, fmt.Errorf("store user key: %w", err)
	}
	wrapped, err := datacrypto.WrapKey(userKey, dataKey, wrapAAD(userRef, dataRef))
	if err != nil {
		return operationRef, userRef, dataRef, nil, err
	}
	wrappedData, err := json.Marshal(wrappedKeyRecord{Version: 1, UserKeyRef: userRef, Nonce: wrapped.Nonce, Ciphertext: wrapped.Ciphertext, CreatedAt: now})
	if err != nil {
		return operationRef, userRef, dataRef, nil, err
	}
	if _, err = v.kv.Create(ctx, dataRef, wrappedData); err != nil {
		return operationRef, userRef, dataRef, nil, fmt.Errorf("store credential data key: %w", err)
	}
	complete = true
	return operationRef, userRef, dataRef, dataKey, nil
}

// RemoveProvisionedCredentialKeys removes a pair that no committed event
// references. It must not be used after event publication succeeds.
func (v *Vault) RemoveProvisionedCredentialKeys(ctx context.Context, operationRef, userRef, dataRef string) error {
	var errs []error
	if dataRef != "" {
		if err := v.kv.Purge(ctx, dataRef); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			errs = append(errs, err)
		}
	}
	if userRef != "" {
		if err := v.kv.Purge(ctx, userRef); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			errs = append(errs, err)
		}
	}
	if operationRef != "" {
		if err := v.kv.Purge(ctx, operationRef); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// CompleteProvisioning removes the durable orphan marker after the referencing
// event commits. A crash before this call leaves a discoverable operation
// marker for the future event-backed cleanup worker.
func (v *Vault) CompleteProvisioning(ctx context.Context, operationRef string) error {
	return v.kv.Purge(ctx, operationRef)
}

// ResolveDataKey unwraps one credential data key and fails closed if either
// key record is absent or malformed.
func (v *Vault) ResolveDataKey(ctx context.Context, dataRef, expectedUserRef string) ([]byte, error) {
	entry, err := v.kv.Get(ctx, dataRef)
	if err != nil {
		return nil, fmt.Errorf("read wrapped data key: %w", err)
	}
	var wrapped wrappedKeyRecord
	if err := json.Unmarshal(entry.Value(), &wrapped); err != nil || wrapped.Version != 1 {
		return nil, fmt.Errorf("decode wrapped data key")
	}
	if wrapped.UserKeyRef != expectedUserRef {
		return nil, fmt.Errorf("wrapped data key user reference mismatch")
	}
	userEntry, err := v.kv.Get(ctx, wrapped.UserKeyRef)
	if err != nil {
		return nil, fmt.Errorf("read user key: %w", err)
	}
	userKey, err := decodeRaw(userEntry.Value())
	if err != nil {
		return nil, err
	}
	defer clear(userKey)
	return datacrypto.UnwrapKey(userKey, wrapped.Ciphertext, wrapped.Nonce, wrapAAD(wrapped.UserKeyRef, dataRef))
}

func decodeRaw(data []byte) ([]byte, error) {
	var record rawKeyRecord
	if err := json.Unmarshal(data, &record); err != nil || record.Version != 1 || len(record.Key) != datacrypto.KeySize {
		return nil, fmt.Errorf("decode raw key")
	}
	return record.Key, nil
}

func decodeSigningKey(data []byte) (SigningKey, error) {
	var record signingKeyRecord
	if err := json.Unmarshal(data, &record); err != nil || record.Version != 1 || record.Algorithm != "RS256" || len(record.PrivateDER) == 0 {
		return SigningKey{}, fmt.Errorf("decode OIDC signing-key record")
	}
	decoded, err := x509.ParsePKCS8PrivateKey(record.PrivateDER)
	if err != nil {
		return SigningKey{}, fmt.Errorf("parse OIDC signing key: %w", err)
	}
	private, ok := decoded.(*rsa.PrivateKey)
	if !ok || private.N.BitLen() < 2048 || private.Validate() != nil {
		return SigningKey{}, fmt.Errorf("invalid OIDC signing key")
	}
	return signingKey(private)
}

func signingKey(private *rsa.PrivateKey) (SigningKey, error) {
	publicDER, err := x509.MarshalPKIXPublicKey(&private.PublicKey)
	if err != nil {
		return SigningKey{}, fmt.Errorf("encode OIDC public key: %w", err)
	}
	digest := sha256.Sum256(publicDER)
	return SigningKey{
		Ref: systemOIDCSigningKey, ID: "sig_" + base64.RawURLEncoding.EncodeToString(digest[:]), Private: private,
	}, nil
}

func wrapAAD(userRef, dataRef string) []byte {
	return []byte("authling:key-wrap:v1\x00" + userRef + "\x00" + dataRef + "\x00credentials")
}
