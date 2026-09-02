package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/kms"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
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
	owners     map[string]map[mentionableOwner]struct{}
	userLogins map[string]string
	// userLoginSources retains only the latest encrypted login event per user.
	// Snapshot codecs use it instead of persisting the decrypted handle index.
	userLoginSources map[string]*evtv1.Event
	dekResolver      *unwrappedDEKResolver
	dekEvents        map[string]map[evtv1.UserDEKPurpose]map[int32]*evtv1.UserDEKGeneratedEvent
}

// NewMentionablesProjection creates the global mentionable-handle read model.
// It keeps encrypted DEK event references for replay, while raw key bytes are
// resolved through the shared unwrapped-DEK resolver.
func NewMentionablesProjection(keyWrapper kms.KeyWrapper, dekStore dekstore.Reader) *MentionablesProjection {
	return newMentionablesProjectionWithDEKResolver(newUnwrappedDEKResolver(keyWrapper, dekStore))
}

func newMentionablesProjectionWithDEKResolver(dekResolver *unwrappedDEKResolver) *MentionablesProjection {
	p := &MentionablesProjection{
		owners:           make(map[string]map[mentionableOwner]struct{}),
		userLogins:       make(map[string]string),
		userLoginSources: make(map[string]*evtv1.Event),
		dekResolver:      dekResolver,
		dekEvents:        make(map[string]map[evtv1.UserDEKPurpose]map[int32]*evtv1.UserDEKGeneratedEvent),
	}
	p.addOwner(MentionHandleAll, mentionableOwner{kind: mentionableOwnerVirtual, id: MentionHandleAll})
	p.addOwner(MentionHandleHere, mentionableOwner{kind: mentionableOwnerVirtual, id: MentionHandleHere})
	return p
}

func (p *MentionablesProjection) Subjects() []string {
	return []string{evtstream.EventSubjectFilter()}
}

func (p *MentionablesProjection) Apply(event *evtv1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()

	switch e := event.GetEvent().(type) {
	case *evtv1.Event_UserDekGenerated:
		p.applyDEKGenerated(e.UserDekGenerated)
	case *evtv1.Event_UserAccountCreated:
		if err := p.applyUserAccountCreated(event.GetId(), e.UserAccountCreated); err != nil {
			return err
		}
		if p.userLogins[e.UserAccountCreated.GetUserId()] != "" {
			p.userLoginSources[e.UserAccountCreated.GetUserId()] = proto.Clone(event).(*evtv1.Event)
		}
	case *evtv1.Event_UserLoginChanged:
		if err := p.applyUserLoginChanged(event.GetId(), e.UserLoginChanged); err != nil {
			return err
		}
		if p.userLogins[e.UserLoginChanged.GetUserId()] != "" {
			p.userLoginSources[e.UserLoginChanged.GetUserId()] = proto.Clone(event).(*evtv1.Event)
		}
	case *evtv1.Event_UserAccountDeleted:
		p.applyUserAccountDeleted(e.UserAccountDeleted)
		delete(p.userLoginSources, e.UserAccountDeleted.GetUserId())
	case *evtv1.Event_UserKeyShreddingRequested:
		p.applyUserKeyShredded(e.UserKeyShreddingRequested.GetUserId())
		delete(p.userLoginSources, e.UserKeyShreddingRequested.GetUserId())
	case *evtv1.Event_UserKeyShredded:
		p.applyUserKeyShredded(e.UserKeyShredded.GetUserId())
		delete(p.userLoginSources, e.UserKeyShredded.GetUserId())
	case *evtv1.Event_RbacRoleCreated:
		p.addOwner(e.RbacRoleCreated.GetRoleName(), mentionableOwner{kind: mentionableOwnerRole, id: strings.ToLower(e.RbacRoleCreated.GetRoleName())})
	case *evtv1.Event_RbacRoleDeleted:
		roleName := strings.ToLower(e.RbacRoleDeleted.GetRoleName())
		p.removeOwner(roleName, mentionableOwner{kind: mentionableOwnerRole, id: roleName})
	}
	return nil
}

type preparedMentionablesEvent struct {
	login string
	ready bool
}

// Prepare resolves encrypted user handles before the content view commits any
// component mutation.
func (p *MentionablesProjection) Prepare(event *evtv1.Event, _ uint64) (events.PreparedMutation, error) {
	if event == nil {
		return nil, nil
	}
	prepared := preparedMentionablesEvent{}
	p.RLock()
	var err error
	switch value := event.GetEvent().(type) {
	case *evtv1.Event_UserAccountCreated:
		created := value.UserAccountCreated
		if created != nil && created.GetUserId() != "" {
			prepared.login, prepared.ready, err = p.userPIIString(
				event.GetId(), created.GetUserId(), evtstream.EventUserAccountCreated, "login", created.GetEncryptedLogin(),
			)
		}
	case *evtv1.Event_UserLoginChanged:
		changed := value.UserLoginChanged
		if changed != nil && changed.GetUserId() != "" {
			prepared.login, prepared.ready, err = p.userPIIString(
				event.GetId(), changed.GetUserId(), evtstream.EventUserLoginChanged, "login", changed.GetEncryptedLogin(),
			)
		}
	}
	p.RUnlock()
	if err != nil {
		return nil, err
	}
	return events.PreparedMutationFunc(func() {
		p.applyPreparedMentionablesEvent(event, prepared)
	}), nil
}

func (p *MentionablesProjection) applyPreparedMentionablesEvent(event *evtv1.Event, prepared preparedMentionablesEvent) {
	p.Lock()
	defer p.Unlock()
	switch value := event.GetEvent().(type) {
	case *evtv1.Event_UserDekGenerated:
		p.applyDEKGenerated(value.UserDekGenerated)
	case *evtv1.Event_UserAccountCreated:
		if prepared.ready && prepared.login != "" {
			p.setUserLogin(value.UserAccountCreated.GetUserId(), prepared.login)
			p.userLoginSources[value.UserAccountCreated.GetUserId()] = proto.Clone(event).(*evtv1.Event)
		}
	case *evtv1.Event_UserLoginChanged:
		if prepared.ready && prepared.login != "" {
			p.setUserLogin(value.UserLoginChanged.GetUserId(), prepared.login)
			p.userLoginSources[value.UserLoginChanged.GetUserId()] = proto.Clone(event).(*evtv1.Event)
		}
	case *evtv1.Event_UserAccountDeleted:
		p.applyUserAccountDeleted(value.UserAccountDeleted)
		delete(p.userLoginSources, value.UserAccountDeleted.GetUserId())
	case *evtv1.Event_UserKeyShreddingRequested:
		p.applyUserKeyShredded(value.UserKeyShreddingRequested.GetUserId())
		delete(p.userLoginSources, value.UserKeyShreddingRequested.GetUserId())
	case *evtv1.Event_UserKeyShredded:
		p.applyUserKeyShredded(value.UserKeyShredded.GetUserId())
		delete(p.userLoginSources, value.UserKeyShredded.GetUserId())
	case *evtv1.Event_RbacRoleCreated:
		p.addOwner(value.RbacRoleCreated.GetRoleName(), mentionableOwner{kind: mentionableOwnerRole, id: strings.ToLower(value.RbacRoleCreated.GetRoleName())})
	case *evtv1.Event_RbacRoleDeleted:
		roleName := strings.ToLower(value.RbacRoleDeleted.GetRoleName())
		p.removeOwner(roleName, mentionableOwner{kind: mentionableOwnerRole, id: roleName})
	}
}

func (p *MentionablesProjection) applyDEKGenerated(e *evtv1.UserDEKGeneratedEvent) {
	if e == nil || e.GetUserId() == "" || e.GetEpoch() <= 0 || e.GetContentKeyRef() == "" {
		return
	}
	byPurpose := p.dekEvents[e.GetUserId()]
	if byPurpose == nil {
		byPurpose = make(map[evtv1.UserDEKPurpose]map[int32]*evtv1.UserDEKGeneratedEvent)
		p.dekEvents[e.GetUserId()] = byPurpose
	}
	epochs := byPurpose[e.GetPurpose()]
	if epochs == nil {
		epochs = make(map[int32]*evtv1.UserDEKGeneratedEvent)
		byPurpose[e.GetPurpose()] = epochs
	}
	epochs[e.GetEpoch()] = proto.Clone(e).(*evtv1.UserDEKGeneratedEvent)
}

func (p *MentionablesProjection) applyUserAccountCreated(eventID string, e *evtv1.UserAccountCreatedEvent) error {
	if e == nil || e.GetUserId() == "" {
		return nil
	}
	login, ok, err := p.userPIIString(eventID, e.GetUserId(), evtstream.EventUserAccountCreated, "login", e.GetEncryptedLogin())
	if err != nil {
		return err
	}
	if !ok || login == "" {
		return nil
	}
	p.setUserLogin(e.GetUserId(), login)
	return nil
}

func (p *MentionablesProjection) applyUserLoginChanged(eventID string, e *evtv1.UserLoginChangedEvent) error {
	if e == nil || e.GetUserId() == "" {
		return nil
	}
	login, ok, err := p.userPIIString(eventID, e.GetUserId(), evtstream.EventUserLoginChanged, "login", e.GetEncryptedLogin())
	if err != nil {
		return err
	}
	if !ok || login == "" {
		return nil
	}
	p.setUserLogin(e.GetUserId(), login)
	return nil
}

func (p *MentionablesProjection) applyUserAccountDeleted(e *evtv1.UserAccountDeletedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	p.removeUserLogin(e.GetUserId())
}

func (p *MentionablesProjection) applyUserKeyShredded(userID string) {
	if userID == "" {
		return
	}
	delete(p.dekEvents, userID)
	p.removeUserLogin(userID)
}

func (p *MentionablesProjection) setUserLogin(userID, login string) {
	p.removeUserLogin(userID)
	key := mentionableLookupKey(login)
	if key == "" {
		return
	}
	p.userLogins[userID] = key
	p.addOwnerKey(key, mentionableOwner{kind: mentionableOwnerUser, id: userID})
}

func (p *MentionablesProjection) removeUserLogin(userID string) {
	old := p.userLogins[userID]
	if old == "" {
		return
	}
	delete(p.userLogins, userID)
	p.removeOwnerKey(old, mentionableOwner{kind: mentionableOwnerUser, id: userID})
}

func (p *MentionablesProjection) addOwner(handle string, owner mentionableOwner) {
	p.addOwnerKey(mentionableLookupKey(handle), owner)
}

func (p *MentionablesProjection) addOwnerKey(key string, owner mentionableOwner) {
	if key == "" || owner.kind == "" || owner.id == "" {
		return
	}
	owners := p.owners[key]
	if owners == nil {
		owners = make(map[mentionableOwner]struct{})
		p.owners[key] = owners
	}
	owners[owner] = struct{}{}
}

func (p *MentionablesProjection) removeOwner(handle string, owner mentionableOwner) {
	p.removeOwnerKey(mentionableLookupKey(handle), owner)
}

func (p *MentionablesProjection) removeOwnerKey(key string, owner mentionableOwner) {
	owners := p.owners[key]
	if owners == nil {
		return
	}
	delete(owners, owner)
	if len(owners) == 0 {
		delete(p.owners, key)
	}
}

func (p *MentionablesProjection) userPIIString(eventID, userID, eventType, purpose string, encrypted *evtv1.EncryptedUserString) (string, bool, error) {
	if encrypted == nil {
		return "", false, nil
	}
	byPurpose := p.dekEvents[userID]
	if byPurpose == nil {
		return "", false, nil
	}
	event := byPurpose[evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII][encrypted.GetContentKeyEpoch()]
	if event == nil {
		event = byPurpose[evtv1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED][encrypted.GetContentKeyEpoch()]
	}
	if event == nil || p.dekResolver == nil {
		return "", false, nil
	}
	dek, err := p.dekResolver.Resolve(context.Background(), event, evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII)
	if err != nil {
		if errors.Is(err, encryption.ErrKeyNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("resolve mentionable user PII key: %w", err)
	}
	if dek == nil || len(dek.key) == 0 {
		return "", false, nil
	}
	plaintext, err := decryptUserPIIString(dek.key, eventID, userID, eventType, purpose, encrypted)
	if err != nil {
		return "", false, fmt.Errorf("decrypt mentionable user PII: %w", err)
	}
	return plaintext, true, nil
}

func (p *MentionablesProjection) Availability(handle string, allowedOwner *mentionableOwner) MentionableAvailability {
	key := mentionableLookupKey(handle)
	if key == "" {
		return MentionableAvailability{Available: false}
	}
	p.RLock()
	defer p.RUnlock()
	owners := p.owners[key]
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

type MentionablesModel struct {
	mentionables events.ProjectionHandle[*MentionablesProjection]
}

func newMentionablesModel(mentionables events.ProjectionHandle[*MentionablesProjection]) *MentionablesModel {
	return &MentionablesModel{mentionables: mentionables}
}

func (s *MentionablesModel) waitFor(ctx context.Context, pos events.StreamPosition) error {
	return s.mentionables.Projector().WaitFor(ctx, pos)
}

func (s *MentionablesModel) Availability(handle string, allowedOwner *mentionableOwner) MentionableAvailability {
	return s.mentionables.Projection().Availability(handle, allowedOwner)
}

func normalizeMentionableHandle(handle string) string {
	return strings.ToLower(strings.TrimSpace(handle))
}

func mentionableLookupKey(handle string) string {
	normalized := normalizeMentionableHandle(handle)
	if normalized == "" {
		return ""
	}
	return userPIILookupHash(normalized)
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
