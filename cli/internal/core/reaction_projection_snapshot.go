package core

import (
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/projection/v1"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"
)

var reactionSnapshotContractID = snapshotContractID("v1", &projectionv1.ReactionProjectionSnapshot{})

func (*ReactionProjection) SnapshotContractID() string { return reactionSnapshotContractID }

func (p *ReactionProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	snapshot := &projectionv1.ReactionProjectionSnapshot{ReplayGuard: snapshotReplayGuard(p.replayGuard)}
	for _, message := range sortedHandleKeys(p.messages, p.byMessage) {
		reactions := slices.Clone(p.byMessage[message])
		slices.SortFunc(reactions, func(a, b reactionProjectionEntry) int {
			if byEmoji := strings.Compare(p.ids.id(a.emoji), p.ids.id(b.emoji)); byEmoji != 0 {
				return byEmoji
			}
			return strings.Compare(p.ids.id(a.user), p.ids.id(b.user))
		})
		row := &projectionv1.MessageReactionsSnapshot{MessageEventId: p.messages.id(message)}
		var group *projectionv1.EmojiReactionsSnapshot
		for _, reaction := range reactions {
			if emoji := p.ids.id(reaction.emoji); group == nil || group.Emoji != emoji {
				group = &projectionv1.EmojiReactionsSnapshot{Emoji: emoji}
				row.Emojis = append(row.Emojis, group)
			}
			group.Users = append(group.Users, &projectionv1.UserReactionSnapshot{
				UserId: p.ids.id(reaction.user), AddedAtNanos: reaction.addedAtNanos, SourceEventId: reaction.source,
			})
		}
		snapshot.Messages = append(snapshot.Messages, row)
	}
	for _, key := range sortedMapKeys(p.roomSeq) {
		snapshot.RoomSequences = append(snapshot.RoomSequences, &projectionv1.StringUint64Snapshot{Key: key, Value: p.roomSeq[key]})
	}
	sortedRows := func(rows []*projectionv1.StringStringSnapshot) []*projectionv1.StringStringSnapshot {
		slices.SortFunc(rows, func(a, b *projectionv1.StringStringSnapshot) int { return strings.Compare(a.Key, b.Key) })
		return rows
	}
	messageRooms := make([]*projectionv1.StringStringSnapshot, 0, len(p.messageRooms))
	for i, room := range p.messageRooms {
		if room != 0 {
			messageRooms = append(messageRooms, &projectionv1.StringStringSnapshot{Key: p.messages.id(uint32(i + 1)), Value: p.ids.id(room)})
		}
	}
	snapshot.MessageRooms = sortedRows(messageRooms)
	echoOriginals := make([]*projectionv1.StringStringSnapshot, 0, len(p.echoOriginal))
	for echo, original := range p.echoOriginal {
		echoOriginals = append(echoOriginals, &projectionv1.StringStringSnapshot{Key: p.messages.id(echo), Value: p.messages.id(original)})
	}
	snapshot.EchoOriginals = sortedRows(echoOriginals)
	assetRooms := make([]*projectionv1.StringStringSnapshot, 0, len(p.assetRoom))
	for asset, room := range p.assetRoom {
		assetRooms = append(assetRooms, &projectionv1.StringStringSnapshot{Key: asset, Value: room})
	}
	snapshot.AssetRooms = sortedRows(assetRooms)
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *ReactionProjection) Restore(data []byte) error {
	snapshot := &projectionv1.ReactionProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return fmt.Errorf("unmarshal reaction snapshot: %w", err)
		}
	}
	guard, err := restoreReplayGuard(snapshot.GetReplayGuard())
	if err != nil {
		return fmt.Errorf("reaction snapshot replay guard: %w", err)
	}
	// The restored model interns into the same message table so handles stay
	// shared with the other ServerContentView components.
	restored := newReactionProjection(p.messages)
	restored.sharedEventIDs = p.sharedEventIDs
	restored.replayGuard = guard
	for _, message := range snapshot.GetMessages() {
		if message.GetMessageEventId() == "" {
			return fmt.Errorf("reaction snapshot has empty message ID")
		}
		messageHandle := restored.messages.intern(message.GetMessageEventId())
		if _, duplicate := restored.byMessage[messageHandle]; duplicate {
			return fmt.Errorf("reaction snapshot repeats message %q", message.GetMessageEventId())
		}
		var reactions []reactionProjectionEntry
		seenEmojis := make(map[uint32]struct{}, len(message.GetEmojis()))
		for _, group := range message.GetEmojis() {
			if group.GetEmoji() == "" {
				return fmt.Errorf("reaction snapshot has empty emoji")
			}
			emoji := restored.ids.intern(group.GetEmoji())
			if _, duplicate := seenEmojis[emoji]; duplicate {
				return fmt.Errorf("reaction snapshot repeats emoji")
			}
			seenEmojis[emoji] = struct{}{}
			for _, user := range group.GetUsers() {
				if user.GetUserId() == "" {
					return fmt.Errorf("reaction snapshot has empty user ID")
				}
				reactions = append(reactions, reactionProjectionEntry{
					addedAtNanos: user.GetAddedAtNanos(), source: user.GetSourceEventId(),
					emoji: emoji, user: restored.ids.intern(user.GetUserId()),
				})
			}
		}
		slices.SortFunc(reactions, compareReactions)
		for i := 1; i < len(reactions); i++ {
			if compareReactions(reactions[i-1], reactions[i]) == 0 {
				return fmt.Errorf("reaction snapshot repeats user")
			}
		}
		if len(reactions) > 0 {
			restored.byMessage[messageHandle] = reactions
		}
	}
	for _, row := range snapshot.GetRoomSequences() {
		if row.GetKey() == "" {
			return fmt.Errorf("reaction snapshot has empty room sequence key")
		}
		if _, duplicate := restored.roomSeq[row.GetKey()]; duplicate {
			return fmt.Errorf("reaction snapshot repeats room sequence")
		}
		restored.roomSeq[row.GetKey()] = row.GetValue()
	}
	validRows := func(rows []*projectionv1.StringStringSnapshot) error {
		seen := make(map[string]struct{}, len(rows))
		for _, row := range rows {
			if row.GetKey() == "" || row.GetValue() == "" {
				return fmt.Errorf("reaction snapshot has invalid string mapping")
			}
			if _, duplicate := seen[row.GetKey()]; duplicate {
				return fmt.Errorf("reaction snapshot repeats string mapping")
			}
			seen[row.GetKey()] = struct{}{}
		}
		return nil
	}
	for _, rows := range [][]*projectionv1.StringStringSnapshot{snapshot.GetMessageRooms(), snapshot.GetEchoOriginals(), snapshot.GetAssetRooms()} {
		if err := validRows(rows); err != nil {
			return err
		}
	}
	for _, row := range snapshot.GetMessageRooms() {
		restored.messageRooms.set(restored.messages.intern(row.GetKey()), restored.ids.intern(row.GetValue()))
	}
	for _, row := range snapshot.GetEchoOriginals() {
		restored.echoOriginal[restored.messages.intern(row.GetKey())] = restored.messages.intern(row.GetValue())
	}
	for _, row := range snapshot.GetAssetRooms() {
		restored.assetRoom[row.GetKey()] = row.GetValue()
	}
	p.Lock()
	p.ids, p.byMessage, p.roomSeq, p.messageRooms, p.echoOriginal, p.assetRoom, p.replayGuard = restored.ids, restored.byMessage, restored.roomSeq, restored.messageRooms, restored.echoOriginal, restored.assetRoom, restored.replayGuard
	p.Unlock()
	return nil
}
