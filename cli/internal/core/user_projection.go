package core

import (
	"context"
	"errors"
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
	eventIDSeen   eventIDSet
	keyWrapper    kms.KeyWrapper
	dekStore      dekstore.Reader
	contentKeys   map[string]map[corev1.UserDEKPurpose]map[int32][]byte
}

type projectedUser struct {
	user               *corev1.User
	deleted            bool
	avatar             *corev1.AssetRecord
	passwordHash       []byte
	passwordSetAt      time.Time
	authGeneration     uint64
	verifiedEmail      map[string]VerifiedEmail
	externalIdentities map[string]ExternalIdentity
	oauthConsent       map[string]struct{}
	preferences        *corev1.ServerUserPreferences
	loginChanged       time.Time
}

func NewUserProjection(keyWrapper kms.KeyWrapper, dekStore dekstore.Reader) *UserProjection {
	return &UserProjection{
		users:         make(map[string]*projectedUser),
		loginIndex:    make(map[string]string),
		emailIndex:    make(map[string]string),
		identityIndex: make(map[string]string),
		eventIDSeen:   newEventIDSet(),
		keyWrapper:    keyWrapper,
		dekStore:      dekStore,
		contentKeys:   make(map[string]map[corev1.UserDEKPurpose]map[int32][]byte),
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
	if p.eventIDSeen.seenOrMark(event) {
		return nil
	}

	switch e := event.GetEvent().(type) {
	case *corev1.Event_UserDekGenerated:
		p.applyDEKGenerated(e.UserDekGenerated)
	case *corev1.Event_UserAccountCreated:
		p.applyAccountCreated(event.GetId(), e.UserAccountCreated, event.GetCreatedAt())
	case *corev1.Event_UserLoginChanged:
		p.applyLoginChanged(event.GetId(), e.UserLoginChanged, event.GetCreatedAt())
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
		p.applyVerifiedEmailAdded(event.GetId(), e.UserVerifiedEmailAdded, event.GetCreatedAt())
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

func (p *UserProjection) ensureUserLocked(userID string) *projectedUser {
	u := p.users[userID]
	if u == nil {
		u = &projectedUser{verifiedEmail: make(map[string]VerifiedEmail)}
		p.users[userID] = u
	}
	if u.verifiedEmail == nil {
		u.verifiedEmail = make(map[string]VerifiedEmail)
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
	if e == nil || e.GetUserId() == "" || e.GetEpoch() <= 0 || e.GetContentKeyRef() == "" || p.keyWrapper == nil || p.dekStore == nil {
		return
	}
	purpose := e.GetPurpose()
	stored, err := p.dekStore.Get(context.Background(), e.GetContentKeyRef())
	if err != nil {
		return
	}
	keyRef := stored.WrappingKeyRef
	if keyRef == "" {
		keyRef = kms.LegacyUserKeyRef(e.GetUserId())
	}
	key, err := p.keyWrapper.UnwrapContentKey(context.Background(), keyRef, kms.WrappedContentKey{
		EncryptedContentKey: stored.EncryptedContentKey,
		Nonce:               stored.ContentKeyNonce,
		Algorithm:           stored.WrappingAlgorithm,
		Metadata:            stored.WrappingMetadata,
	}, userDEKAAD(e.GetUserId(), purpose, e.GetEpoch()))
	if err != nil {
		return
	}
	byPurpose := p.contentKeys[e.GetUserId()]
	if byPurpose == nil {
		byPurpose = make(map[corev1.UserDEKPurpose]map[int32][]byte)
		p.contentKeys[e.GetUserId()] = byPurpose
	}
	epochs := byPurpose[purpose]
	if epochs == nil {
		epochs = make(map[int32][]byte)
		byPurpose[purpose] = epochs
	}
	epochs[e.GetEpoch()] = append([]byte(nil), key...)
}

func (p *UserProjection) applyAccountCreated(eventID string, e *corev1.UserAccountCreatedEvent, envelopeCreatedAt *timestamppb.Timestamp) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	login, ok := p.userPIIString(eventID, e.GetUserId(), events.EventUserAccountCreated, "login", e.GetEncryptedLogin())
	if !ok {
		return
	}
	displayName, ok := p.userPIIString(eventID, e.GetUserId(), events.EventUserAccountCreated, "display_name", e.GetEncryptedDisplayName())
	if !ok {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.user = &corev1.User{
		Id:          e.GetUserId(),
		Login:       login,
		DisplayName: displayName,
		CreatedAt:   envelopeCreatedAt,
	}
	u.deleted = false
	if login != "" {
		p.loginIndex[strings.ToLower(login)] = e.GetUserId()
	}
}

func (p *UserProjection) applyLoginChanged(eventID string, e *corev1.UserLoginChangedEvent, envelopeCreatedAt *timestamppb.Timestamp) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	login, ok := p.userPIIString(eventID, e.GetUserId(), events.EventUserLoginChanged, "login", e.GetEncryptedLogin())
	if !ok || login == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	if u.user == nil {
		u.user = &corev1.User{Id: e.GetUserId(), CreatedAt: envelopeCreatedAt}
	}
	if old := u.user.GetLogin(); old != "" {
		delete(p.loginIndex, strings.ToLower(old))
	}
	u.user.Login = login
	p.loginIndex[strings.ToLower(login)] = e.GetUserId()
}

func (p *UserProjection) applyDisplayNameChanged(eventID string, e *corev1.UserDisplayNameChangedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	displayName, ok := p.userPIIString(eventID, e.GetUserId(), events.EventUserDisplayNameChanged, "display_name", e.GetEncryptedDisplayName())
	if !ok {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	if u.user == nil {
		u.user = &corev1.User{Id: e.GetUserId()}
	}
	u.user.DisplayName = displayName
}

func (p *UserProjection) applyAvatarSet(e *corev1.UserAvatarSetEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	if e.GetAvatar() == nil {
		return
	}
	u.avatar = assetFromDeprecatedAsset(e.GetAvatar(), "avatar.webp", "image/webp")
}

func (p *UserProjection) applyAssetCreated(e *corev1.AssetCreatedEvent) {
	if e == nil || e.GetAsset() == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.avatar = proto.Clone(e.GetAsset()).(*corev1.AssetRecord)
}

func (p *UserProjection) applyAssetDeleted(e *corev1.AssetDeletedEvent) {
	if e == nil || e.GetAssetId() == "" {
		return
	}
	for _, u := range p.users {
		if u != nil && u.avatar != nil && u.avatar.GetId() == e.GetAssetId() {
			u.avatar = nil
		}
	}
}

func (p *UserProjection) applyAvatarCleared(e *corev1.UserAvatarClearedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.avatar = nil
}

func (p *UserProjection) applyVerifiedEmailAdded(eventID string, e *corev1.UserVerifiedEmailAddedEvent, envelopeCreatedAt *timestamppb.Timestamp) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	email, ok := p.userPIIString(eventID, e.GetUserId(), events.EventUserVerifiedEmailAdded, "email", e.GetEncryptedEmail())
	if !ok || email == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	verifiedAt := time.Now()
	if envelopeCreatedAt != nil {
		verifiedAt = envelopeCreatedAt.AsTime()
	}
	hash := emailHash(email)
	u.verifiedEmail[hash] = VerifiedEmail{Email: email, VerifiedAt: verifiedAt}
	p.emailIndex[hash] = e.GetUserId()
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
	if u.user != nil && u.user.GetLogin() != "" {
		delete(p.loginIndex, strings.ToLower(u.user.GetLogin()))
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
	u.avatar = nil
	u.passwordHash = nil
	u.passwordSetAt = time.Time{}
	u.preferences = nil
	if u.user != nil {
		u.user.CustomStatus = nil
	}
	u.verifiedEmail = make(map[string]VerifiedEmail)
	u.externalIdentities = make(map[string]ExternalIdentity)
	u.oauthConsent = make(map[string]struct{})
	u.loginChanged = time.Time{}
	delete(p.contentKeys, e.GetUserId())
}

func (p *UserProjection) applyKeyShredded(e *corev1.UserKeyShreddedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	delete(p.contentKeys, e.GetUserId())
	u := p.ensureUserLocked(e.GetUserId())
	if u.user != nil && u.user.GetLogin() != "" {
		delete(p.loginIndex, strings.ToLower(u.user.GetLogin()))
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
	u.passwordHash = nil
	u.passwordSetAt = time.Time{}
	u.preferences = nil
	u.verifiedEmail = make(map[string]VerifiedEmail)
	u.externalIdentities = make(map[string]ExternalIdentity)
	u.oauthConsent = make(map[string]struct{})
	u.loginChanged = time.Time{}
}

func cloneUserWithActiveStatus(user *corev1.User, now time.Time) *corev1.User {
	if user == nil {
		return nil
	}
	out := proto.Clone(user).(*corev1.User)
	if statusExpired(out.GetCustomStatus(), now) {
		out.CustomStatus = nil
	}
	return out
}

func statusExpired(status *corev1.CustomUserStatus, now time.Time) bool {
	if status == nil || status.GetExpiresAt() == nil {
		return false
	}
	return !status.GetExpiresAt().AsTime().After(now)
}

func (p *UserProjection) userPIIString(eventID, userID, eventType, purpose string, encrypted *corev1.EncryptedUserString) (string, bool) {
	if encrypted == nil {
		return "", false
	}
	byPurpose := p.contentKeys[userID]
	if byPurpose == nil {
		return "", false
	}
	key := byPurpose[corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII][encrypted.GetContentKeyEpoch()]
	if len(key) == 0 {
		key = byPurpose[corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED][encrypted.GetContentKeyEpoch()]
	}
	if len(key) == 0 {
		return "", false
	}
	plaintext, err := decryptUserPIIString(key, eventID, userID, eventType, purpose, encrypted)
	if err != nil {
		if errors.Is(err, encryption.ErrDecryptionFailed) || errors.Is(err, encryption.ErrKeyNotFound) {
			return "", false
		}
		return "", false
	}
	return plaintext, true
}

func (p *UserProjection) Get(userID string) (*corev1.User, bool) {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || u.user == nil {
		return nil, false
	}
	return cloneUserWithActiveStatus(u.user, time.Now()), true
}

func (p *UserProjection) GetReference(userID string) (*corev1.User, bool) {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil {
		return nil, false
	}
	if u.deleted || u.user == nil || u.user.GetLogin() == "" || u.user.GetDisplayName() == "" {
		return DeletedUserReference(userID), true
	}
	return cloneUserWithActiveStatus(u.user, time.Now()), true
}

func (p *UserProjection) GetByLogin(login string) (*corev1.User, bool) {
	p.RLock()
	userID := p.loginIndex[strings.ToLower(strings.TrimSpace(login))]
	p.RUnlock()
	if userID == "" {
		return nil, false
	}
	return p.Get(userID)
}

func (p *UserProjection) GetByEmail(email string) (*corev1.User, bool) {
	p.RLock()
	userID := p.emailIndex[emailHash(email)]
	p.RUnlock()
	if userID == "" {
		return nil, false
	}
	return p.Get(userID)
}

func (p *UserProjection) GetByOIDCSubject(issuer, subject string) (*corev1.User, bool) {
	return p.GetByExternalIdentity(issuer, subject)
}

func (p *UserProjection) GetByExternalIdentity(issuer, subject string) (*corev1.User, bool) {
	p.RLock()
	userID := p.identityIndex[externalIdentityHash(issuer, subject)]
	p.RUnlock()
	if userID == "" {
		return nil, false
	}
	return p.Get(userID)
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
	_, ok := p.loginIndex[strings.ToLower(strings.TrimSpace(login))]
	return ok
}

func (p *UserProjection) EmailClaimed(email string) bool {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.emailIndex[emailHash(email)]
	return ok
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

func (p *UserProjection) Preferences(userID string) (*corev1.ServerUserPreferences, bool) {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || u.preferences == nil {
		return nil, false
	}
	return proto.Clone(u.preferences).(*corev1.ServerUserPreferences), true
}

func (p *UserProjection) VerifiedEmails(userID string) []VerifiedEmail {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || len(u.verifiedEmail) == 0 {
		return nil
	}
	out := make([]VerifiedEmail, 0, len(u.verifiedEmail))
	for _, email := range u.verifiedEmail {
		out = append(out, email)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].VerifiedAt.Equal(out[j].VerifiedAt) {
			return out[i].VerifiedAt.Before(out[j].VerifiedAt)
		}
		return strings.ToLower(out[i].Email) < strings.ToLower(out[j].Email)
	})
	return out
}

func (p *UserProjection) HasVerifiedEmail(userID string) bool {
	return len(p.VerifiedEmails(userID)) > 0
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

func (p *UserProjection) Users() []*corev1.User {
	p.RLock()
	defer p.RUnlock()
	out := make([]*corev1.User, 0, len(p.users))
	for _, u := range p.users {
		if u == nil || u.deleted || u.user == nil {
			continue
		}
		out = append(out, cloneUserWithActiveStatus(u.user, time.Now()))
	}
	return out
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
