package core

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const userSnapshotContractID = "v5"

func (*UserProjection) SnapshotContractID() string { return userSnapshotContractID }

func (p *UserProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()

	snapshot := &corev1.UserProfileProjectionSnapshot{ReplayGuard: snapshotReplayGuard(p.replayGuard)}
	for _, userID := range sortedMapKeys(p.users) {
		u := p.users[userID]
		if u == nil {
			continue
		}
		entry := &corev1.ProjectedUserProfileSnapshot{
			UserId:          userID,
			Login:           snapshotProjectedUserPII(u.login),
			LoginHash:       u.loginHash,
			DisplayName:     snapshotProjectedUserPII(u.displayName),
			BotDescription:  snapshotProjectedUserPII(u.botDescription),
			DeletionStarted: u.deletionStarted,
			Deleted:         u.deleted,
			Shredded:        u.shredded,
		}
		if u.user != nil {
			entry.User = proto.Clone(u.user).(*corev1.User)
			// These fields must never be populated in retained projection state.
			// Clear defensively so a regression cannot leak plaintext to storage.
			entry.User.Login = ""
			entry.User.DisplayName = ""
			if entry.User.GetBot() != nil {
				entry.User.GetBot().Description = ""
			}
		}
		if u.avatar != nil {
			entry.Avatar = proto.Clone(u.avatar).(*corev1.AssetRecord)
		}
		if u.preferences != nil {
			entry.Preferences = proto.Clone(u.preferences).(*corev1.ServerUserPreferences)
		}
		if !u.loginChanged.IsZero() {
			entry.LoginChangedAt = timestamppb.New(u.loginChanged)
		}
		for _, digest := range sortedMapKeys(u.verifiedEmail) {
			email := u.verifiedEmail[digest]
			entry.VerifiedEmails = append(entry.VerifiedEmails, &corev1.ProjectedVerifiedEmailSnapshot{
				Digest: digest, Value: snapshotProjectedUserPII(email.pii), VerifiedAt: timestamppb.New(email.verifiedAt),
			})
		}
		snapshot.Users = append(snapshot.Users, entry)
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
	for _, digest := range sortedMapKeys(p.loginIndex) {
		snapshot.LoginIndex = append(snapshot.LoginIndex, &corev1.StringStringSnapshot{Key: digest, Value: p.loginIndex[digest]})
	}
	for _, digest := range sortedMapKeys(p.emailIndex) {
		snapshot.EmailIndex = append(snapshot.EmailIndex, &corev1.StringStringSnapshot{Key: digest, Value: p.emailIndex[digest]})
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func snapshotProjectedUserPII(value *projectedUserPII) *corev1.ProjectedEncryptedUserStringSnapshot {
	if value == nil {
		return nil
	}
	out := &corev1.ProjectedEncryptedUserStringSnapshot{EventId: value.eventID, EventType: value.eventType, Purpose: value.purpose}
	if value.encrypted != nil {
		out.Encrypted = proto.Clone(value.encrypted).(*corev1.EncryptedUserString)
	}
	return out
}

func restoreProjectedUserPII(value *corev1.ProjectedEncryptedUserStringSnapshot) (*projectedUserPII, error) {
	if value == nil {
		return nil, nil
	}
	if value.GetEventId() == "" || value.GetEventType() == "" || value.GetPurpose() == "" || value.GetEncrypted() == nil || value.GetEncrypted().GetContentKeyEpoch() <= 0 {
		return nil, fmt.Errorf("invalid encrypted profile value")
	}
	return newProjectedUserPII(value.GetEventId(), value.GetEventType(), value.GetPurpose(), value.GetEncrypted()), nil
}

func (p *UserProjection) Restore(data []byte) error {
	snapshot := &corev1.UserProfileProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return fmt.Errorf("unmarshal user profile snapshot: %w", err)
		}
	}
	guard, err := restoreReplayGuard(snapshot.GetReplayGuard())
	if err != nil {
		return fmt.Errorf("user profile snapshot replay guard: %w", err)
	}
	restored := newUserProjectionWithDEKResolver(p.dekResolver)
	restored.replayGuard = guard

	seenKeys := make(map[string]struct{}, len(snapshot.GetKeys()))
	for _, key := range snapshot.GetKeys() {
		if key.GetUserId() == "" || key.GetEpoch() <= 0 || key.GetContentKeyRef() == "" {
			return fmt.Errorf("user profile snapshot has invalid DEK record")
		}
		identity := fmt.Sprintf("%s\x00%d\x00%d", key.GetUserId(), key.GetPurpose(), key.GetEpoch())
		if _, duplicate := seenKeys[identity]; duplicate {
			return fmt.Errorf("user profile snapshot repeats DEK record")
		}
		seenKeys[identity] = struct{}{}
		restored.applyDEKGenerated(key)
	}

	for _, entry := range snapshot.GetUsers() {
		userID := entry.GetUserId()
		if userID == "" {
			return fmt.Errorf("user profile snapshot has empty user ID")
		}
		if _, duplicate := restored.users[userID]; duplicate {
			return fmt.Errorf("user profile snapshot repeats user %q", userID)
		}
		if entry.GetUser() != nil && (entry.GetUser().GetId() != userID || entry.GetUser().GetLogin() != "" || entry.GetUser().GetDisplayName() != "" || entry.GetUser().GetBot().GetDescription() != "") {
			return fmt.Errorf("user profile snapshot has invalid or plaintext user %q", userID)
		}
		login, err := restoreProjectedUserPII(entry.GetLogin())
		if err != nil {
			return fmt.Errorf("user profile snapshot login for %q: %w", userID, err)
		}
		displayName, err := restoreProjectedUserPII(entry.GetDisplayName())
		if err != nil {
			return fmt.Errorf("user profile snapshot display name for %q: %w", userID, err)
		}
		if (login == nil) != (entry.GetLoginHash() == "") {
			return fmt.Errorf("user profile snapshot has inconsistent login for %q", userID)
		}
		botDescription, err := restoreProjectedUserPII(entry.GetBotDescription())
		if err != nil {
			return fmt.Errorf("user profile snapshot bot description for %q: %w", userID, err)
		}
		active := !entry.GetDeleted() && !entry.GetShredded()
		if active && (entry.GetUser() == nil || entry.GetUser().GetAccountProfile() == nil || login == nil || displayName == nil || (isBotAccount(entry.GetUser()) && botDescription == nil)) {
			return fmt.Errorf("user profile snapshot has incomplete active user %q", userID)
		}
		if !active && (login != nil || entry.GetLoginHash() != "" || displayName != nil || botDescription != nil || len(entry.GetVerifiedEmails()) > 0 || entry.GetPreferences() != nil || entry.GetLoginChangedAt() != nil) {
			return fmt.Errorf("user profile snapshot has profile state on inactive user %q", userID)
		}
		for name, pii := range map[string]*projectedUserPII{"login": login, "display name": displayName, "bot description": botDescription} {
			if pii != nil && !restored.hasUserPIIKeyLocked(userID, pii.encrypted.GetContentKeyEpoch()) {
				return fmt.Errorf("user profile snapshot %s for %q has no matching DEK", name, userID)
			}
		}
		u := &projectedUser{
			login: login, loginHash: entry.GetLoginHash(), displayName: displayName, botDescription: botDescription,
			deletionStarted: entry.GetDeletionStarted(), deleted: entry.GetDeleted(), shredded: entry.GetShredded(), verifiedEmail: make(map[string]projectedVerifiedEmail),
		}
		if entry.GetUser() != nil {
			u.user = proto.Clone(entry.GetUser()).(*corev1.User)
			if isBotAccount(u.user) && u.user.GetBot().GetOwnerId() == "" {
				return fmt.Errorf("user profile snapshot bot %q has no owner", userID)
			}
		}
		if entry.GetPreferences() != nil {
			u.preferences = proto.Clone(entry.GetPreferences()).(*corev1.ServerUserPreferences)
		}
		if entry.GetLoginChangedAt() != nil {
			if err := entry.GetLoginChangedAt().CheckValid(); err != nil {
				return fmt.Errorf("user profile snapshot login cooldown for %q: %w", userID, err)
			}
			u.loginChanged = entry.GetLoginChangedAt().AsTime()
		}
		for _, email := range entry.GetVerifiedEmails() {
			if email.GetDigest() == "" || email.GetValue() == nil || email.GetVerifiedAt() == nil {
				return fmt.Errorf("user profile snapshot has invalid verified email for %q", userID)
			}
			if _, duplicate := u.verifiedEmail[email.GetDigest()]; duplicate {
				return fmt.Errorf("user profile snapshot repeats verified email for %q", userID)
			}
			if err := email.GetVerifiedAt().CheckValid(); err != nil {
				return fmt.Errorf("user profile snapshot verified email time for %q: %w", userID, err)
			}
			pii, err := restoreProjectedUserPII(email.GetValue())
			if err != nil {
				return fmt.Errorf("user profile snapshot verified email for %q: %w", userID, err)
			}
			if !restored.hasUserPIIKeyLocked(userID, pii.encrypted.GetContentKeyEpoch()) {
				return fmt.Errorf("user profile snapshot verified email for %q has no matching DEK", userID)
			}
			u.verifiedEmail[email.GetDigest()] = projectedVerifiedEmail{pii: pii, verifiedAt: email.GetVerifiedAt().AsTime()}
		}
		restored.users[userID] = u
		if entry.GetAvatar() != nil {
			restored.replaceAvatarLocked(u, proto.Clone(entry.GetAvatar()).(*corev1.AssetRecord))
		}
	}

	restored.loginIndex, err = restoreUserProfileIndex(snapshot.GetLoginIndex(), restored.users, func(u *projectedUser, digest string) bool {
		return u.loginHash == digest
	})
	if err != nil {
		return fmt.Errorf("user profile snapshot login index: %w", err)
	}
	restored.emailIndex, err = restoreUserProfileIndex(snapshot.GetEmailIndex(), restored.users, func(u *projectedUser, digest string) bool {
		_, ok := u.verifiedEmail[digest]
		return ok
	})
	if err != nil {
		return fmt.Errorf("user profile snapshot email index: %w", err)
	}

	p.Lock()
	p.users, p.loginIndex, p.emailIndex, p.avatarIndex = restored.users, restored.loginIndex, restored.emailIndex, restored.avatarIndex
	p.replayGuard, p.dekEvents = restored.replayGuard, restored.dekEvents
	p.Unlock()
	return nil
}

func restoreUserProfileIndex(rows []*corev1.StringStringSnapshot, users map[string]*projectedUser, ownerMatches func(*projectedUser, string) bool) (map[string]string, error) {
	index := make(map[string]string, len(rows))
	for _, row := range rows {
		digest, userID := row.GetKey(), row.GetValue()
		if digest == "" || userID == "" {
			return nil, fmt.Errorf("has invalid entry")
		}
		if _, duplicate := index[digest]; duplicate {
			return nil, fmt.Errorf("repeats digest")
		}
		u := users[userID]
		if u == nil || u.deleted || u.shredded || !ownerMatches(u, digest) {
			return nil, fmt.Errorf("has invalid owner")
		}
		index[digest] = userID
	}
	return index, nil
}

func (p *UserProjection) hasUserPIIKeyLocked(userID string, epoch int32) bool {
	byPurpose := p.dekEvents[userID]
	if byPurpose == nil {
		return false
	}
	return byPurpose[corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII][epoch] != nil ||
		byPurpose[corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED][epoch] != nil
}
