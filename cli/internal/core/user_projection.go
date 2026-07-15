package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// UserProjection derives current account/profile/auth lookup state from
// durable evt.user.{userID} events.
type UserProjection struct {
	events.MemoryProjection
	users         map[string]*projectedUser
	loginIndex    map[string]string
	emailIndex    map[string]string
	identityIndex map[string]string
	avatarIndex   map[string]int
	replayGuard   projectionReplayGuard
	dekResolver   *unwrappedDEKResolver
	dekEvents     map[string]map[corev1.UserDEKPurpose]map[int32]*corev1.UserDEKGeneratedEvent
}

type projectedUser struct {
	user               *corev1.User
	login              *projectedUserPII
	loginHash          string
	displayName        *projectedUserPII
	deleted            bool
	shredded           bool
	avatar             *corev1.AssetRecord
	passwordHash       []byte
	passwordSetAt      time.Time
	authGeneration     uint64
	verifiedEmail      map[string]projectedVerifiedEmail
	externalIdentities map[string]ExternalIdentity
	oauthConsent       map[string]struct{}
	preferences        *corev1.ServerUserPreferences
	loginChanged       time.Time
}

// projectedUserPII retains only the encrypted field and the event context
// needed to authenticate it. Plaintext is materialised only for a read.
type projectedUserPII struct {
	eventID   string
	eventType string
	purpose   string
	encrypted *corev1.EncryptedUserString
}

type projectedVerifiedEmail struct {
	pii        *projectedUserPII
	verifiedAt time.Time
}

// NewUserProjection creates the user/account read model. It owns user-facing
// projected state, while unwrapped DEK bytes are delegated to a resolver so
// user PII and message-body reads share one cache and shred path.
func NewUserProjection(keyWrapper kms.KeyWrapper, dekStore dekstore.Reader) *UserProjection {
	return newUserProjectionWithDEKResolver(newUnwrappedDEKResolver(keyWrapper, dekStore))
}

func newUserProjectionWithDEKResolver(dekResolver *unwrappedDEKResolver) *UserProjection {
	return &UserProjection{
		users:         make(map[string]*projectedUser),
		loginIndex:    make(map[string]string),
		emailIndex:    make(map[string]string),
		identityIndex: make(map[string]string),
		avatarIndex:   make(map[string]int),
		replayGuard:   newProjectionReplayGuard(),
		dekResolver:   dekResolver,
		dekEvents:     make(map[string]map[corev1.UserDEKPurpose]map[int32]*corev1.UserDEKGeneratedEvent),
	}
}

func (p *UserProjection) Subjects() []string {
	return []string{events.UserSubjectFilter()}
}

func (p *UserProjection) Apply(event *corev1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	if p.replayGuard.seenOrMark(event, seq) {
		return nil
	}

	switch e := event.GetEvent().(type) {
	case *corev1.Event_UserDekGenerated:
		p.applyDEKGenerated(e.UserDekGenerated)
	case *corev1.Event_UserAccountCreated:
		return p.applyAccountCreated(event.GetId(), e.UserAccountCreated, event.GetCreatedAt())
	case *corev1.Event_UserLoginChanged:
		return p.applyLoginChanged(event.GetId(), e.UserLoginChanged, event.GetCreatedAt())
	case *corev1.Event_UserDisplayNameChanged:
		p.applyDisplayNameChanged(event.GetId(), e.UserDisplayNameChanged)
	case *corev1.Event_UserAvatarSet:
		p.applyAvatarSet(e.UserAvatarSet)
	case *corev1.Event_UserAvatarCleared:
		p.applyAvatarCleared(e.UserAvatarCleared)
	case *corev1.Event_AssetCreated:
		p.applyAssetCreated(e.AssetCreated)
	case *corev1.Event_AssetDeleted:
		p.applyAssetDeleted(e.AssetDeleted)
	case *corev1.Event_UserVerifiedEmailAdded:
		return p.applyVerifiedEmailAdded(event.GetId(), e.UserVerifiedEmailAdded, event.GetCreatedAt())
	case *corev1.Event_UserPasswordHashChanged:
		p.applyPasswordHashChanged(e.UserPasswordHashChanged, event.GetCreatedAt(), seq)
	case *corev1.Event_UserOidcSubjectLinked:
		p.applyOIDCSubjectLinked(e.UserOidcSubjectLinked)
	case *corev1.Event_UserExternalIdentityLinked:
		p.applyExternalIdentityLinked(e.UserExternalIdentityLinked)
	case *corev1.Event_UserExternalIdentityUnlinked:
		p.applyExternalIdentityUnlinked(e.UserExternalIdentityUnlinked, seq)
	case *corev1.Event_UserServerPreferencesChanged:
		p.applyServerPreferencesChanged(e.UserServerPreferencesChanged)
	case *corev1.Event_UserLoginCooldownStarted:
		p.applyLoginCooldownStarted(e.UserLoginCooldownStarted, event.GetCreatedAt())
	case *corev1.Event_UserLoginCooldownCleared:
		p.applyLoginCooldownCleared(e.UserLoginCooldownCleared)
	case *corev1.Event_UserCustomStatusSet:
		p.applyCustomStatusSet(e.UserCustomStatusSet)
	case *corev1.Event_UserCustomStatusCleared:
		p.applyCustomStatusCleared(e.UserCustomStatusCleared)
	case *corev1.Event_UserAccountDeleted:
		p.applyAccountDeleted(e.UserAccountDeleted, seq)
	case *corev1.Event_UserKeyShredded:
		p.applyKeyShredded(e.UserKeyShredded)
	case *corev1.Event_OauthConsentGranted:
		p.applyOAuthConsentGranted(e.OauthConsentGranted)
	}
	return nil
}

func (p *UserProjection) CompleteStartupReplay() {
	p.Lock()
	defer p.Unlock()
	p.replayGuard.completeReplay()
}

func (p *UserProjection) ensureUserLocked(userID string) *projectedUser {
	u := p.users[userID]
	if u == nil {
		u = &projectedUser{verifiedEmail: make(map[string]projectedVerifiedEmail)}
		p.users[userID] = u
	}
	if u.verifiedEmail == nil {
		u.verifiedEmail = make(map[string]projectedVerifiedEmail)
	}
	if u.externalIdentities == nil {
		u.externalIdentities = make(map[string]ExternalIdentity)
	}
	if u.oauthConsent == nil {
		u.oauthConsent = make(map[string]struct{})
	}
	return u
}

func (p *UserProjection) applyDEKGenerated(e *corev1.UserDEKGeneratedEvent) {
	if e == nil || e.GetUserId() == "" || e.GetEpoch() <= 0 || e.GetContentKeyRef() == "" {
		return
	}
	purpose := e.GetPurpose()
	byPurpose := p.dekEvents[e.GetUserId()]
	if byPurpose == nil {
		byPurpose = make(map[corev1.UserDEKPurpose]map[int32]*corev1.UserDEKGeneratedEvent)
		p.dekEvents[e.GetUserId()] = byPurpose
	}
	epochs := byPurpose[purpose]
	if epochs == nil {
		epochs = make(map[int32]*corev1.UserDEKGeneratedEvent)
		byPurpose[purpose] = epochs
	}
	epochs[e.GetEpoch()] = proto.Clone(e).(*corev1.UserDEKGeneratedEvent)
}

func (p *UserProjection) applyAccountCreated(eventID string, e *corev1.UserAccountCreatedEvent, envelopeCreatedAt *timestamppb.Timestamp) error {
	if e == nil || e.GetUserId() == "" || e.GetEncryptedLogin() == nil || e.GetEncryptedDisplayName() == nil {
		return nil
	}
	login, ok, err := p.userPIIStringLocked(context.Background(), eventID, e.GetUserId(), events.EventUserAccountCreated, "login", e.GetEncryptedLogin())
	if err != nil {
		return err
	}
	if !ok || login == "" {
		return nil
	}
	loginHash := userPIILookupHash(login)
	u := p.ensureUserLocked(e.GetUserId())
	u.user = &corev1.User{
		Id:        e.GetUserId(),
		CreatedAt: envelopeCreatedAt,
	}
	u.login = newProjectedUserPII(eventID, events.EventUserAccountCreated, "login", e.GetEncryptedLogin())
	u.loginHash = loginHash
	u.displayName = newProjectedUserPII(eventID, events.EventUserAccountCreated, "display_name", e.GetEncryptedDisplayName())
	u.deleted = false
	u.shredded = false
	p.loginIndex[loginHash] = e.GetUserId()
	return nil
}

func (p *UserProjection) applyLoginChanged(eventID string, e *corev1.UserLoginChangedEvent, envelopeCreatedAt *timestamppb.Timestamp) error {
	if e == nil || e.GetUserId() == "" || e.GetEncryptedLogin() == nil {
		return nil
	}
	login, ok, err := p.userPIIStringLocked(context.Background(), eventID, e.GetUserId(), events.EventUserLoginChanged, "login", e.GetEncryptedLogin())
	if err != nil {
		return err
	}
	if !ok || login == "" {
		return nil
	}
	loginHash := userPIILookupHash(login)
	u := p.ensureUserLocked(e.GetUserId())
	if u.user == nil {
		u.user = &corev1.User{Id: e.GetUserId(), CreatedAt: envelopeCreatedAt}
	}
	if u.loginHash != "" && p.loginIndex[u.loginHash] == e.GetUserId() {
		delete(p.loginIndex, u.loginHash)
	}
	u.login = newProjectedUserPII(eventID, events.EventUserLoginChanged, "login", e.GetEncryptedLogin())
	u.loginHash = loginHash
	p.loginIndex[loginHash] = e.GetUserId()
	return nil
}

func (p *UserProjection) applyDisplayNameChanged(eventID string, e *corev1.UserDisplayNameChangedEvent) {
	if e == nil || e.GetUserId() == "" || e.GetEncryptedDisplayName() == nil {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	if u.user == nil {
		u.user = &corev1.User{Id: e.GetUserId()}
	}
	u.displayName = newProjectedUserPII(eventID, events.EventUserDisplayNameChanged, "display_name", e.GetEncryptedDisplayName())
}

func (p *UserProjection) applyAvatarSet(e *corev1.UserAvatarSetEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	if e.GetAvatar() == nil {
		return
	}
	p.replaceAvatarLocked(u, assetFromDeprecatedAsset(e.GetAvatar(), "avatar.webp", "image/webp"))
}

func (p *UserProjection) applyAssetCreated(e *corev1.AssetCreatedEvent) {
	if e == nil || e.GetAsset() == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	p.replaceAvatarLocked(u, proto.Clone(e.GetAsset()).(*corev1.AssetRecord))
}

func (p *UserProjection) applyAssetDeleted(e *corev1.AssetDeletedEvent) {
	if e == nil || e.GetAssetId() == "" {
		return
	}
	for _, u := range p.users {
		if u != nil && u.avatar != nil && u.avatar.GetId() == e.GetAssetId() {
			p.replaceAvatarLocked(u, nil)
		}
	}
}

func (p *UserProjection) applyAvatarCleared(e *corev1.UserAvatarClearedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	p.replaceAvatarLocked(u, nil)
}

func (p *UserProjection) applyVerifiedEmailAdded(eventID string, e *corev1.UserVerifiedEmailAddedEvent, envelopeCreatedAt *timestamppb.Timestamp) error {
	if e == nil || e.GetUserId() == "" || e.GetEncryptedEmail() == nil {
		return nil
	}
	email, ok, err := p.userPIIStringLocked(context.Background(), eventID, e.GetUserId(), events.EventUserVerifiedEmailAdded, "email", e.GetEncryptedEmail())
	if err != nil {
		return err
	}
	if !ok || email == "" {
		return nil
	}
	hash := emailHash(email)
	u := p.ensureUserLocked(e.GetUserId())
	verifiedAt := time.Now()
	if envelopeCreatedAt != nil {
		verifiedAt = envelopeCreatedAt.AsTime()
	}
	u.verifiedEmail[hash] = projectedVerifiedEmail{
		pii:        newProjectedUserPII(eventID, events.EventUserVerifiedEmailAdded, "email", e.GetEncryptedEmail()),
		verifiedAt: verifiedAt,
	}
	p.emailIndex[hash] = e.GetUserId()
	return nil
}

func (p *UserProjection) applyPasswordHashChanged(e *corev1.UserPasswordHashChangedEvent, envelopeCreatedAt *timestamppb.Timestamp, seq uint64) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.passwordHash = append(u.passwordHash[:0], e.GetPasswordHash()...)
	if !e.GetPreserveExistingCredentials() {
		u.authGeneration = seq
		if envelopeCreatedAt != nil {
			u.passwordSetAt = envelopeCreatedAt.AsTime()
		} else {
			u.passwordSetAt = time.Time{}
		}
	}
}

func (p *UserProjection) applyOIDCSubjectLinked(e *corev1.UserOIDCSubjectLinkedEvent) {
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
	u.externalIdentities[hash] = ExternalIdentity{
		ProviderID:   "oidc",
		ProviderType: "oidc",
		Issuer:       e.GetIssuer(),
		Subject:      e.GetSubject(),
		SubjectHash:  hash,
	}
}

func (p *UserProjection) applyExternalIdentityLinked(e *corev1.UserExternalIdentityLinkedEvent) {
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
	providerID := e.GetProviderId()
	if providerID == "" {
		providerID = e.GetIssuer()
	}
	providerType := e.GetProviderType()
	if providerType == "" {
		providerType = providerID
	}
	u.externalIdentities[hash] = ExternalIdentity{
		ProviderID:   providerID,
		ProviderType: providerType,
		Issuer:       e.GetIssuer(),
		Subject:      e.GetSubject(),
		SubjectHash:  hash,
	}
}

func (p *UserProjection) applyExternalIdentityUnlinked(e *corev1.UserExternalIdentityUnlinkedEvent, seq uint64) {
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

func (p *UserProjection) applyOAuthConsentGranted(e *corev1.OAuthConsentGrantedEvent) {
	if e == nil || e.GetUserId() == "" || e.GetRedirectOrigin() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.oauthConsent[e.GetRedirectOrigin()] = struct{}{}
}

func (p *UserProjection) applyServerPreferencesChanged(e *corev1.UserServerPreferencesChangedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	if e.GetPreferences() == nil {
		u.preferences = nil
		return
	}
	u.preferences = proto.Clone(e.GetPreferences()).(*corev1.ServerUserPreferences)
}

func (p *UserProjection) applyLoginCooldownStarted(e *corev1.UserLoginCooldownStartedEvent, envelopeCreatedAt *timestamppb.Timestamp) {
	if e == nil || e.GetUserId() == "" || envelopeCreatedAt == nil {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.loginChanged = envelopeCreatedAt.AsTime()
}

func (p *UserProjection) applyLoginCooldownCleared(e *corev1.UserLoginCooldownClearedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.loginChanged = time.Time{}
}

func (p *UserProjection) applyCustomStatusSet(e *corev1.UserCustomStatusSetEvent) {
	if e == nil || e.GetUserId() == "" || e.GetStatus() == nil {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	if u.user == nil {
		u.user = &corev1.User{Id: e.GetUserId()}
	}
	u.user.CustomStatus = proto.Clone(e.GetStatus()).(*corev1.CustomUserStatus)
}

func (p *UserProjection) applyCustomStatusCleared(e *corev1.UserCustomStatusClearedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	if u.user != nil {
		u.user.CustomStatus = nil
	}
}

func (p *UserProjection) applyAccountDeleted(e *corev1.UserAccountDeletedEvent, seq uint64) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.deleted = true
	u.authGeneration = seq
	if u.loginHash != "" && p.loginIndex[u.loginHash] == e.GetUserId() {
		delete(p.loginIndex, u.loginHash)
	}
	for hash, userID := range p.emailIndex {
		if userID == e.GetUserId() {
			delete(p.emailIndex, hash)
		}
	}
	for hash, userID := range p.identityIndex {
		if userID == e.GetUserId() {
			delete(p.identityIndex, hash)
		}
	}
	p.replaceAvatarLocked(u, nil)
	u.passwordHash = nil
	u.passwordSetAt = time.Time{}
	u.preferences = nil
	if u.user != nil {
		u.user.CustomStatus = nil
	}
	u.login = nil
	u.loginHash = ""
	u.displayName = nil
	u.verifiedEmail = make(map[string]projectedVerifiedEmail)
	u.externalIdentities = make(map[string]ExternalIdentity)
	u.oauthConsent = make(map[string]struct{})
	u.loginChanged = time.Time{}
	delete(p.dekEvents, e.GetUserId())
}

func (p *UserProjection) applyKeyShredded(e *corev1.UserKeyShreddedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	delete(p.dekEvents, e.GetUserId())
	u := p.ensureUserLocked(e.GetUserId())
	u.shredded = true
	if u.loginHash != "" && p.loginIndex[u.loginHash] == e.GetUserId() {
		delete(p.loginIndex, u.loginHash)
	}
	for hash, userID := range p.emailIndex {
		if userID == e.GetUserId() {
			delete(p.emailIndex, hash)
		}
	}
	for hash, userID := range p.identityIndex {
		if userID == e.GetUserId() {
			delete(p.identityIndex, hash)
		}
	}
	u.user = &corev1.User{Id: e.GetUserId()}
	u.login = nil
	u.loginHash = ""
	u.displayName = nil
	u.passwordHash = nil
	u.passwordSetAt = time.Time{}
	u.preferences = nil
	u.verifiedEmail = make(map[string]projectedVerifiedEmail)
	u.externalIdentities = make(map[string]ExternalIdentity)
	u.oauthConsent = make(map[string]struct{})
	u.loginChanged = time.Time{}
}

func statusExpired(status *corev1.CustomUserStatus, now time.Time) bool {
	if status == nil || status.GetExpiresAt() == nil {
		return false
	}
	return !status.GetExpiresAt().AsTime().After(now)
}

func newProjectedUserPII(eventID, eventType, purpose string, encrypted *corev1.EncryptedUserString) *projectedUserPII {
	if encrypted == nil {
		return nil
	}
	return &projectedUserPII{
		eventID:   eventID,
		eventType: eventType,
		purpose:   purpose,
		encrypted: proto.Clone(encrypted).(*corev1.EncryptedUserString),
	}
}

// userPIIStringLocked decrypts transiently while applying login/email facts so
// the projection can derive lookup digests without retaining plaintext. The
// caller must hold the projection lock.
func (p *UserProjection) userPIIStringLocked(ctx context.Context, eventID, userID, eventType, purpose string, encrypted *corev1.EncryptedUserString) (string, bool, error) {
	if encrypted == nil {
		return "", false, nil
	}
	byPurpose := p.dekEvents[userID]
	if byPurpose == nil {
		return "", false, nil
	}
	event := byPurpose[corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII][encrypted.GetContentKeyEpoch()]
	if event == nil {
		event = byPurpose[corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED][encrypted.GetContentKeyEpoch()]
	}
	if event == nil || p.dekResolver == nil {
		return "", false, nil
	}
	dek, err := p.dekResolver.Resolve(ctx, event, corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII)
	if err != nil {
		if errors.Is(err, encryption.ErrKeyNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("resolve user PII lookup key: %w", err)
	}
	if dek == nil || len(dek.key) == 0 {
		return "", false, nil
	}
	plaintext, err := decryptUserPIIString(dek.key, eventID, userID, eventType, purpose, encrypted)
	if err != nil {
		return "", false, fmt.Errorf("decrypt user PII lookup value: %w", err)
	}
	return plaintext, true, nil
}

type projectedPIISnapshot struct {
	value    *projectedUserPII
	dekEvent *corev1.UserDEKGeneratedEvent
}

type projectedUserSnapshot struct {
	user        *corev1.User
	login       *projectedPIISnapshot
	displayName *projectedPIISnapshot
	deleted     bool
	shredded    bool
}

func (p *UserProjection) piiSnapshotLocked(userID string, value *projectedUserPII) *projectedPIISnapshot {
	if value == nil || value.encrypted == nil {
		return nil
	}
	byPurpose := p.dekEvents[userID]
	if byPurpose == nil {
		return nil
	}
	event := byPurpose[corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII][value.encrypted.GetContentKeyEpoch()]
	if event == nil {
		event = byPurpose[corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED][value.encrypted.GetContentKeyEpoch()]
	}
	if event == nil {
		return nil
	}
	return &projectedPIISnapshot{
		value: &projectedUserPII{
			eventID:   value.eventID,
			eventType: value.eventType,
			purpose:   value.purpose,
			encrypted: proto.Clone(value.encrypted).(*corev1.EncryptedUserString),
		},
		dekEvent: proto.Clone(event).(*corev1.UserDEKGeneratedEvent),
	}
}

func (p *UserProjection) userSnapshotLocked(userID string, u *projectedUser) *projectedUserSnapshot {
	if u == nil {
		return nil
	}
	var user *corev1.User
	if u.user != nil {
		user = proto.Clone(u.user).(*corev1.User)
	}
	return &projectedUserSnapshot{
		user:        user,
		login:       p.piiSnapshotLocked(userID, u.login),
		displayName: p.piiSnapshotLocked(userID, u.displayName),
		deleted:     u.deleted,
		shredded:    u.shredded,
	}
}

func (p *UserProjection) decryptPIISnapshot(ctx context.Context, userID string, snapshot *projectedPIISnapshot) (string, bool, error) {
	if snapshot == nil || snapshot.value == nil || snapshot.dekEvent == nil || p.dekResolver == nil {
		return "", false, nil
	}
	dek, err := p.dekResolver.Resolve(ctx, snapshot.dekEvent, corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII)
	if err != nil {
		return "", false, fmt.Errorf("resolve projected user PII key: %w", err)
	}
	if dek == nil || len(dek.key) == 0 {
		return "", false, nil
	}
	plaintext, err := decryptUserPIIString(
		dek.key,
		snapshot.value.eventID,
		userID,
		snapshot.value.eventType,
		snapshot.value.purpose,
		snapshot.value.encrypted,
	)
	if err != nil {
		return "", false, fmt.Errorf("decrypt projected user PII: %w", err)
	}
	return plaintext, true, nil
}

func (p *UserProjection) hydrateUserSnapshot(ctx context.Context, snapshot *projectedUserSnapshot, now time.Time) (*corev1.User, bool, error) {
	if snapshot == nil || snapshot.deleted || snapshot.shredded || snapshot.user == nil {
		return nil, false, nil
	}
	login, ok, err := p.decryptPIISnapshot(ctx, snapshot.user.GetId(), snapshot.login)
	if err != nil {
		return nil, false, err
	}
	if !ok || login == "" {
		return nil, false, nil
	}
	displayName, ok, err := p.decryptPIISnapshot(ctx, snapshot.user.GetId(), snapshot.displayName)
	if err != nil {
		return nil, false, err
	}
	if !ok || displayName == "" {
		return nil, false, nil
	}
	snapshot.user.Login = login
	snapshot.user.DisplayName = displayName
	if statusExpired(snapshot.user.GetCustomStatus(), now) {
		snapshot.user.CustomStatus = nil
	}
	return snapshot.user, true, nil
}

func (p *UserProjection) GetContext(ctx context.Context, userID string) (*corev1.User, bool, error) {
	p.RLock()
	snapshot := p.userSnapshotLocked(userID, p.users[userID])
	p.RUnlock()
	return p.hydrateUserSnapshot(WithDEKRequestCache(ctx), snapshot, time.Now())
}

func (p *UserProjection) Get(userID string) (*corev1.User, bool) {
	user, ok, _ := p.GetContext(context.Background(), userID)
	return user, ok
}

func (p *UserProjection) GetReferenceContext(ctx context.Context, userID string) (*corev1.User, bool, error) {
	p.RLock()
	snapshot := p.userSnapshotLocked(userID, p.users[userID])
	p.RUnlock()
	if snapshot == nil {
		return nil, false, nil
	}
	user, ok, err := p.hydrateUserSnapshot(WithDEKRequestCache(ctx), snapshot, time.Now())
	if err != nil {
		return nil, false, err
	}
	if ok {
		return user, true, nil
	}
	if snapshot.deleted || snapshot.shredded {
		return DeletedUserReference(userID), true, nil
	}
	return nil, false, nil
}

func (p *UserProjection) GetReference(userID string) (*corev1.User, bool) {
	user, ok, _ := p.GetReferenceContext(context.Background(), userID)
	return user, ok
}

// GetReferences returns public user references aligned with userIDs. Unknown users are nil.
func (p *UserProjection) GetReferencesContext(ctx context.Context, userIDs []string) ([]*corev1.User, error) {
	p.RLock()
	snapshots := make([]*projectedUserSnapshot, len(userIDs))
	for i, userID := range userIDs {
		snapshots[i] = p.userSnapshotLocked(userID, p.users[userID])
	}
	p.RUnlock()

	ctx = WithDEKRequestCache(ctx)
	now := time.Now()
	users := make([]*corev1.User, len(userIDs))
	for i, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		user, ok, err := p.hydrateUserSnapshot(ctx, snapshot, now)
		if err != nil {
			return nil, err
		}
		if ok {
			users[i] = user
		} else if snapshot.deleted || snapshot.shredded {
			users[i] = DeletedUserReference(userIDs[i])
		}
	}
	return users, nil
}

func (p *UserProjection) GetReferences(userIDs []string) []*corev1.User {
	users, _ := p.GetReferencesContext(context.Background(), userIDs)
	return users
}

func (p *UserProjection) GetByLoginContext(ctx context.Context, login string) (*corev1.User, bool, error) {
	lookupHash := userPIILookupHash(login)
	p.RLock()
	userID := p.loginIndex[lookupHash]
	p.RUnlock()
	if userID == "" {
		return nil, false, nil
	}
	user, ok, err := p.GetContext(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	if !ok || userPIILookupHash(user.GetLogin()) != lookupHash {
		return nil, false, nil
	}
	return user, true, nil
}

func (p *UserProjection) GetByLogin(login string) (*corev1.User, bool) {
	user, ok, _ := p.GetByLoginContext(context.Background(), login)
	return user, ok
}

func (p *UserProjection) GetByEmailContext(ctx context.Context, email string) (*corev1.User, bool, error) {
	lookupHash := emailHash(email)
	p.RLock()
	userID := p.emailIndex[lookupHash]
	p.RUnlock()
	if userID == "" {
		return nil, false, nil
	}
	ctx = WithDEKRequestCache(ctx)
	user, ok, err := p.GetContext(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	verifiedEmails, err := p.VerifiedEmailsContext(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	for _, verified := range verifiedEmails {
		if emailHash(verified.Email) == lookupHash {
			return user, true, nil
		}
	}
	return nil, false, nil
}

func (p *UserProjection) GetByEmail(email string) (*corev1.User, bool) {
	user, ok, _ := p.GetByEmailContext(context.Background(), email)
	return user, ok
}

func (p *UserProjection) GetByOIDCSubject(issuer, subject string) (*corev1.User, bool) {
	return p.GetByExternalIdentity(issuer, subject)
}

func (p *UserProjection) GetByExternalIdentityContext(ctx context.Context, issuer, subject string) (*corev1.User, bool, error) {
	p.RLock()
	userID := p.identityIndex[externalIdentityHash(issuer, subject)]
	p.RUnlock()
	if userID == "" {
		return nil, false, nil
	}
	return p.GetContext(ctx, userID)
}

func (p *UserProjection) GetByExternalIdentity(issuer, subject string) (*corev1.User, bool) {
	user, ok, _ := p.GetByExternalIdentityContext(context.Background(), issuer, subject)
	return user, ok
}

func (p *UserProjection) ExternalIdentities(userID string) []ExternalIdentity {
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

func (p *UserProjection) LoginExists(login string) bool {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.loginIndex[userPIILookupHash(login)]
	return ok
}

func (p *UserProjection) EmailClaimed(email string) bool {
	_, ok := p.EmailOwnerID(email)
	return ok
}

// EmailOwnerID returns the projected owner of an email digest without
// decrypting profile data. Mutation invariants use this lookup so KMS
// availability cannot make an existing claim appear free.
func (p *UserProjection) EmailOwnerID(email string) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	userID := p.emailIndex[emailHash(email)]
	return userID, userID != ""
}

// ExternalIdentityOwnerID returns the projected owner of an identity digest
// without hydrating the user's encrypted profile.
func (p *UserProjection) ExternalIdentityOwnerID(issuer, subject string) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	userID := p.identityIndex[externalIdentityHash(issuer, subject)]
	return userID, userID != ""
}

func (p *UserProjection) PasswordHash(userID string) ([]byte, bool) {
	hash, _, ok := p.PasswordHashWithSetAt(userID)
	return hash, ok
}

func (p *UserProjection) PasswordHashWithSetAt(userID string) ([]byte, time.Time, bool) {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || len(u.passwordHash) == 0 {
		return nil, time.Time{}, false
	}
	return append([]byte(nil), u.passwordHash...), u.passwordSetAt, true
}

func (p *UserProjection) AuthGeneration(userID string) (uint64, bool) {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted {
		return 0, false
	}
	return u.authGeneration, true
}

func (p *UserProjection) Avatar(userID string) (*corev1.AssetRecord, bool) {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || u.avatar == nil {
		return nil, false
	}
	return proto.Clone(u.avatar).(*corev1.AssetRecord), true
}

// IsPublicAvatarAsset reports whether assetID or key is the current avatar of
// a non-deleted user. User avatars are intentionally public server assets. The
// replay-built index keeps this unauthenticated request-path check O(1).
func (p *UserProjection) IsPublicAvatarAsset(assetID string) bool {
	p.RLock()
	defer p.RUnlock()
	return assetID != "" && p.avatarIndex[assetID] > 0
}

func (p *UserProjection) replaceAvatarLocked(u *projectedUser, avatar *corev1.AssetRecord) {
	if u == nil {
		return
	}
	if p.avatarIndex == nil {
		p.avatarIndex = make(map[string]int)
	}
	for key := range assetRecordKeys(u.avatar) {
		if p.avatarIndex[key] <= 1 {
			delete(p.avatarIndex, key)
		} else {
			p.avatarIndex[key]--
		}
	}
	u.avatar = avatar
	if u.deleted {
		return
	}
	for key := range assetRecordKeys(avatar) {
		p.avatarIndex[key]++
	}
}

func assetRecordKeys(asset *corev1.AssetRecord) map[string]struct{} {
	keys := make(map[string]struct{}, 3)
	if asset == nil {
		return keys
	}
	for _, key := range []string{asset.GetId(), asset.GetNats().GetKey(), asset.GetS3().GetKey()} {
		if key != "" {
			keys[key] = struct{}{}
		}
	}
	return keys
}

func (p *UserProjection) Preferences(userID string) (*corev1.ServerUserPreferences, bool) {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || u.preferences == nil {
		return nil, false
	}
	return proto.Clone(u.preferences).(*corev1.ServerUserPreferences), true
}

func (p *UserProjection) VerifiedEmailsContext(ctx context.Context, userID string) ([]VerifiedEmail, error) {
	p.RLock()
	u := p.users[userID]
	if u == nil || u.deleted || len(u.verifiedEmail) == 0 {
		p.RUnlock()
		return nil, nil
	}
	type emailSnapshot struct {
		pii        *projectedPIISnapshot
		verifiedAt time.Time
	}
	snapshots := make([]emailSnapshot, 0, len(u.verifiedEmail))
	for _, email := range u.verifiedEmail {
		snapshots = append(snapshots, emailSnapshot{
			pii:        p.piiSnapshotLocked(userID, email.pii),
			verifiedAt: email.verifiedAt,
		})
	}
	p.RUnlock()

	ctx = WithDEKRequestCache(ctx)
	out := make([]VerifiedEmail, 0, len(snapshots))
	for _, snapshot := range snapshots {
		email, ok, err := p.decryptPIISnapshot(ctx, userID, snapshot.pii)
		if err != nil {
			return nil, err
		}
		if !ok || email == "" {
			continue
		}
		out = append(out, VerifiedEmail{Email: email, VerifiedAt: snapshot.verifiedAt})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].VerifiedAt.Equal(out[j].VerifiedAt) {
			return out[i].VerifiedAt.Before(out[j].VerifiedAt)
		}
		return strings.ToLower(out[i].Email) < strings.ToLower(out[j].Email)
	})
	return out, nil
}

func (p *UserProjection) VerifiedEmails(userID string) []VerifiedEmail {
	emails, _ := p.VerifiedEmailsContext(context.Background(), userID)
	return emails
}

func (p *UserProjection) HasVerifiedEmail(userID string) bool {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	return u != nil && !u.deleted && len(u.verifiedEmail) > 0
}

func (p *UserProjection) HasVerifiedFactor(userID string) bool {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted {
		return false
	}
	return len(u.verifiedEmail) > 0 || len(u.externalIdentities) > 0
}

func (p *UserProjection) HasOAuthConsent(userID, redirectOrigin string) bool {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || redirectOrigin == "" {
		return false
	}
	_, ok := u.oauthConsent[redirectOrigin]
	return ok
}

func (p *UserProjection) LoginChangedAt(userID string) time.Time {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted {
		return time.Time{}
	}
	return u.loginChanged
}

func (p *UserProjection) UsersContext(ctx context.Context) ([]*corev1.User, error) {
	p.RLock()
	snapshots := make([]*projectedUserSnapshot, 0, len(p.users))
	for userID, u := range p.users {
		if u == nil || u.deleted || u.user == nil {
			continue
		}
		snapshots = append(snapshots, p.userSnapshotLocked(userID, u))
	}
	p.RUnlock()

	ctx = WithDEKRequestCache(ctx)
	out := make([]*corev1.User, 0, len(snapshots))
	now := time.Now()
	for _, snapshot := range snapshots {
		user, ok, err := p.hydrateUserSnapshot(ctx, snapshot, now)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, user)
		}
	}
	return out, nil
}

func (p *UserProjection) Users() []*corev1.User {
	users, _ := p.UsersContext(context.Background())
	return users
}

func (p *UserProjection) VerifiedUserIDs() []string {
	p.RLock()
	defer p.RUnlock()
	seen := map[string]struct{}{}
	for _, userID := range p.emailIndex {
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

func (p *UserProjection) VerifiedAccountIDs() []string {
	p.RLock()
	defer p.RUnlock()
	seen := map[string]struct{}{}
	for _, userID := range p.emailIndex {
		if u := p.users[userID]; u != nil && !u.deleted {
			seen[userID] = struct{}{}
		}
	}
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

func (p *UserProjection) Count() int {
	p.RLock()
	defer p.RUnlock()
	var count int
	for _, u := range p.users {
		if u != nil && !u.deleted && u.user != nil {
			count++
		}
	}
	return count
}

func (p *UserProjection) Stats() (users int, verifiedEmails int, oidcSubjects int) {
	p.RLock()
	defer p.RUnlock()
	for _, u := range p.users {
		if u == nil || u.deleted || u.user == nil {
			continue
		}
		users++
		verifiedEmails += len(u.verifiedEmail)
	}
	oidcSubjects = len(p.identityIndex)
	return users, verifiedEmails, oidcSubjects
}
