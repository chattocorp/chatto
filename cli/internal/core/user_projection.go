package core

import (
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// UserProjection derives current account/profile/auth lookup state from
// durable evt.user.{userID} events.
type UserProjection struct {
	events.MemoryProjection
	users       map[string]*projectedUser
	loginIndex  map[string]string
	emailIndex  map[string]string
	oidcIndex   map[string]string
	eventIDSeen map[string]struct{}
}

type projectedUser struct {
	user          *corev1.User
	deleted       bool
	avatar        *corev1.Asset
	passwordHash  []byte
	verifiedEmail map[string]VerifiedEmail
	preferences   *corev1.ServerUserPreferences
	loginChanged  time.Time
}

func NewUserProjection() *UserProjection {
	return &UserProjection{
		users:       make(map[string]*projectedUser),
		loginIndex:  make(map[string]string),
		emailIndex:  make(map[string]string),
		oidcIndex:   make(map[string]string),
		eventIDSeen: make(map[string]struct{}),
	}
}

func (p *UserProjection) Subjects() []string {
	return []string{events.UserSubjectFilter()}
}

func (p *UserProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	if id := event.GetId(); id != "" {
		p.Lock()
		if _, ok := p.eventIDSeen[id]; ok {
			p.Unlock()
			return nil
		}
		p.eventIDSeen[id] = struct{}{}
		p.Unlock()
	}

	p.Lock()
	defer p.Unlock()

	switch e := event.GetEvent().(type) {
	case *corev1.Event_UserAccountCreated:
		p.applyAccountCreated(e.UserAccountCreated, event.GetCreatedAt())
	case *corev1.Event_UserLoginChanged:
		p.applyLoginChanged(e.UserLoginChanged, event.GetCreatedAt())
	case *corev1.Event_UserDisplayNameChanged:
		p.applyDisplayNameChanged(e.UserDisplayNameChanged)
	case *corev1.Event_UserAvatarSet:
		p.applyAvatarSet(e.UserAvatarSet)
	case *corev1.Event_UserAvatarCleared:
		p.applyAvatarCleared(e.UserAvatarCleared)
	case *corev1.Event_UserVerifiedEmailAdded:
		p.applyVerifiedEmailAdded(e.UserVerifiedEmailAdded, event.GetCreatedAt())
	case *corev1.Event_UserPasswordHashChanged:
		p.applyPasswordHashChanged(e.UserPasswordHashChanged)
	case *corev1.Event_UserOidcSubjectLinked:
		p.applyOIDCSubjectLinked(e.UserOidcSubjectLinked)
	case *corev1.Event_UserServerPreferencesChanged:
		p.applyServerPreferencesChanged(e.UserServerPreferencesChanged)
	case *corev1.Event_UserLoginCooldownStarted:
		p.applyLoginCooldownStarted(e.UserLoginCooldownStarted, event.GetCreatedAt())
	case *corev1.Event_UserLoginCooldownCleared:
		p.applyLoginCooldownCleared(e.UserLoginCooldownCleared)
	case *corev1.Event_UserAccountDeleted:
		p.applyAccountDeleted(e.UserAccountDeleted)
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
	return u
}

func (p *UserProjection) applyAccountCreated(e *corev1.UserAccountCreatedEvent, envelopeCreatedAt *timestamppb.Timestamp) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.user = &corev1.User{
		Id:          e.GetUserId(),
		Login:       e.GetLogin(),
		DisplayName: e.GetDisplayName(),
		CreatedAt:   envelopeCreatedAt,
	}
	u.deleted = false
	if e.GetLogin() != "" {
		p.loginIndex[strings.ToLower(e.GetLogin())] = e.GetUserId()
	}
}

func (p *UserProjection) applyLoginChanged(e *corev1.UserLoginChangedEvent, envelopeCreatedAt *timestamppb.Timestamp) {
	if e == nil || e.GetUserId() == "" || e.GetLogin() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	if u.user == nil {
		u.user = &corev1.User{Id: e.GetUserId(), CreatedAt: envelopeCreatedAt}
	}
	if old := u.user.GetLogin(); old != "" {
		delete(p.loginIndex, strings.ToLower(old))
	}
	u.user.Login = e.GetLogin()
	p.loginIndex[strings.ToLower(e.GetLogin())] = e.GetUserId()
}

func (p *UserProjection) applyDisplayNameChanged(e *corev1.UserDisplayNameChangedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	if u.user == nil {
		u.user = &corev1.User{Id: e.GetUserId()}
	}
	u.user.DisplayName = e.GetDisplayName()
}

func (p *UserProjection) applyAvatarSet(e *corev1.UserAvatarSetEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	if e.GetAvatar() == nil {
		return
	}
	u.avatar = proto.Clone(e.GetAvatar()).(*corev1.Asset)
}

func (p *UserProjection) applyAvatarCleared(e *corev1.UserAvatarClearedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.avatar = nil
}

func (p *UserProjection) applyVerifiedEmailAdded(e *corev1.UserVerifiedEmailAddedEvent, envelopeCreatedAt *timestamppb.Timestamp) {
	if e == nil || e.GetUserId() == "" || e.GetEmail() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	verifiedAt := time.Now()
	if envelopeCreatedAt != nil {
		verifiedAt = envelopeCreatedAt.AsTime()
	}
	hash := emailHash(e.GetEmail())
	u.verifiedEmail[hash] = VerifiedEmail{Email: e.GetEmail(), VerifiedAt: verifiedAt}
	p.emailIndex[hash] = e.GetUserId()
}

func (p *UserProjection) applyPasswordHashChanged(e *corev1.UserPasswordHashChangedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.passwordHash = append(u.passwordHash[:0], e.GetPasswordHash()...)
}

func (p *UserProjection) applyOIDCSubjectLinked(e *corev1.UserOIDCSubjectLinkedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	hash := e.GetSubjectHash()
	if hash == "" && e.GetIssuer() != "" && e.GetSubject() != "" {
		hash = oidcSubjectHash(e.GetIssuer(), e.GetSubject())
	}
	if hash == "" {
		return
	}
	p.oidcIndex[hash] = e.GetUserId()
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

func (p *UserProjection) applyAccountDeleted(e *corev1.UserAccountDeletedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	u.deleted = true
	if u.user != nil && u.user.GetLogin() != "" {
		delete(p.loginIndex, strings.ToLower(u.user.GetLogin()))
	}
	for hash, userID := range p.emailIndex {
		if userID == e.GetUserId() {
			delete(p.emailIndex, hash)
		}
	}
	for hash, userID := range p.oidcIndex {
		if userID == e.GetUserId() {
			delete(p.oidcIndex, hash)
		}
	}
	u.avatar = nil
	u.passwordHash = nil
	u.preferences = nil
	u.verifiedEmail = make(map[string]VerifiedEmail)
	u.loginChanged = time.Time{}
}

func (p *UserProjection) Get(userID string) (*corev1.User, bool) {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || u.user == nil {
		return nil, false
	}
	return proto.Clone(u.user).(*corev1.User), true
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
	p.RLock()
	userID := p.oidcIndex[oidcSubjectHash(issuer, subject)]
	p.RUnlock()
	if userID == "" {
		return nil, false
	}
	return p.Get(userID)
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
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || len(u.passwordHash) == 0 {
		return nil, false
	}
	return append([]byte(nil), u.passwordHash...), true
}

func (p *UserProjection) Avatar(userID string) (*corev1.Asset, bool) {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.deleted || u.avatar == nil {
		return nil, false
	}
	return proto.Clone(u.avatar).(*corev1.Asset), true
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
		out = append(out, proto.Clone(u.user).(*corev1.User))
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
	oidcSubjects = len(p.oidcIndex)
	return users, verifiedEmails, oidcSubjects
}
