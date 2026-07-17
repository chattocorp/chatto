package core

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const mentionablesSnapshotContractID = "v1"

func (*MentionablesProjection) SnapshotContractID() string {
	return mentionablesSnapshotContractID
}

func (p *MentionablesProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	snapshot := &corev1.MentionablesProjectionSnapshot{}
	for _, userID := range sortedMapKeys(p.userLoginSources) {
		snapshot.UserLoginSources = append(snapshot.UserLoginSources, proto.Clone(p.userLoginSources[userID]).(*corev1.Event))
	}
	for _, userID := range sortedMapKeys(p.dekEvents) {
		purposes := make([]int, 0, len(p.dekEvents[userID]))
		for purpose := range p.dekEvents[userID] {
			purposes = append(purposes, int(purpose))
		}
		sort.Ints(purposes)
		for _, rawPurpose := range purposes {
			purpose := corev1.UserDEKPurpose(rawPurpose)
			epochs := make([]int, 0, len(p.dekEvents[userID][purpose]))
			for epoch := range p.dekEvents[userID][purpose] {
				epochs = append(epochs, int(epoch))
			}
			sort.Ints(epochs)
			for _, epoch := range epochs {
				snapshot.Keys = append(snapshot.Keys, proto.Clone(p.dekEvents[userID][purpose][int32(epoch)]).(*corev1.UserDEKGeneratedEvent))
			}
		}
	}
	roleSet := make(map[string]struct{})
	for _, owners := range p.owners {
		for owner := range owners {
			if owner.kind == mentionableOwnerRole {
				roleSet[owner.id] = struct{}{}
			}
		}
	}
	snapshot.RoleNames = sortedMapKeys(roleSet)
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *MentionablesProjection) Restore(data []byte) error {
	snapshot := &corev1.MentionablesProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return fmt.Errorf("unmarshal mentionables snapshot: %w", err)
		}
	}
	restored := newMentionablesProjectionWithDEKResolver(p.dekResolver)
	seenKeys := make(map[string]struct{}, len(snapshot.GetKeys()))
	for _, key := range snapshot.GetKeys() {
		identity := fmt.Sprintf("%s\x00%d\x00%d", key.GetUserId(), key.GetPurpose(), key.GetEpoch())
		if _, duplicate := seenKeys[identity]; duplicate {
			return fmt.Errorf("mentionables snapshot repeats key")
		}
		seenKeys[identity] = struct{}{}
		restored.applyDEKGenerated(key)
	}
	for _, event := range snapshot.GetUserLoginSources() {
		if event.GetId() == "" {
			return fmt.Errorf("mentionables snapshot has login source without event ID")
		}
		var userID string
		switch value := event.GetEvent().(type) {
		case *corev1.Event_UserAccountCreated:
			userID = value.UserAccountCreated.GetUserId()
			restored.applyUserAccountCreated(event.GetId(), value.UserAccountCreated)
		case *corev1.Event_UserLoginChanged:
			userID = value.UserLoginChanged.GetUserId()
			restored.applyUserLoginChanged(event.GetId(), value.UserLoginChanged)
		default:
			return fmt.Errorf("mentionables snapshot has invalid login source")
		}
		if userID == "" || restored.userLogins[userID] == "" {
			return fmt.Errorf("mentionables snapshot login source could not be restored")
		}
		if _, duplicate := restored.userLoginSources[userID]; duplicate {
			return fmt.Errorf("mentionables snapshot repeats user %q", userID)
		}
		restored.userLoginSources[userID] = proto.Clone(event).(*corev1.Event)
	}
	seenRoles := make(map[string]struct{}, len(snapshot.GetRoleNames()))
	for _, roleName := range snapshot.GetRoleNames() {
		if roleName == "" {
			return fmt.Errorf("mentionables snapshot has empty role name")
		}
		if _, duplicate := seenRoles[roleName]; duplicate {
			return fmt.Errorf("mentionables snapshot repeats role %q", roleName)
		}
		seenRoles[roleName] = struct{}{}
		restored.addOwner(roleName, mentionableOwner{kind: mentionableOwnerRole, id: roleName})
	}
	p.Lock()
	p.owners, p.userLogins, p.userLoginSources, p.dekEvents = restored.owners, restored.userLogins, restored.userLoginSources, restored.dekEvents
	p.Unlock()
	return nil
}
