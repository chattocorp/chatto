// Package authorizations owns durable OIDC authorization grants and their
// account-facing projection.
package authorizations

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/authling/internal/evtstream"
	"hmans.de/authling/internal/ids"
	"hmans.de/authling/internal/keyvault"
	corev1 "hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/chatto/pkg/datacrypto"
	"hmans.de/chatto/pkg/events"
)

// ConsentVersion identifies disclosure of sub, preferred_username, and name.
// Increase it before expanding the claim-release policy.
const ConsentVersion uint32 = 1

// ErrNotFound indicates that an account or active authorization grant is not
// available to the command.
var ErrNotFound = errors.New("authorization grant not found")

// Client is validated OIDC client metadata supplied to a grant command. ID is
// reduced to a deployment-keyed digest before persistence; future relying-
// party grouping may deliberately place more than one client behind a grant.
type Client struct {
	ID   string
	Name string
	Host string
}

// Grant is one active account-owned authorization for an exact OIDC client.
type Grant struct {
	ID                   string
	AccountID            string
	ClientName           string
	ClientHost           string
	Scopes               []string
	AuthorizedAt         time.Time
	AuthorizationEventID string
	clientDigest         string
	metadata             *corev1.OIDCGrantAuthorizedEvent
}

type accountKeys struct{ userRef, dataRef string }

// Projection rebuilds active OIDC grants from account aggregate history.
type Projection struct {
	events.MemoryProjection
	accounts map[string]accountKeys
	byClient map[string]map[string]Grant
	byID     map[string]map[string]Grant
	usedIDs  map[string]struct{}
}

// NewProjection creates an empty authorization-grant projection.
func NewProjection() *Projection { return &Projection{} }

// Subjects returns the account event family consumed by this projection.
func (*Projection) Subjects() []string { return []string{evtstream.AccountSubjectFilter} }

// Apply adds one durable account fact to the authorization-grant model.
func (p *Projection) Apply(event *corev1.Event, _ uint64) error {
	if created := event.GetAccountCreated(); created != nil {
		p.Lock()
		defer p.Unlock()
		if p.accounts == nil {
			p.accounts = make(map[string]accountKeys)
		}
		if _, exists := p.accounts[created.GetAccountId()]; exists {
			return fmt.Errorf("authorization projection saw duplicate account creation")
		}
		p.accounts[created.GetAccountId()] = accountKeys{created.GetUserKeyRef(), created.GetCredentialKeyRef()}
		return nil
	}
	if authorized := event.GetOidcGrantAuthorized(); authorized != nil {
		grant := Grant{
			ID:                   authorized.GetGrantId(),
			AccountID:            authorized.GetAccountId(),
			Scopes:               append([]string(nil), authorized.GetScopes()...),
			AuthorizedAt:         event.GetCreatedAt().AsTime(),
			AuthorizationEventID: event.GetId(),
			clientDigest:         string(authorized.GetClientIdDigest()),
			metadata:             proto.Clone(authorized).(*corev1.OIDCGrantAuthorizedEvent),
		}
		p.Lock()
		defer p.Unlock()
		if _, exists := p.accounts[grant.AccountID]; !exists {
			return fmt.Errorf("OIDC grant authorization references an absent account")
		}
		keys := p.accounts[grant.AccountID]
		if keys.userRef == "" || keys.dataRef == "" || keys.userRef != authorized.GetUserKeyRef() || keys.dataRef != authorized.GetCredentialKeyRef() {
			return fmt.Errorf("OIDC grant metadata references another account key hierarchy")
		}
		if p.byClient == nil {
			p.byClient = make(map[string]map[string]Grant)
			p.byID = make(map[string]map[string]Grant)
			p.usedIDs = make(map[string]struct{})
		}
		clients := p.byClient[grant.AccountID]
		if clients == nil {
			clients = make(map[string]Grant)
			p.byClient[grant.AccountID] = clients
		}
		grants := p.byID[grant.AccountID]
		if grants == nil {
			grants = make(map[string]Grant)
			p.byID[grant.AccountID] = grants
		}
		priorID := authorized.GetPriorAuthorizationEventId()
		prior, active := clients[grant.clientDigest]
		if priorID == "" {
			if active {
				return fmt.Errorf("new OIDC grant overlaps an active client grant")
			}
			if _, used := p.usedIDs[grant.ID]; used {
				return fmt.Errorf("OIDC grant id was reused after its generation ended")
			}
		} else if !active || prior.ID != grant.ID || prior.AuthorizationEventID != priorID {
			return fmt.Errorf("OIDC grant renewal references another active authorization")
		}
		if existing, exists := grants[grant.ID]; exists && existing.clientDigest != grant.clientDigest {
			return fmt.Errorf("OIDC grant id references multiple clients")
		}
		p.usedIDs[grant.ID] = struct{}{}
		clients[grant.clientDigest] = grant
		grants[grant.ID] = grant
		return nil
	}
	if revoked := event.GetOidcGrantRevoked(); revoked != nil {
		p.Lock()
		defer p.Unlock()
		if _, exists := p.accounts[revoked.GetAccountId()]; !exists {
			return fmt.Errorf("OIDC grant revocation references an absent account")
		}
		grant, exists := p.byID[revoked.GetAccountId()][revoked.GetGrantId()]
		if !exists || grant.AuthorizationEventID != revoked.GetAuthorizationEventId() {
			return fmt.Errorf("OIDC grant revocation references another active authorization")
		}
		delete(p.byID[grant.AccountID], grant.ID)
		delete(p.byClient[grant.AccountID], grant.clientDigest)
		return nil
	}
	// Other account events are historical facts owned by sibling projections.
	return nil
}

func (p *Projection) hasAccount(accountID string) bool {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.accounts[accountID]
	return ok
}

func (p *Projection) grantForClientDigest(accountID, clientDigest string) (Grant, bool) {
	p.RLock()
	defer p.RUnlock()
	grant, ok := p.byClient[accountID][clientDigest]
	grant.Scopes = append([]string(nil), grant.Scopes...)
	return grant, ok
}

func (p *Projection) grantByID(accountID, grantID string) (Grant, bool) {
	p.RLock()
	defer p.RUnlock()
	grant, ok := p.byID[accountID][grantID]
	grant.Scopes = append([]string(nil), grant.Scopes...)
	return grant, ok
}

func (p *Projection) list(accountID string) []Grant {
	p.RLock()
	defer p.RUnlock()
	grants := make([]Grant, 0, len(p.byID[accountID]))
	for _, grant := range p.byID[accountID] {
		grant.Scopes = append([]string(nil), grant.Scopes...)
		grants = append(grants, grant)
	}
	sort.Slice(grants, func(i, j int) bool {
		if grants[i].AuthorizedAt.Equal(grants[j].AuthorizedAt) {
			return grants[i].ID < grants[j].ID
		}
		return grants[i].AuthorizedAt.After(grants[j].AuthorizedAt)
	})
	return grants
}

// Service validates grant commands, commits them with account OCC, and serves
// read-your-writes grant decisions.
type Service struct {
	publisher *evtstream.Publisher
	handle    events.ProjectionHandle[*Projection]
	indexKey  []byte
	vault     *keyvault.Vault
}

// NewService constructs the OIDC authorization-grant boundary.
func NewService(publisher *evtstream.Publisher, handle events.ProjectionHandle[*Projection], indexKey []byte, vault *keyvault.Vault) (*Service, error) {
	if publisher == nil || handle.Projector() == nil || len(indexKey) != 32 || vault == nil {
		return nil, fmt.Errorf("authorization grant dependencies are incomplete")
	}
	return &Service{publisher: publisher, handle: handle, indexKey: append([]byte(nil), indexKey...), vault: vault}, nil
}

// Authorize records explicit consent. Re-consenting renews the current grant;
// authorizing after revocation creates a new grant generation.
func (s *Service) Authorize(ctx context.Context, accountID string, client Client, scopes []string) (Grant, error) {
	if client.ID == "" || !validMetadata(client.Name, client.Host) {
		return Grant{}, fmt.Errorf("OIDC client metadata is invalid")
	}
	clientDigest := s.clientDigest(client.ID)
	for range 5 {
		tail, err := s.syncAccount(ctx, accountID)
		if err != nil {
			return Grant{}, err
		}
		prior, active := s.handle.Projection().grantForClientDigest(accountID, clientDigest)
		grantID := prior.ID
		priorEventID := prior.AuthorizationEventID
		if !active {
			grantID, err = ids.New("grant")
			if err != nil {
				return Grant{}, err
			}
			priorEventID = ""
		}
		eventID, err := ids.New("evt")
		if err != nil {
			return Grant{}, err
		}
		event := &corev1.Event{Id: eventID, CreatedAt: timestamppb.Now(), Event: &corev1.Event_OidcGrantAuthorized{OidcGrantAuthorized: &corev1.OIDCGrantAuthorizedEvent{
			AccountId: accountID, GrantId: grantID, ClientIdDigest: []byte(clientDigest),
			Scopes: append([]string(nil), scopes...), PriorAuthorizationEventId: priorEventID,
		}}}
		s.handle.Projection().RLock()
		keys := s.handle.Projection().accounts[accountID]
		s.handle.Projection().RUnlock()
		if err := s.sealMetadata(ctx, event, keys, client); err != nil {
			return Grant{}, err
		}
		position, err := s.publisher.AppendOIDCGrantAuthorized(ctx, event, tail)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return Grant{}, fmt.Errorf("commit OIDC grant authorization: %w", err)
		}
		if err := s.handle.Projector().WaitFor(ctx, position); err != nil {
			return Grant{}, fmt.Errorf("wait for OIDC grant authorization: %w", err)
		}
		grant, ok := s.handle.Projection().grantForClientDigest(accountID, clientDigest)
		if !ok || grant.ID != grantID {
			return Grant{}, fmt.Errorf("authorized OIDC grant is absent from projection")
		}
		return s.openMetadata(ctx, grant)
	}
	return Grant{}, fmt.Errorf("OIDC grant authorization conflict")
}

// Covers reports whether the active client grant contains every requested
// scope after synchronizing to the account aggregate tail.
func (s *Service) Covers(ctx context.Context, accountID, clientID string, scopes []string) (bool, error) {
	if clientID == "" {
		return false, fmt.Errorf("OIDC client id is required")
	}
	if _, err := s.syncAccount(ctx, accountID); err != nil {
		return false, err
	}
	grant, ok := s.handle.Projection().grantForClientDigest(accountID, s.clientDigest(clientID))
	if !ok || grant.metadata.GetConsentVersion() != ConsentVersion {
		return false, nil
	}
	if _, err := s.openMetadata(ctx, grant); err != nil {
		return false, err
	}
	have := make(map[string]struct{}, len(grant.Scopes))
	for _, scope := range grant.Scopes {
		have[scope] = struct{}{}
	}
	for _, scope := range scopes {
		if _, ok := have[scope]; !ok {
			return false, nil
		}
	}
	return true, nil
}

// List returns the account's active grants after synchronizing to its durable
// tail.
func (s *Service) List(ctx context.Context, accountID string) ([]Grant, error) {
	if _, err := s.syncAccount(ctx, accountID); err != nil {
		return nil, err
	}
	grants := s.handle.Projection().list(accountID)
	for i, grant := range grants {
		opened, err := s.openMetadata(ctx, grant)
		if err != nil {
			return nil, err
		}
		grants[i] = opened
	}
	return grants, nil
}

// Revoke ends one active grant owned by accountID.
func (s *Service) Revoke(ctx context.Context, accountID, grantID string) error {
	for range 5 {
		tail, err := s.syncAccount(ctx, accountID)
		if err != nil {
			return err
		}
		grant, ok := s.handle.Projection().grantByID(accountID, grantID)
		if !ok {
			return ErrNotFound
		}
		eventID, err := ids.New("evt")
		if err != nil {
			return err
		}
		event := &corev1.Event{Id: eventID, CreatedAt: timestamppb.Now(), Event: &corev1.Event_OidcGrantRevoked{OidcGrantRevoked: &corev1.OIDCGrantRevokedEvent{
			AccountId: accountID, GrantId: grant.ID, AuthorizationEventId: grant.AuthorizationEventID,
		}}}
		position, err := s.publisher.AppendOIDCGrantRevoked(ctx, event, tail)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return fmt.Errorf("commit OIDC grant revocation: %w", err)
		}
		if err := s.handle.Projector().WaitFor(ctx, position); err != nil {
			return fmt.Errorf("wait for OIDC grant revocation: %w", err)
		}
		if _, ok := s.handle.Projection().grantByID(accountID, grantID); ok {
			return fmt.Errorf("revoked OIDC grant remains in projection")
		}
		return nil
	}
	return fmt.Errorf("OIDC grant revocation conflict")
}

func (s *Service) clientDigest(clientID string) string {
	digest := hmac.New(sha256.New, s.indexKey)
	_, _ = digest.Write([]byte("authling:oidc-client-index:v1\x00"))
	_, _ = digest.Write([]byte(clientID))
	return string(digest.Sum(nil))
}

func (s *Service) syncAccount(ctx context.Context, accountID string) (uint64, error) {
	tail, err := s.publisher.AccountTail(ctx, accountID)
	if err != nil {
		return 0, fmt.Errorf("read account tail: %w", err)
	}
	if tail == 0 {
		return 0, ErrNotFound
	}
	subject, err := evtstream.AccountSubject(accountID)
	if err != nil {
		return 0, ErrNotFound
	}
	if err := s.handle.Projector().WaitFor(ctx, events.SubjectPosition(subject, tail)); err != nil {
		return 0, fmt.Errorf("wait for authorization grants: %w", err)
	}
	if !s.handle.Projection().hasAccount(accountID) {
		return 0, ErrNotFound
	}
	return tail, nil
}

// metadataAAD binds the encrypted display snapshot to its complete durable
// authorization context. JSON encoding keeps variable-length fields distinct.
func metadataAAD(eventID string, grant *corev1.OIDCGrantAuthorizedEvent) []byte {
	data, _ := json.Marshal([]any{"authling:oidc-grant-metadata:v1", eventID,
		grant.GetAccountId(), grant.GetGrantId(), grant.GetClientIdDigest(),
		grant.GetScopes(), grant.GetPriorAuthorizationEventId(), grant.GetConsentVersion(),
		grant.GetUserKeyRef(), grant.GetCredentialKeyRef()})
	return data
}

type clientMetadata struct {
	Name string `json:"name"`
	Host string `json:"host"`
}

func validMetadata(name, host string) bool {
	return strings.TrimSpace(name) != "" && len(name) <= 256 && strings.TrimSpace(host) != "" && len(host) <= 256
}

func (s *Service) sealMetadata(ctx context.Context, event *corev1.Event, keys accountKeys, client Client) error {
	if keys.userRef == "" || keys.dataRef == "" {
		return fmt.Errorf("OIDC grant requires account encryption keys")
	}
	key, err := s.vault.ResolveDataKey(ctx, keys.dataRef, keys.userRef)
	if err != nil {
		return fmt.Errorf("resolve OIDC grant metadata key: %w", err)
	}
	defer clear(key)
	grant := event.GetOidcGrantAuthorized()
	grant.MetadataEnvelopeVersion, grant.ConsentVersion = 1, ConsentVersion
	grant.UserKeyRef, grant.CredentialKeyRef = keys.userRef, keys.dataRef
	plain, err := json.Marshal(clientMetadata{client.Name, client.Host})
	if err != nil {
		return fmt.Errorf("encode OIDC grant metadata")
	}
	defer clear(plain)
	sealed, err := datacrypto.Seal(key, plain, metadataAAD(event.GetId(), grant))
	if err != nil {
		return fmt.Errorf("protect OIDC grant metadata: %w", err)
	}
	grant.MetadataNonce, grant.MetadataCiphertext = sealed.Nonce, sealed.Ciphertext
	return nil
}

// openMetadata decrypts only at the service read boundary. The projection
// retains ciphertext; grant metadata is never read from plaintext fields.
func (s *Service) openMetadata(ctx context.Context, grant Grant) (Grant, error) {
	key, err := s.vault.ResolveDataKey(ctx, grant.metadata.GetCredentialKeyRef(), grant.metadata.GetUserKeyRef())
	if err != nil {
		return Grant{}, fmt.Errorf("resolve OIDC grant metadata key: %w", err)
	}
	defer clear(key)
	plain, err := datacrypto.Open(key, grant.metadata.GetMetadataCiphertext(), grant.metadata.GetMetadataNonce(), metadataAAD(grant.AuthorizationEventID, grant.metadata))
	if err != nil {
		return Grant{}, fmt.Errorf("open OIDC grant metadata: %w", err)
	}
	defer clear(plain)
	var metadata clientMetadata
	if json.Unmarshal(plain, &metadata) != nil || !validMetadata(metadata.Name, metadata.Host) {
		return Grant{}, fmt.Errorf("invalid OIDC grant metadata")
	}
	grant.ClientName, grant.ClientHost = metadata.Name, metadata.Host
	return grant, nil
}
