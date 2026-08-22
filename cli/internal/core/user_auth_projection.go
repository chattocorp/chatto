package core

import (
	"bytes"
	"sort"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

// UserAuthProjection retains credential and external-identity state. It is a
// separate cold-replay projection so profile snapshots can never serialize
// authentication material by construction.
type UserAuthProjection struct {
	events.MemoryProjection
	users                   map[string]*projectedUserAuth
	identityIndex           map[string]string
	replayGuard             projectionReplayGuard
	nextBotAPIKeyWatcherID  uint64
	botAPIKeyWatchersByUser map[string]map[uint64]botAPIKeyWatcher
}

type botAPIKeyWatcher struct {
	verifier    []byte
	invalidated chan struct{}
}

type projectedUserAuth struct {
	deleted            bool
	isBot              bool
	botOwnerUserID     string
	botAPIKeyVerifier  []byte
	botAPIKeyCreatedAt time.Time
	botAPIKeyRotatedAt time.Time
	passwordHash       []byte
	passwordSetAt      time.Time
	authGeneration     uint64
	externalIdentities map[string]ExternalIdentity
	oauthConsent       map[string]struct{}
}

func newUserAuthProjection() *UserAuthProjection {
	return &UserAuthProjection{
		users:                   make(map[string]*projectedUserAuth),
		identityIndex:           make(map[string]string),
		replayGuard:             newProjectionReplayGuard(),
		botAPIKeyWatchersByUser: make(map[string]map[uint64]botAPIKeyWatcher),
	}
}

func (p *UserAuthProjection) Subjects() []string {
	return []string{
		evtstream.UserEventTypeFilter(evtstream.EventUserAccountCreated),
		evtstream.UserEventTypeFilter(evtstream.EventUserPasswordHashChanged),
		evtstream.UserEventTypeFilter(evtstream.EventUserOIDCSubjectLinked),
		evtstream.UserEventTypeFilter(evtstream.EventUserExternalIdentityLinked),
		evtstream.UserEventTypeFilter(evtstream.EventUserExternalIdentityUnlinked),
		evtstream.UserEventTypeFilter(evtstream.EventOAuthConsentGranted),
		evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted),
		evtstream.UserEventTypeFilter(evtstream.EventUserKeyShreddingRequested),
		evtstream.UserEventTypeFilter(evtstream.EventUserKeyShredded),
		evtstream.UserEventTypeFilter(evtstream.EventBotAPIKeyCreated),
		evtstream.UserEventTypeFilter(evtstream.EventBotAPIKeyRotated),
		evtstream.UserEventTypeFilter(evtstream.EventBotOwnerReassigned),
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
			u := p.ensureUserLocked(e.UserAccountCreated.GetUserId())
			u.isBot = e.UserAccountCreated.GetIsBot()
			u.botOwnerUserID = e.UserAccountCreated.GetBotOwnerUserId()
		}
	case *corev1.Event_BotApiKeyCreated:
		p.applyBotAPIKeyCreated(e.BotApiKeyCreated, event.GetCreatedAt())
	case *corev1.Event_BotApiKeyRotated:
		p.applyBotAPIKeyRotated(e.BotApiKeyRotated, event.GetCreatedAt())
	case *corev1.Event_BotOwnerReassigned:
		if e.BotOwnerReassigned != nil {
			u := p.ensureUserLocked(e.BotOwnerReassigned.GetUserId())
			if u.isBot && !u.deleted {
				u.botOwnerUserID = e.BotOwnerReassigned.GetOwnerUserId()
			}
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
	case *corev1.Event_UserAccountDeleted:
		p.applyAccountDeleted(e.UserAccountDeleted, seq)
	case *corev1.Event_UserKeyShreddingRequested:
		p.applyKeyShredded(e.UserKeyShreddingRequested.GetUserId(), seq)
	case *corev1.Event_UserKeyShredded:
		p.applyKeyShredded(e.UserKeyShredded.GetUserId(), seq)
	}
	return nil
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
	if u.deleted {
		return
	}
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
	u := p.ensureUserLocked(e.GetUserId())
	if u.deleted {
		return
	}
	p.identityIndex[hash] = e.GetUserId()
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
	u := p.ensureUserLocked(e.GetUserId())
	if u.deleted {
		return
	}
	p.identityIndex[hash] = e.GetUserId()
	u.externalIdentities[hash] = ExternalIdentity{
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
	if u.deleted {
		return
	}
	delete(u.externalIdentities, e.GetSubjectHash())
	u.authGeneration = seq
}

func (p *UserAuthProjection) applyOAuthConsentGranted(e *corev1.OAuthConsentGrantedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	key := OAuthConsentKey(e.GetClientId(), e.GetRedirectOrigin())
	if key == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	if u.deleted {
		return
	}
	u.oauthConsent[key] = struct{}{}
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
	u.botAPIKeyVerifier = nil
	u.botAPIKeyCreatedAt = time.Time{}
	u.botAPIKeyRotatedAt = time.Time{}
	p.closeBotAPIKeyWatchersLocked(e.GetUserId())
	p.deleteIdentityIndexLocked(e.GetUserId())
}

func (p *UserAuthProjection) applyKeyShredded(userID string, seq uint64) {
	if userID == "" {
		return
	}
	u := p.ensureUserLocked(userID)
	u.deleted = true
	u.authGeneration = seq
	u.passwordHash = nil
	u.passwordSetAt = time.Time{}
	u.externalIdentities = make(map[string]ExternalIdentity)
	u.oauthConsent = make(map[string]struct{})
	u.botAPIKeyVerifier = nil
	u.botAPIKeyCreatedAt = time.Time{}
	u.botAPIKeyRotatedAt = time.Time{}
	p.closeBotAPIKeyWatchersLocked(userID)
	p.deleteIdentityIndexLocked(userID)
}

func (p *UserAuthProjection) applyBotAPIKeyCreated(e *corev1.BotApiKeyCreatedEvent, createdAt *timestamppb.Timestamp) {
	if e == nil || e.GetUserId() == "" || len(e.GetVerifier()) == 0 {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	if u.deleted || !u.isBot {
		return
	}
	u.botAPIKeyVerifier = append(u.botAPIKeyVerifier[:0], e.GetVerifier()...)
	u.botAPIKeyCreatedAt = timestampTime(createdAt)
	u.botAPIKeyRotatedAt = time.Time{}
}

func (p *UserAuthProjection) applyBotAPIKeyRotated(e *corev1.BotApiKeyRotatedEvent, createdAt *timestamppb.Timestamp) {
	if e == nil || e.GetUserId() == "" || len(e.GetVerifier()) == 0 {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	if u.deleted || !u.isBot || len(u.botAPIKeyVerifier) == 0 {
		return
	}
	nextVerifier := e.GetVerifier()
	for watcherID, watcher := range p.botAPIKeyWatchersByUser[e.GetUserId()] {
		if bytes.Equal(watcher.verifier, nextVerifier) {
			continue
		}
		close(watcher.invalidated)
		delete(p.botAPIKeyWatchersByUser[e.GetUserId()], watcherID)
	}
	if len(p.botAPIKeyWatchersByUser[e.GetUserId()]) == 0 {
		delete(p.botAPIKeyWatchersByUser, e.GetUserId())
	}
	u.botAPIKeyVerifier = append(u.botAPIKeyVerifier[:0], nextVerifier...)
	u.botAPIKeyRotatedAt = timestampTime(createdAt)
}

func timestampTime(value *timestamppb.Timestamp) time.Time {
	if value == nil || !value.IsValid() {
		return time.Time{}
	}
	return value.AsTime()
}

// BotAPIKeyCredential is the projected verifier and safe metadata for one bot.
type BotAPIKeyCredential struct {
	Verifier  []byte
	CreatedAt time.Time
	RotatedAt time.Time
}

func (p *UserAuthProjection) BotAPIKeyCredential(userID string) (BotAPIKeyCredential, bool) {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || !u.isBot || len(u.botAPIKeyVerifier) == 0 {
		return BotAPIKeyCredential{}, false
	}
	return BotAPIKeyCredential{
		Verifier:  append([]byte(nil), u.botAPIKeyVerifier...),
		CreatedAt: u.botAPIKeyCreatedAt,
		RotatedAt: u.botAPIKeyRotatedAt,
	}, true
}

// watchBotAPIKeyInvalidated registers a process-local notification backed by
// the durable user-auth projection. Registration and the current-verifier
// check share the projection lock, so a concurrent rotation cannot leave a
// stale realtime connection unwatched.
func (p *UserAuthProjection) watchBotAPIKeyInvalidated(userID string, verifier []byte) (<-chan struct{}, func()) {
	invalidated := make(chan struct{})
	p.Lock()
	u := p.users[userID]
	if u == nil || u.deleted || !u.isBot ||
		!bytes.Equal(u.botAPIKeyVerifier, verifier) {
		close(invalidated)
		p.Unlock()
		return invalidated, func() {}
	}
	p.nextBotAPIKeyWatcherID++
	watcherID := p.nextBotAPIKeyWatcherID
	watchers := p.botAPIKeyWatchersByUser[userID]
	if watchers == nil {
		watchers = make(map[uint64]botAPIKeyWatcher)
		p.botAPIKeyWatchersByUser[userID] = watchers
	}
	watchers[watcherID] = botAPIKeyWatcher{
		verifier:    append([]byte(nil), verifier...),
		invalidated: invalidated,
	}
	p.Unlock()

	var cancelOnce sync.Once
	return invalidated, func() {
		cancelOnce.Do(func() {
			p.Lock()
			if watchers := p.botAPIKeyWatchersByUser[userID]; watchers != nil {
				delete(watchers, watcherID)
				if len(watchers) == 0 {
					delete(p.botAPIKeyWatchersByUser, userID)
				}
			}
			p.Unlock()
		})
	}
}

func (p *UserAuthProjection) closeBotAPIKeyWatchersLocked(userID string) {
	for _, watcher := range p.botAPIKeyWatchersByUser[userID] {
		close(watcher.invalidated)
	}
	delete(p.botAPIKeyWatchersByUser, userID)
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
