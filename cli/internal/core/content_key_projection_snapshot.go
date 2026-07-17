package core

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const contentKeySnapshotContractID = "v1"

func (*ContentKeyProjection) SnapshotContractID() string {
	return contentKeySnapshotContractID
}

func (p *ContentKeyProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	snapshot := &corev1.ContentKeyProjectionSnapshot{ReplayGuard: snapshotReplayGuard(p.replayGuard)}
	for _, userID := range sortedMapKeys(p.byUserPurposeEpoch) {
		purposes := make([]int, 0, len(p.byUserPurposeEpoch[userID]))
		for purpose := range p.byUserPurposeEpoch[userID] {
			purposes = append(purposes, int(purpose))
		}
		sort.Ints(purposes)
		for _, rawPurpose := range purposes {
			purpose := corev1.UserDEKPurpose(rawPurpose)
			epochs := make([]int, 0, len(p.byUserPurposeEpoch[userID][purpose]))
			for epoch := range p.byUserPurposeEpoch[userID][purpose] {
				epochs = append(epochs, int(epoch))
			}
			sort.Ints(epochs)
			for _, epoch := range epochs {
				snapshot.Keys = append(snapshot.Keys, proto.Clone(p.byUserPurposeEpoch[userID][purpose][int32(epoch)]).(*corev1.UserDEKGeneratedEvent))
			}
		}
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *ContentKeyProjection) Restore(data []byte) error {
	snapshot := &corev1.ContentKeyProjectionSnapshot{}
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
	p.byUserPurposeEpoch, p.activeEpoch, p.replayGuard = restored.byUserPurposeEpoch, restored.activeEpoch, restored.replayGuard
	p.Unlock()
	return nil
}
