package core

import (
	"sort"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// UserAuthProjection retains credential and external-identity state. It is a
// separate cold-replay projection so profile snapshots can never serialize
// authentication material by construction.
type UserAuthProjection struct {
	events.MemoryProjection
	users         map[string]*projectedUserAuth
	identityIndex map[string]string
	replayGuard   projectionReplayGuard
}

type projectedUserAuth struct {
	deleted            bool
	passwordHash       []byte
	passwordSetAt      time.Time
	authGeneration     uint64
	externalIdentities map[string]ExternalIdentity
	oauthConsent       map[string]struct{}
	botAPIKeyHash      string
	botAPIKeyCreatedAt time.Time
	botAPIKeyIntentSeq uint64
}

type botAPIKeyIntent struct {
	TokenHash string
	CreatedAt time.Time
	Sequence  uint64
	Active    bool
}

func newUserAuthProjection() *UserAuthProjection {
	return &UserAuthProjection{
		users:         make(map[string]*projectedUserAuth),
		identityIndex: make(map[string]string),
		replayGuard:   newProjectionReplayGuard(),
	}
}

func (p *UserAuthProjection) Subjects() []string {
	return []string{
		events.UserEventTypeFilter(events.EventUserAccountCreated),
		events.UserEventTypeFilter(events.EventUserPasswordHashChanged),
		events.UserEventTypeFilter(events.EventUserOIDCSubjectLinked),
		events.UserEventTypeFilter(events.EventUserExternalIdentityLinked),
		events.UserEventTypeFilter(events.EventUserExternalIdentityUnlinked),
		events.UserEventTypeFilter(events.EventOAuthConsentGranted),
		events.UserEventTypeFilter(events.EventBotAPIKeyRotated),
		events.UserEventTypeFilter(events.EventBotAPIKeyRevoked),
		events.UserEventTypeFilter(events.EventUserAccountDeleted),
		events.UserEventTypeFilter(events.EventUserKeyShredded),
	}
}

func (p *UserAuthProjection) Apply(event *corev1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	if p.replayGuard.seenOrMark(event, seq) {
		return nil
	}
	switch e := event.GetEvent().(type) {
	case *corev1.Event_UserAccountCreated:
		if e.UserAccountCreated != nil {
			p.ensureUserLocked(e.UserAccountCreated.GetUserId()).deleted = false
		}
	case *corev1.Event_UserPasswordHashChanged:
		p.applyPasswordHashChanged(e.UserPasswordHashChanged, event.GetCreatedAt(), seq)
	case *corev1.Event_UserOidcSubjectLinked:
		p.applyOIDCSubjectLinked(e.UserOidcSubjectLinked)
	case *corev1.Event_UserExternalIdentityLinked:
		p.applyExternalIdentityLinked(e.UserExternalIdentityLinked)
	case *corev1.Event_UserExternalIdentityUnlinked:
		p.applyExternalIdentityUnlinked(e.UserExternalIdentityUnlinked, seq)
	case *corev1.Event_OauthConsentGranted:
		p.applyOAuthConsentGranted(e.OauthConsentGranted)
	case *corev1.Event_BotApiKeyRotated:
		p.applyBotAPIKeyRotated(e.BotApiKeyRotated, event.GetCreatedAt(), seq)
	case *corev1.Event_BotApiKeyRevoked:
		p.applyBotAPIKeyRevoked(e.BotApiKeyRevoked, seq)
	case *corev1.Event_UserAccountDeleted:
		p.applyAccountDeleted(e.UserAccountDeleted, seq)
	case *corev1.Event_UserKeyShredded:
		p.applyKeyShredded(e.UserKeyShredded)
	}
	return nil
}

func (p *UserAuthProjection) applyBotAPIKeyRotated(e *corev1.BotAPIKeyRotatedEvent, createdAt *timestamppb.Timestamp, seq uint64) {
	if e == nil || e.GetUserId() == "" || e.GetTokenHash() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.botAPIKeyHash = e.GetTokenHash()
	u.botAPIKeyIntentSeq = seq
	u.botAPIKeyCreatedAt = time.Time{}
	if createdAt != nil {
		u.botAPIKeyCreatedAt = createdAt.AsTime()
	}
}

func (p *UserAuthProjection) applyBotAPIKeyRevoked(e *corev1.BotAPIKeyRevokedEvent, seq uint64) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.botAPIKeyHash = ""
	u.botAPIKeyCreatedAt = time.Time{}
	u.botAPIKeyIntentSeq = seq
}

func (p *UserAuthProjection) CompleteStartupReplay() {
	p.Lock()
	defer p.Unlock()
	p.replayGuard.completeReplay()
}

func (p *UserAuthProjection) ensureUserLocked(userID string) *projectedUserAuth {
	u := p.users[userID]
	if u == nil {
		u = &projectedUserAuth{}
		p.users[userID] = u
	}
	if u.externalIdentities == nil {
		u.externalIdentities = make(map[string]ExternalIdentity)
	}
	if u.oauthConsent == nil {
		u.oauthConsent = make(map[string]struct{})
	}
	return u
}

func (p *UserAuthProjection) applyPasswordHashChanged(e *corev1.UserPasswordHashChangedEvent, createdAt *timestamppb.Timestamp, seq uint64) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.passwordHash = append(u.passwordHash[:0], e.GetPasswordHash()...)
	if !e.GetPreserveExistingCredentials() {
		u.authGeneration = seq
		u.passwordSetAt = time.Time{}
		if createdAt != nil {
			u.passwordSetAt = createdAt.AsTime()
		}
	}
}

func (p *UserAuthProjection) applyOIDCSubjectLinked(e *corev1.UserOIDCSubjectLinkedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	hash := e.GetSubjectHash()
	if hash == "" && e.GetIssuer() != "" && e.GetSubject() != "" {
		hash = externalIdentityHash(e.GetIssuer(), e.GetSubject())
	}
	if hash == "" {
		return
	}
	p.identityIndex[hash] = e.GetUserId()
	u := p.ensureUserLocked(e.GetUserId())
	u.externalIdentities[hash] = ExternalIdentity{ProviderID: "oidc", ProviderType: "oidc", Issuer: e.GetIssuer(), Subject: e.GetSubject(), SubjectHash: hash}
}

func (p *UserAuthProjection) applyExternalIdentityLinked(e *corev1.UserExternalIdentityLinkedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	hash := e.GetSubjectHash()
	if hash == "" && e.GetIssuer() != "" && e.GetSubject() != "" {
		hash = externalIdentityHash(e.GetIssuer(), e.GetSubject())
	}
	if hash == "" {
		return
	}
	providerID := e.GetProviderId()
	if providerID == "" {
		providerID = e.GetIssuer()
	}
	providerType := e.GetProviderType()
	if providerType == "" {
		providerType = providerID
	}
	p.identityIndex[hash] = e.GetUserId()
	p.ensureUserLocked(e.GetUserId()).externalIdentities[hash] = ExternalIdentity{
		ProviderID: providerID, ProviderType: providerType, Issuer: e.GetIssuer(), Subject: e.GetSubject(), SubjectHash: hash,
	}
}

func (p *UserAuthProjection) applyExternalIdentityUnlinked(e *corev1.UserExternalIdentityUnlinkedEvent, seq uint64) {
	if e == nil || e.GetUserId() == "" || e.GetSubjectHash() == "" {
		return
	}
	if p.identityIndex[e.GetSubjectHash()] == e.GetUserId() {
		delete(p.identityIndex, e.GetSubjectHash())
	}
	u := p.ensureUserLocked(e.GetUserId())
	delete(u.externalIdentities, e.GetSubjectHash())
	u.authGeneration = seq
}

func (p *UserAuthProjection) applyOAuthConsentGranted(e *corev1.OAuthConsentGrantedEvent) {
	if e == nil || e.GetUserId() == "" || e.GetRedirectOrigin() == "" {
		return
	}
	p.ensureUserLocked(e.GetUserId()).oauthConsent[e.GetRedirectOrigin()] = struct{}{}
}

func (p *UserAuthProjection) applyAccountDeleted(e *corev1.UserAccountDeletedEvent, seq uint64) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.deleted = true
	u.authGeneration = seq
	u.passwordHash = nil
	u.passwordSetAt = time.Time{}
	u.externalIdentities = make(map[string]ExternalIdentity)
	u.oauthConsent = make(map[string]struct{})
	u.botAPIKeyHash = ""
	u.botAPIKeyCreatedAt = time.Time{}
	u.botAPIKeyIntentSeq = seq
	p.deleteIdentityIndexLocked(e.GetUserId())
}

func (p *UserAuthProjection) BotAPIKeyIntent(userID string) (botAPIKeyIntent, bool) {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || u.botAPIKeyIntentSeq == 0 {
		return botAPIKeyIntent{}, false
	}
	return botAPIKeyIntent{TokenHash: u.botAPIKeyHash, CreatedAt: u.botAPIKeyCreatedAt, Sequence: u.botAPIKeyIntentSeq, Active: u.botAPIKeyHash != ""}, true
}

func (p *UserAuthProjection) applyKeyShredded(e *corev1.UserKeyShreddedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.passwordHash = nil
	u.passwordSetAt = time.Time{}
	u.externalIdentities = make(map[string]ExternalIdentity)
	u.oauthConsent = make(map[string]struct{})
	u.botAPIKeyHash = ""
	u.botAPIKeyCreatedAt = time.Time{}
	p.deleteIdentityIndexLocked(e.GetUserId())
}

func (p *UserAuthProjection) deleteIdentityIndexLocked(userID string) {
	for hash, owner := range p.identityIndex {
		if owner == userID {
			delete(p.identityIndex, hash)
		}
	}
}

func (p *UserAuthProjection) ExternalIdentityOwnerID(issuer, subject string) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	userID := p.identityIndex[externalIdentityHash(issuer, subject)]
	return userID, userID != ""
}

func (p *UserAuthProjection) ExternalIdentities(userID string) []ExternalIdentity {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || len(u.externalIdentities) == 0 {
		return nil
	}
	identities := make([]ExternalIdentity, 0, len(u.externalIdentities))
	for _, identity := range u.externalIdentities {
		identities = append(identities, identity)
	}
	sort.Slice(identities, func(i, j int) bool {
		if identities[i].ProviderID != identities[j].ProviderID {
			return identities[i].ProviderID < identities[j].ProviderID
		}
		return identities[i].SubjectHash < identities[j].SubjectHash
	})
	return identities
}

func (p *UserAuthProjection) PasswordHashWithSetAt(userID string) ([]byte, time.Time, bool) {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || len(u.passwordHash) == 0 {
		return nil, time.Time{}, false
	}
	return append([]byte(nil), u.passwordHash...), u.passwordSetAt, true
}

func (p *UserAuthProjection) AuthGeneration(userID string) (uint64, bool) {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted {
		return 0, false
	}
	return u.authGeneration, true
}

func (p *UserAuthProjection) HasExternalIdentity(userID string) bool {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	return u != nil && !u.deleted && len(u.externalIdentities) > 0
}

func (p *UserAuthProjection) HasOAuthConsent(userID, redirectOrigin string) bool {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || redirectOrigin == "" {
		return false
	}
	_, ok := u.oauthConsent[redirectOrigin]
	return ok
}

func (p *UserAuthProjection) VerifiedAccountIDs() []string {
	p.RLock()
	defer p.RUnlock()
	seen := make(map[string]struct{})
	for _, userID := range p.identityIndex {
		if u := p.users[userID]; u != nil && !u.deleted {
			seen[userID] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for userID := range seen {
		out = append(out, userID)
	}
	sort.Strings(out)
	return out
}

func (p *UserAuthProjection) IdentityCount() int {
	p.RLock()
	defer p.RUnlock()
	return len(p.identityIndex)
}
