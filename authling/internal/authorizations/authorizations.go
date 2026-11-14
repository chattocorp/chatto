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
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/chattocorp/chatto/authling/internal/evtstream"
	"github.com/chattocorp/chatto/authling/internal/ids"
	"github.com/chattocorp/chatto/authling/internal/keyvault"
	corev1 "github.com/chattocorp/chatto/authling/internal/pb/authling/core/v1"
	"github.com/chattocorp/chatto/pkg/datacrypto"
	"github.com/chattocorp/chatto/pkg/events"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ConsentVersion identifies scope, API resource, and offline-access disclosure.
// Increase it before expanding the claim-release policy.
const ConsentVersion uint32 = 3

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
	ID         string
	AccountID  string
	ClientName string
	ClientHost string
	// Permissions binds each approved resource ("" for UserInfo only) to its
	// consented scopes, avoiding cross-resource expansion. Resources lists its
	// keys in sorted order.
	Permissions map[string][]string
	// PasswordVersion is the latest account creation or password-change event ID.
	// It is projection state, not a grant fact; email changes do not advance it.
	PasswordVersion      string
	Resources            []string
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
	passwordVersions map[string]string
	accounts         map[string]accountKeys
	erased           map[string]bool
	byClient         map[string]map[string]Grant
	byID             map[string]map[string]Grant
	usedIDs          map[string]struct{}
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
			p.passwordVersions = make(map[string]string)
		}
		if _, exists := p.accounts[created.GetAccountId()]; exists || p.erased[created.GetAccountId()] {
			return fmt.Errorf("authorization projection saw duplicate account creation")
		}
		p.passwordVersions[created.GetAccountId()] = event.GetId()
		p.accounts[created.GetAccountId()] = accountKeys{created.GetUserKeyRef(), created.GetCredentialKeyRef()}
		return nil
	}
	if changed := event.GetPasswordChanged(); changed != nil {
		p.Lock()
		defer p.Unlock()
		if _, ok := p.accounts[changed.GetAccountId()]; !ok {
			return fmt.Errorf("password change references absent account")
		}
		p.passwordVersions[changed.GetAccountId()] = event.GetId()
		return nil
	}
	if requested := event.GetAccountErasureRequested(); requested != nil {
		p.Lock()
		defer p.Unlock()
		id := requested.GetAccountId()
		keys, ok := p.accounts[id]
		if !ok || keys.userRef != requested.GetUserKeyRef() || keys.dataRef != requested.GetCredentialKeyRef() {
			return fmt.Errorf("erasure references absent account or wrong keys")
		}
		if p.erased == nil {
			p.erased = make(map[string]bool)
		}
		p.erased[id] = true
		delete(p.accounts, id)
		delete(p.passwordVersions, id)
		delete(p.byClient, id)
		delete(p.byID, id)
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
	grant.PasswordVersion = p.passwordVersions[accountID]
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
// An empty resource means UserInfo only.
func (s *Service) Authorize(ctx context.Context, accountID string, client Client, scopes []string, resource string) (Grant, error) {
	if len(resource) > 2048 {
		return Grant{}, fmt.Errorf("invalid grant resource")
	}
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
		permissions := map[string][]string{}
		mergedScopes := append([]string(nil), scopes...)
		if active && prior.metadata.GetConsentVersion() == ConsentVersion {
			prior, err = s.openMetadata(ctx, prior)
			if err != nil {
				return Grant{}, err
			}
			permissions = prior.Permissions
			mergedScopes = union(prior.Scopes, scopes)
		}
		permissions[resource] = union(permissions[resource], scopes)
		eventID, err := ids.New("evt")
		if err != nil {
			return Grant{}, err
		}
		event := &corev1.Event{Id: eventID, CreatedAt: timestamppb.Now(), Event: &corev1.Event_OidcGrantAuthorized{OidcGrantAuthorized: &corev1.OIDCGrantAuthorizedEvent{
			AccountId: accountID, GrantId: grantID, ClientIdDigest: []byte(clientDigest),
			Scopes: mergedScopes, PriorAuthorizationEventId: priorEventID,
		}}}
		s.handle.Projection().RLock()
		keys := s.handle.Projection().accounts[accountID]
		s.handle.Projection().RUnlock()
		if err := s.sealPermissions(ctx, event, keys, client, permissions); err != nil {
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

// CoveringGrant returns the same authoritative grant snapshot that was checked.
// Keeping the permission decision and generation together prevents a concurrent
// grant replacement from binding a request to unrelated authorization.
// An empty resource means UserInfo only.
func (s *Service) CoveringGrant(ctx context.Context, accountID, clientID string, scopes []string, resource string) (Grant, bool, error) {
	if clientID == "" {
		return Grant{}, false, fmt.Errorf("invalid grant lookup")
	}
	grant, err := s.Current(ctx, accountID, clientID)
	if errors.Is(err, ErrNotFound) {
		return Grant{}, false, nil
	}
	if err != nil {
		return Grant{}, false, err
	}
	if !grant.Permits(resource, scopes) {
		return Grant{}, false, nil
	}
	return grant, true, nil
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

// clientMetadata is the encrypted grant metadata. Permissions maps each
// consented resource ("" for UserInfo only) to its scopes. Consent versions 1
// and 2 have no permissions.
type clientMetadata struct {
	Name        string              `json:"name"`
	Host        string              `json:"host"`
	Permissions map[string][]string `json:"permissions,omitempty"`
}

// maxMetadataPlaintext keeps the sealed metadata within the event validator's
// 4096-byte ciphertext limit, which includes the 16-byte authentication tag.
const maxMetadataPlaintext = 4096 - 16

// grantScopes are all scopes that a grant can hold for one resource.
var grantScopes = []string{"openid", "profile", "email", "offline_access"}

// CheckMetadataCapacity reports whether a grant for a client with this name
// and host can hold every scope for every listed resource. Configuration
// validation uses it, so that a later consent cannot exceed the capacity.
func CheckMetadataCapacity(name, host string, resources []string) error {
	permissions := map[string][]string{"": grantScopes}
	for _, resource := range resources {
		permissions[resource] = grantScopes
	}
	plain, err := json.Marshal(clientMetadata{Name: name, Host: host, Permissions: permissions})
	if err != nil {
		return err
	}
	if len(plain) > maxMetadataPlaintext {
		return fmt.Errorf("grant metadata needs %d bytes; the limit is %d", len(plain), maxMetadataPlaintext)
	}
	return nil
}

func validMetadata(name, host string) bool {
	return strings.TrimSpace(name) != "" && len(name) <= 256 && strings.TrimSpace(host) != "" && len(host) <= 256
}

func (s *Service) sealPermissions(ctx context.Context, event *corev1.Event, keys accountKeys, client Client, permissions map[string][]string) error {
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
	plain, err := json.Marshal(clientMetadata{Name: client.Name, Host: client.Host, Permissions: permissions})
	if err != nil {
		return fmt.Errorf("encode OIDC grant metadata")
	}
	defer clear(plain)
	if len(plain) > maxMetadataPlaintext {
		return fmt.Errorf("authorization metadata capacity exceeded")
	}
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
	if (grant.metadata.GetConsentVersion() < 3) != (len(metadata.Permissions) == 0) {
		return Grant{}, fmt.Errorf("invalid grant resource permissions")
	}
	resources := make([]string, 0, len(metadata.Permissions))
	for resource, permissions := range metadata.Permissions {
		if len(resource) > 2048 || len(permissions) == 0 {
			return Grant{}, fmt.Errorf("invalid resource permissions")
		}
		resources = append(resources, resource)
		for _, scope := range permissions {
			if !slices.Contains(grant.Scopes, scope) {
				return Grant{}, fmt.Errorf("invalid resource scope")
			}
		}
	}
	grant.ClientName, grant.ClientHost = metadata.Name, metadata.Host
	sort.Strings(resources)
	grant.Resources, grant.Permissions = resources, metadata.Permissions
	return grant, nil
}

// Current returns the active exact-client grant after an authoritative account-tail read.
// Callers must compare the generation and permissions before issuing credentials.
func (s *Service) Current(ctx context.Context, accountID, clientID string) (Grant, error) {
	if _, err := s.syncAccount(ctx, accountID); err != nil {
		return Grant{}, err
	}
	grant, ok := s.handle.Projection().grantForClientDigest(accountID, s.clientDigest(clientID))
	if !ok || grant.metadata.GetConsentVersion() != ConsentVersion {
		return Grant{}, ErrNotFound
	}
	return s.openMetadata(ctx, grant)
}

// Permits checks permissions for one resource; scopes granted to other targets do not apply.
func (g Grant) Permits(resource string, scopes []string) bool {
	allowed, ok := g.Permissions[resource]
	if !ok {
		return false
	}
	for _, scope := range scopes {
		if !slices.Contains(allowed, scope) {
			return false
		}
	}
	return true
}

func union(prior, added []string) []string {
	result := append([]string(nil), prior...)
	for _, value := range added {
		if !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}
