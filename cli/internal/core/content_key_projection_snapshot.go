package core

import (
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/projection/v1"
	"sort"

	"google.golang.org/protobuf/proto"
)

var contentKeySnapshotContractID = snapshotContractID("v1", &projectionv1.ContentKeyProjectionSnapshot{})

func (*ContentKeyProjection) SnapshotContractID() string {
	return contentKeySnapshotContractID
}

func (p *ContentKeyProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	snapshot := &projectionv1.ContentKeyProjectionSnapshot{ReplayGuard: snapshotReplayGuard(p.replayGuard)}
	snapshot.ShreddedUserIds = sortedMapKeys(p.shreddedUsers)
	ids := make([]contentKeyID, 0, len(p.keys))
	for id := range p.keys {
		ids = append(ids, id)
	}
	// Keys are ordered by user ID, purpose, and epoch.
	sort.Slice(ids, func(i, j int) bool {
		a, b := ids[i], ids[j]
		if a.user != b.user {
			return p.users.id(a.user) < p.users.id(b.user)
		}
		if a.purpose != b.purpose {
			return a.purpose < b.purpose
		}
		return a.epoch < b.epoch
	})
	for _, id := range ids {
		snapshot.Keys = append(snapshot.Keys, p.keys[id].event(p.users.id(id.user), id))
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *ContentKeyProjection) Restore(data []byte) error {
	snapshot := &projectionv1.ContentKeyProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return fmt.Errorf("unmarshal content key snapshot: %w", err)
		}
	}
	guard, err := restoreReplayGuard(snapshot.GetReplayGuard())
	if err != nil {
		return fmt.Errorf("content key snapshot replay guard: %w", err)
	}
	restored := NewContentKeyProjection()
	restored.replayGuard = guard
	for _, userID := range snapshot.GetShreddedUserIds() {
		if userID == "" {
			return fmt.Errorf("content key snapshot has empty shredded user id")
		}
		if _, duplicate := restored.shreddedUsers[userID]; duplicate {
			return fmt.Errorf("content key snapshot repeats shredded user %q", userID)
		}
		restored.shreddedUsers[userID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(snapshot.GetKeys()))
	for _, key := range snapshot.GetKeys() {
		if key.GetUserId() == "" || key.GetEpoch() <= 0 || key.GetContentKeyRef() == "" {
			return fmt.Errorf("content key snapshot has invalid key")
		}
		identity := fmt.Sprintf("%s\x00%d\x00%d", key.GetUserId(), key.GetPurpose(), key.GetEpoch())
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("content key snapshot repeats key %q", identity)
		}
		seen[identity] = struct{}{}
		restored.applyDEKGeneratedLocked(key)
	}
	p.Lock()
	p.users, p.keys, p.activeEpoch, p.algorithms, p.shreddedUsers, p.replayGuard = restored.users, restored.keys, restored.activeEpoch, restored.algorithms, restored.shreddedUsers, restored.replayGuard
	p.Unlock()
	return nil
}
