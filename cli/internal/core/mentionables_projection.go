package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type mentionableOwnerKind string

const (
	mentionableOwnerVirtual mentionableOwnerKind = "virtual"
	mentionableOwnerUser    mentionableOwnerKind = "user"
	mentionableOwnerRole    mentionableOwnerKind = "role"
)

type mentionableOwner struct {
	kind mentionableOwnerKind
	id   string
}

// MentionablesProjection derives the global @handle namespace from durable
// user and RBAC facts. It consumes the whole EVT stream so callers can use the
// stream-wide OCC boundary for user-vs-role uniqueness without adding a
// separate claim record.
type MentionablesProjection struct {
	events.MemoryProjection
	owners      map[string]map[mentionableOwner]struct{}
	userLogins  map[string]string
	keyWrapper  kms.KeyWrapper
	dekStore    dekstore.Reader
	contentKeys map[string]map[corev1.UserDEKPurpose]map[int32][]byte
}

func NewMentionablesProjection(keyWrapper kms.KeyWrapper, dekStore dekstore.Reader) *MentionablesProjection {
	p := &MentionablesProjection{
		owners:      make(map[string]map[mentionableOwner]struct{}),
		userLogins:  make(map[string]string),
		keyWrapper:  keyWrapper,
		dekStore:    dekStore,
		contentKeys: make(map[string]map[corev1.UserDEKPurpose]map[int32][]byte),
	}
	p.addOwner(MentionHandleAll, mentionableOwner{kind: mentionableOwnerVirtual, id: MentionHandleAll})
	p.addOwner(MentionHandleHere, mentionableOwner{kind: mentionableOwnerVirtual, id: MentionHandleHere})
	return p
}

func (p *MentionablesProjection) Subjects() []string {
	return []string{events.EventSubjectFilter()}
}

func (p *MentionablesProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()

	switch e := event.GetEvent().(type) {
	case *corev1.Event_UserDekGenerated:
		p.applyDEKGenerated(e.UserDekGenerated)
	case *corev1.Event_UserAccountCreated:
		p.applyUserAccountCreated(event.GetId(), e.UserAccountCreated)
	case *corev1.Event_UserLoginChanged:
		p.applyUserLoginChanged(event.GetId(), e.UserLoginChanged)
	case *corev1.Event_UserAccountDeleted:
		p.applyUserAccountDeleted(e.UserAccountDeleted)
	case *corev1.Event_UserKeyShredded:
		p.applyUserKeyShredded(e.UserKeyShredded)
	case *corev1.Event_RbacRoleCreated:
		p.addOwner(e.RbacRoleCreated.GetRoleName(), mentionableOwner{kind: mentionableOwnerRole, id: strings.ToLower(e.RbacRoleCreated.GetRoleName())})
	case *corev1.Event_RbacRoleDeleted:
		roleName := strings.ToLower(e.RbacRoleDeleted.GetRoleName())
		p.removeOwner(roleName, mentionableOwner{kind: mentionableOwnerRole, id: roleName})
	}
	return nil
}

func (p *MentionablesProjection) applyDEKGenerated(e *corev1.UserDEKGeneratedEvent) {
	if e == nil || e.GetUserId() == "" || e.GetEpoch() <= 0 || e.GetContentKeyRef() == "" || p.keyWrapper == nil || p.dekStore == nil {
		return
	}
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
	}, userDEKAAD(e.GetUserId(), e.GetPurpose(), e.GetEpoch()))
	if err != nil {
		return
	}
	byPurpose := p.contentKeys[e.GetUserId()]
	if byPurpose == nil {
		byPurpose = make(map[corev1.UserDEKPurpose]map[int32][]byte)
		p.contentKeys[e.GetUserId()] = byPurpose
	}
	epochs := byPurpose[e.GetPurpose()]
	if epochs == nil {
		epochs = make(map[int32][]byte)
		byPurpose[e.GetPurpose()] = epochs
	}
	epochs[e.GetEpoch()] = append([]byte(nil), key...)
}

func (p *MentionablesProjection) applyUserAccountCreated(eventID string, e *corev1.UserAccountCreatedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	login, ok := p.userPIIString(eventID, e.GetUserId(), events.EventUserAccountCreated, "login", e.GetEncryptedLogin())
	if !ok || login == "" {
		return
	}
	p.setUserLogin(e.GetUserId(), login)
}

func (p *MentionablesProjection) applyUserLoginChanged(eventID string, e *corev1.UserLoginChangedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	login, ok := p.userPIIString(eventID, e.GetUserId(), events.EventUserLoginChanged, "login", e.GetEncryptedLogin())
	if !ok || login == "" {
		return
	}
	p.setUserLogin(e.GetUserId(), login)
}

func (p *MentionablesProjection) applyUserAccountDeleted(e *corev1.UserAccountDeletedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	p.removeUserLogin(e.GetUserId())
}

func (p *MentionablesProjection) applyUserKeyShredded(e *corev1.UserKeyShreddedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	delete(p.contentKeys, e.GetUserId())
}

func (p *MentionablesProjection) setUserLogin(userID, login string) {
	p.removeUserLogin(userID)
	normalized := normalizeMentionableHandle(login)
	if normalized == "" {
		return
	}
	p.userLogins[userID] = normalized
	p.addOwner(normalized, mentionableOwner{kind: mentionableOwnerUser, id: userID})
}

func (p *MentionablesProjection) removeUserLogin(userID string) {
	old := p.userLogins[userID]
	if old == "" {
		return
	}
	delete(p.userLogins, userID)
	p.removeOwner(old, mentionableOwner{kind: mentionableOwnerUser, id: userID})
}

func (p *MentionablesProjection) addOwner(handle string, owner mentionableOwner) {
	normalized := normalizeMentionableHandle(handle)
	if normalized == "" || owner.kind == "" || owner.id == "" {
		return
	}
	owners := p.owners[normalized]
	if owners == nil {
		owners = make(map[mentionableOwner]struct{})
		p.owners[normalized] = owners
	}
	owners[owner] = struct{}{}
}

func (p *MentionablesProjection) removeOwner(handle string, owner mentionableOwner) {
	normalized := normalizeMentionableHandle(handle)
	owners := p.owners[normalized]
	if owners == nil {
		return
	}
	delete(owners, owner)
	if len(owners) == 0 {
		delete(p.owners, normalized)
	}
}

func (p *MentionablesProjection) userPIIString(eventID, userID, eventType, purpose string, encrypted *corev1.EncryptedUserString) (string, bool) {
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

func (p *MentionablesProjection) Availability(handle string, allowedOwner *mentionableOwner) MentionableAvailability {
	normalized := normalizeMentionableHandle(handle)
	if normalized == "" {
		return MentionableAvailability{Available: false}
	}
	p.RLock()
	defer p.RUnlock()
	owners := p.owners[normalized]
	if len(owners) == 0 {
		return MentionableAvailability{Available: true}
	}
	if allowedOwner != nil && len(owners) == 1 {
		if _, ok := owners[*allowedOwner]; ok {
			return MentionableAvailability{Available: true}
		}
	}
	for owner := range owners {
		return MentionableAvailability{Available: false, OwnerKind: owner.kind, OwnerID: owner.id}
	}
	return MentionableAvailability{Available: false}
}

func (p *MentionablesProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var owners int64
	var bytes int64
	for handle, byOwner := range p.owners {
		bytes += projectionMapEntryOverhead + int64(len(handle))
		for owner := range byOwner {
			owners++
			bytes += projectionMapEntryOverhead + int64(len(owner.kind)+len(owner.id))
		}
	}
	for userID, handle := range p.userLogins {
		bytes += projectionMapEntryOverhead + int64(len(userID)+len(handle))
	}
	return int64(len(p.owners)), bytes, []ProjectionAdminMetric{
		{Name: "handles", Value: int64(len(p.owners)), Bytes: 0},
		{Name: "owners", Value: owners, Bytes: 0},
		{Name: "user_logins", Value: int64(len(p.userLogins)), Bytes: 0},
	}
}

type MentionableAvailability struct {
	Available bool
	OwnerKind mentionableOwnerKind
	OwnerID   string
}

type MentionablesService struct {
	projection *MentionablesProjection
	projector  *events.Projector
}

func newMentionablesService(projection *MentionablesProjection, projector *events.Projector) *MentionablesService {
	return &MentionablesService{projection: projection, projector: projector}
}

func (s *MentionablesService) waitFor(ctx context.Context, pos events.StreamPosition) error {
	return s.projector.WaitFor(ctx, pos)
}

func (s *MentionablesService) Availability(handle string, allowedOwner *mentionableOwner) MentionableAvailability {
	return s.projection.Availability(handle, allowedOwner)
}

func normalizeMentionableHandle(handle string) string {
	return strings.ToLower(strings.TrimSpace(handle))
}

func mentionableRetryDelay(attempt int) time.Duration {
	return time.Duration(1<<attempt) * time.Millisecond
}

func (a MentionableAvailability) String() string {
	if a.Available {
		return "available"
	}
	if a.OwnerKind == "" || a.OwnerID == "" {
		return "unavailable"
	}
	return fmt.Sprintf("%s:%s", a.OwnerKind, a.OwnerID)
}
