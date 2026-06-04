package core

import (
	"slices"
	"sort"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ReactionProjection derives current reaction state from durable
// room-aggregate reaction events. v1 intentionally keeps the whole
// current reaction set in RAM; bounded/windowed variants can build on
// this once real access patterns are known.
type ReactionProjection struct {
	events.MemoryProjection
	byMessage map[string]map[string]map[string]int64 // message event ID -> emoji -> user ID -> added timestamp
	seen      map[string]struct{}
}

func NewReactionProjection() *ReactionProjection {
	return &ReactionProjection{
		byMessage: make(map[string]map[string]map[string]int64),
		seen:      make(map[string]struct{}),
	}
}

func (p *ReactionProjection) Subjects() []string {
	return []string{
		events.RoomEventTypeFilter(events.EventReactionAdded),
		events.RoomEventTypeFilter(events.EventReactionRemoved),
	}
}

func (p *ReactionProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	payload := event.GetEvent()
	switch payload.(type) {
	case *corev1.Event_ReactionAdded, *corev1.Event_ReactionRemoved:
	default:
		return nil
	}

	p.Lock()
	defer p.Unlock()

	if id := event.GetId(); id != "" {
		if _, ok := p.seen[id]; ok {
			return nil
		}
		p.seen[id] = struct{}{}
	}

	switch e := payload.(type) {
	case *corev1.Event_ReactionAdded:
		p.applyAdded(e.ReactionAdded, event.GetActorId(), eventCreatedNanos(event))
	case *corev1.Event_ReactionRemoved:
		p.applyRemoved(e.ReactionRemoved, event.GetActorId())
	}
	return nil
}

func (p *ReactionProjection) applyAdded(e *corev1.ReactionAddedEvent, userID string, nanos int64) {
	if e == nil || userID == "" || e.GetMessageEventId() == "" || e.GetEmoji() == "" {
		return
	}
	byEmoji := p.byMessage[e.GetMessageEventId()]
	if byEmoji == nil {
		byEmoji = make(map[string]map[string]int64)
		p.byMessage[e.GetMessageEventId()] = byEmoji
	}
	byUser := byEmoji[e.GetEmoji()]
	if byUser == nil {
		byUser = make(map[string]int64)
		byEmoji[e.GetEmoji()] = byUser
	}
	if _, exists := byUser[userID]; !exists {
		byUser[userID] = nanos
	}
}

func (p *ReactionProjection) applyRemoved(e *corev1.ReactionRemovedEvent, userID string) {
	if e == nil || userID == "" || e.GetMessageEventId() == "" || e.GetEmoji() == "" {
		return
	}
	byEmoji := p.byMessage[e.GetMessageEventId()]
	if byEmoji == nil {
		return
	}
	byUser := byEmoji[e.GetEmoji()]
	if byUser == nil {
		return
	}
	delete(byUser, userID)
	if len(byUser) == 0 {
		delete(byEmoji, e.GetEmoji())
	}
	if len(byEmoji) == 0 {
		delete(p.byMessage, e.GetMessageEventId())
	}
}

func eventCreatedNanos(event *corev1.Event) int64 {
	if ts := event.GetCreatedAt(); ts != nil {
		return ts.AsTime().UnixNano()
	}
	return time.Now().UnixNano()
}

func (p *ReactionProjection) HasReaction(messageEventID, emoji, userID string) bool {
	p.RLock()
	defer p.RUnlock()

	byEmoji := p.byMessage[messageEventID]
	if byEmoji == nil {
		return false
	}
	byUser := byEmoji[emoji]
	if byUser == nil {
		return false
	}
	_, ok := byUser[userID]
	return ok
}

func (p *ReactionProjection) Reactions(messageEventID string) []ReactionSummary {
	p.RLock()
	defer p.RUnlock()
	return reactionSummariesForMessage(p.byMessage[messageEventID])
}

func (p *ReactionProjection) ReactionsBatch(messageEventIDs []string) map[string][]ReactionSummary {
	p.RLock()
	defer p.RUnlock()

	result := make(map[string][]ReactionSummary, len(messageEventIDs))
	for _, eventID := range messageEventIDs {
		if byEmoji := p.byMessage[eventID]; byEmoji != nil {
			result[eventID] = reactionSummariesForMessage(byEmoji)
		}
	}
	return result
}

// Stats returns aggregate counts useful for import/rollout diagnostics.
func (p *ReactionProjection) Stats() (messages int, activeReactions int) {
	p.RLock()
	defer p.RUnlock()
	messages = len(p.byMessage)
	for _, byEmoji := range p.byMessage {
		for _, byUser := range byEmoji {
			activeReactions += len(byUser)
		}
	}
	return messages, activeReactions
}

func reactionSummariesForMessage(byEmoji map[string]map[string]int64) []ReactionSummary {
	if len(byEmoji) == 0 {
		return nil
	}
	type group struct {
		summary       ReactionSummary
		earliestNanos int64
	}
	groups := make([]group, 0, len(byEmoji))
	for emoji, byUser := range byEmoji {
		userIDs := make([]string, 0, len(byUser))
		var earliest int64
		for userID, nanos := range byUser {
			userIDs = append(userIDs, userID)
			if earliest == 0 || nanos < earliest {
				earliest = nanos
			}
		}
		slices.Sort(userIDs)
		groups = append(groups, group{
			summary:       ReactionSummary{Emoji: emoji, UserIDs: userIDs},
			earliestNanos: earliest,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].earliestNanos != groups[j].earliestNanos {
			return groups[i].earliestNanos < groups[j].earliestNanos
		}
		return groups[i].summary.Emoji < groups[j].summary.Emoji
	})
	result := make([]ReactionSummary, len(groups))
	for i, g := range groups {
		result[i] = g.summary
	}
	return result
}
