package core

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const reactionSnapshotCompatibilityID = "v1"

func (*ReactionProjection) SnapshotCompatibilityID() string { return reactionSnapshotCompatibilityID }

func (p *ReactionProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	snapshot := &corev1.ReactionProjectionSnapshot{ReplayGuard: snapshotReplayGuard(p.replayGuard)}
	for _, messageID := range sortedMapKeys(p.byMessage) {
		message := &corev1.MessageReactionsSnapshot{MessageEventId: messageID}
		for _, emoji := range sortedMapKeys(p.byMessage[messageID]) {
			group := &corev1.EmojiReactionsSnapshot{Emoji: emoji}
			for _, userID := range sortedMapKeys(p.byMessage[messageID][emoji]) {
				group.Users = append(group.Users, &corev1.UserReactionSnapshot{UserId: userID, AddedAtNanos: p.byMessage[messageID][emoji][userID]})
			}
			message.Emojis = append(message.Emojis, group)
		}
		snapshot.Messages = append(snapshot.Messages, message)
	}
	for _, key := range sortedMapKeys(p.roomSeq) {
		snapshot.RoomSequences = append(snapshot.RoomSequences, &corev1.StringUint64Snapshot{Key: key, Value: p.roomSeq[key]})
	}
	appendStrings := func(values map[string]string) []*corev1.StringStringSnapshot {
		rows := make([]*corev1.StringStringSnapshot, 0, len(values))
		for _, key := range sortedMapKeys(values) {
			rows = append(rows, &corev1.StringStringSnapshot{Key: key, Value: values[key]})
		}
		return rows
	}
	snapshot.MessageRooms = appendStrings(p.messageRoom)
	snapshot.EchoOriginals = appendStrings(p.echoOriginal)
	snapshot.AssetRooms = appendStrings(p.assetRoom)
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *ReactionProjection) Restore(data []byte) error {
	snapshot := &corev1.ReactionProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return fmt.Errorf("unmarshal reaction snapshot: %w", err)
		}
	}
	guard, err := restoreReplayGuard(snapshot.GetReplayGuard())
	if err != nil {
		return fmt.Errorf("reaction snapshot replay guard: %w", err)
	}
	byMessage := make(map[string]map[string]map[string]int64, len(snapshot.GetMessages()))
	for _, message := range snapshot.GetMessages() {
		if message.GetMessageEventId() == "" {
			return fmt.Errorf("reaction snapshot has empty message ID")
		}
		if _, duplicate := byMessage[message.GetMessageEventId()]; duplicate {
			return fmt.Errorf("reaction snapshot repeats message %q", message.GetMessageEventId())
		}
		emojis := make(map[string]map[string]int64)
		for _, group := range message.GetEmojis() {
			if group.GetEmoji() == "" {
				return fmt.Errorf("reaction snapshot has empty emoji")
			}
			if _, duplicate := emojis[group.GetEmoji()]; duplicate {
				return fmt.Errorf("reaction snapshot repeats emoji")
			}
			users := make(map[string]int64)
			for _, user := range group.GetUsers() {
				if user.GetUserId() == "" {
					return fmt.Errorf("reaction snapshot has empty user ID")
				}
				if _, duplicate := users[user.GetUserId()]; duplicate {
					return fmt.Errorf("reaction snapshot repeats user")
				}
				users[user.GetUserId()] = user.GetAddedAtNanos()
			}
			emojis[group.GetEmoji()] = users
		}
		byMessage[message.GetMessageEventId()] = emojis
	}
	roomSeq := make(map[string]uint64)
	for _, row := range snapshot.GetRoomSequences() {
		if row.GetKey() == "" {
			return fmt.Errorf("reaction snapshot has empty room sequence key")
		}
		if _, duplicate := roomSeq[row.GetKey()]; duplicate {
			return fmt.Errorf("reaction snapshot repeats room sequence")
		}
		roomSeq[row.GetKey()] = row.GetValue()
	}
	restoreStrings := func(rows []*corev1.StringStringSnapshot) (map[string]string, error) {
		values := make(map[string]string, len(rows))
		for _, row := range rows {
			if row.GetKey() == "" || row.GetValue() == "" {
				return nil, fmt.Errorf("reaction snapshot has invalid string mapping")
			}
			if _, duplicate := values[row.GetKey()]; duplicate {
				return nil, fmt.Errorf("reaction snapshot repeats string mapping")
			}
			values[row.GetKey()] = row.GetValue()
		}
		return values, nil
	}
	messageRoom, err := restoreStrings(snapshot.GetMessageRooms())
	if err != nil {
		return err
	}
	echoOriginal, err := restoreStrings(snapshot.GetEchoOriginals())
	if err != nil {
		return err
	}
	assetRoom, err := restoreStrings(snapshot.GetAssetRooms())
	if err != nil {
		return err
	}
	p.Lock()
	p.byMessage, p.roomSeq, p.messageRoom, p.echoOriginal, p.assetRoom, p.replayGuard = byMessage, roomSeq, messageRoom, echoOriginal, assetRoom, guard
	p.Unlock()
	return nil
}
