package core

import (
	"context"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomModel owns the room-derived projections and their projectors.
//
// ChattoCore is still the compatibility facade for most room APIs, but room
// write paths should use this type for projection readiness instead of naming
// individual projector fields. That keeps the "which projections must catch
// up?" knowledge with the room read models.
type RoomModel struct {
	directory          *RoomDirectoryProjection
	directoryProjector *events.Projector

	groupLayout          *RoomGroupLayoutProjection
	groupLayoutProjector *events.Projector

	timeline          *RoomTimelineProjection
	timelineProjector *events.Projector

	threads          *ThreadProjection
	threadsProjector *events.Projector

	reactions          *ReactionProjection
	reactionsProjector *events.Projector
}

func newRoomModel(
	directory *RoomDirectoryProjection,
	directoryProjector *events.Projector,
	groupLayout *RoomGroupLayoutProjection,
	groupLayoutProjector *events.Projector,
	timeline *RoomTimelineProjection,
	timelineProjector *events.Projector,
	threads *ThreadProjection,
	threadsProjector *events.Projector,
	reactions *ReactionProjection,
	reactionsProjector *events.Projector,
) *RoomModel {
	return &RoomModel{
		directory:            directory,
		directoryProjector:   directoryProjector,
		groupLayout:          groupLayout,
		groupLayoutProjector: groupLayoutProjector,
		timeline:             timeline,
		timelineProjector:    timelineProjector,
		threads:              threads,
		threadsProjector:     threadsProjector,
		reactions:            reactions,
		reactionsProjector:   reactionsProjector,
	}
}

func (c *ChattoCore) rooms() *RoomModel {
	if c.roomModel == nil {
		c.roomModel = newRoomModel(
			c.RoomDirectory,
			c.RoomDirectoryProjector,
			c.RoomGroupLayout,
			c.RoomGroupLayoutProjector,
			c.RoomTimeline,
			c.RoomTimelineProjector,
			c.Threads,
			c.ThreadsProjector,
			c.Reactions,
			c.ReactionsProjector,
		)
	}
	return c.roomModel
}

func (m *RoomModel) waitForDirectory(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("room directory", m.directoryProjector))
}

func (m *RoomModel) waitForGroupLayout(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("room group layout", m.groupLayoutProjector))
}

func (m *RoomModel) waitForGroupLayoutCurrent(ctx context.Context, publisher *events.Publisher) error {
	pos, err := publisher.LastSubjectPosition(ctx, events.GroupSubjectFilter())
	if err != nil {
		return err
	}
	if pos.IsZero() {
		return nil
	}
	return m.waitForGroupLayout(ctx, pos)
}

func (m *RoomModel) waitForTimeline(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("room timeline", m.timelineProjector))
}

func (m *RoomModel) waitForThreads(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("threads", m.threadsProjector))
}

func (m *RoomModel) waitForReactions(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("reactions", m.reactionsProjector))
}

func (m *RoomModel) waitForReactionsCurrent(ctx context.Context, publisher *events.Publisher, roomID string) error {
	pos, err := publisher.LastSubjectPosition(ctx, events.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		return err
	}
	if pos.IsZero() {
		return nil
	}
	return m.waitForReactions(ctx, pos)
}

func (m *RoomModel) waitForDirectoryAndTimeline(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos,
		waitForProjection("room directory", m.directoryProjector),
		waitForProjection("room timeline", m.timelineProjector),
	)
}

func (m *RoomModel) waitForTimelineAndThreads(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos,
		waitForProjection("room timeline", m.timelineProjector),
		waitForProjection("threads", m.threadsProjector),
	)
}

func (m *RoomModel) waitForLiveEVTEvent(ctx context.Context, pos events.StreamPosition, event *corev1.Event) error {
	if err := m.waitForTimeline(ctx, pos); err != nil {
		return err
	}
	if eventNeedsThreadProjection(event) {
		if err := m.waitForThreads(ctx, pos); err != nil {
			return err
		}
	}
	if eventNeedsReactionProjection(event) {
		if err := m.waitForReactions(ctx, pos); err != nil {
			return err
		}
	}
	if eventNeedsRoomDirectoryProjection(event) {
		if err := m.waitForDirectory(ctx, pos); err != nil {
			return err
		}
	}
	return nil
}

func (m *RoomModel) room(roomID string) (*corev1.Room, bool) {
	return m.directory.Catalog.Get(roomID)
}

func (m *RoomModel) roomsByKind(kind corev1.RoomKind) []*corev1.Room {
	return m.directory.Catalog.AllByKind(kind)
}

func (m *RoomModel) roomIDByName(name string) string {
	return m.directory.Catalog.FindByName(name)
}

func (m *RoomModel) nameClaimSnapshot(name string) RoomNameClaimSnapshot {
	return m.directory.Catalog.NameClaimSnapshot(name)
}

func (m *RoomModel) waitForDirectoryCurrent(ctx context.Context, publisher *events.Publisher) error {
	pos, err := publisher.LastSubjectPosition(ctx, events.RoomSubjectFilter())
	if err != nil {
		return err
	}
	if pos.IsZero() {
		return nil
	}
	return m.waitForDirectory(ctx, pos)
}

func (m *RoomModel) activeRoomBan(roomID, userID string, now time.Time) (RoomBan, bool) {
	return m.directory.Bans.ActiveBan(roomID, userID, now)
}

func (m *RoomModel) activeRoomBans(roomID string, now time.Time) []RoomBan {
	return m.directory.Bans.ActiveRoomBans(roomID, now)
}

func (m *RoomModel) activeBans(now time.Time) []RoomBan {
	return m.directory.Bans.ActiveBans(now)
}

func (m *RoomModel) isRoomBanActive(roomID, userID string, now time.Time) bool {
	return m.directory.Bans.IsActive(roomID, userID, now)
}

func (m *RoomModel) timelineEntry(eventID string) (*TimelineEntry, bool) {
	return m.timeline.Get(eventID)
}

func (m *RoomModel) latestBody(eventID string) (*corev1.MessageBody, bool, bool) {
	return m.timeline.LatestBody(eventID)
}

func (m *RoomModel) currentRoomAttachmentMessages(roomID string) []projectedRoomAttachmentMessage {
	return m.timeline.CurrentRoomAttachmentMessages(roomID)
}

func (m *RoomModel) isEcho(eventID string) bool {
	return m.timeline.IsEcho(eventID)
}

func (m *RoomModel) isHiddenEcho(eventID string) bool {
	return m.timeline.IsHiddenEcho(eventID)
}

func (m *RoomModel) linkedEventIDs(eventID string) []string {
	return m.timeline.LinkedEventIDs(eventID)
}

func (m *RoomModel) bodyEventSeqs(eventID string) ([]uint64, uint64, bool) {
	return m.timeline.BodyEventSeqs(eventID)
}

func (m *RoomModel) obsoleteBodyEventSeqs(eventID string) []uint64 {
	return m.timeline.ObsoleteBodyEventSeqs(eventID)
}

func (m *RoomModel) allObsoleteBodyEventSeqs() []uint64 {
	return m.timeline.AllObsoleteBodyEventSeqs()
}

func (m *RoomModel) messageTombstoned(eventID string) bool {
	return m.timeline.MessageTombstoned(eventID)
}

func (m *RoomModel) lastVisibleRoomEntry(roomID string, visible func(*corev1.Event) bool) (*TimelineEntry, bool) {
	return m.timeline.LastVisibleRoomEntry(roomID, visible)
}

func (m *RoomModel) lastRoomMessageEntry(roomID string) (*TimelineEntry, bool) {
	return m.timeline.LastRoomMessageEntry(roomID)
}

func (m *RoomModel) visibleRoomTimeline(roomID string, limit int, beforeStreamSeq uint64, visible func(*corev1.Event) bool) []*TimelineEntry {
	return m.timeline.VisibleRoomTimeline(roomID, limit, beforeStreamSeq, visible)
}

func (m *RoomModel) roomEventCount(roomID string) int {
	return m.timeline.RoomEventCount(roomID)
}

func (m *RoomModel) visibleRoomTimelineAfter(roomID string, limit int, afterStreamSeq uint64, visible func(*corev1.Event) bool) []*TimelineEntry {
	return m.timeline.VisibleRoomTimelineAfter(roomID, limit, afterStreamSeq, visible)
}

func (m *RoomModel) visibleRoomTimelineAround(roomID, eventID string, limit int) ([]*TimelineEntry, int, bool, bool, bool) {
	return m.timeline.VisibleRoomTimelineAround(roomID, eventID, limit)
}

func (m *RoomModel) threadExists(rootEventID string) bool {
	return m.threads.ThreadExists(rootEventID)
}

func (m *RoomModel) threadEvents(rootEventID string) []*TimelineEntry {
	return m.threads.ThreadEvents(rootEventID)
}

func (m *RoomModel) threadMetadata(rootEventID string) *ThreadMetadata {
	return m.threads.ThreadMetadata(rootEventID)
}

func (m *RoomModel) threadFollowState(userID, roomID, threadRootEventID string) ThreadFollowState {
	return m.threads.FollowState(userID, roomID, threadRootEventID)
}

func (m *RoomModel) threadFollowers(roomID, threadRootEventID string) []string {
	return m.threads.ThreadFollowers(roomID, threadRootEventID)
}

func (m *RoomModel) followedThreadsForUser(userID string) []threadFollowRef {
	return m.threads.FollowedThreadsForUser(userID)
}

func (m *RoomModel) reactionsForMessage(messageEventID string) []ReactionSummary {
	return m.reactions.Reactions(messageEventID)
}

func (m *RoomModel) reactionsBatch(eventIDs []string) map[string][]ReactionSummary {
	return m.reactions.ReactionsBatch(eventIDs)
}

func (m *RoomModel) hasReaction(messageEventID, emoji, userID string) bool {
	return m.reactions.HasReaction(messageEventID, emoji, userID)
}

func (m *RoomModel) reactionMutationSnapshot(roomID, messageEventID, emoji, userID string) ReactionMutationSnapshot {
	return m.reactions.ReactionMutationSnapshot(roomID, messageEventID, emoji, userID)
}

func (m *RoomModel) appendDirectoryEventually(ctx context.Context, pub *events.Publisher, agg events.Aggregate, event *corev1.Event) (events.StreamPosition, error) {
	subject := agg.SubjectFor(event)
	seq, err := pub.AppendEventually(ctx, subject, event)
	if err != nil {
		return events.StreamPosition{}, err
	}
	pos := events.SubjectPosition(subject, seq)
	if err := m.waitForDirectory(ctx, pos); err != nil {
		return pos, err
	}
	return pos, nil
}

func (m *RoomModel) appendGroupLayout(ctx context.Context, pub *events.Publisher, agg events.Aggregate, event *corev1.Event) (events.StreamPosition, error) {
	subject := agg.SubjectFor(event)
	seq, err := pub.Append(ctx, subject, event)
	if err != nil {
		return events.StreamPosition{}, err
	}
	pos := events.SubjectPosition(subject, seq)
	if err := m.waitForGroupLayout(ctx, pos); err != nil {
		return pos, err
	}
	return pos, nil
}

func (m *RoomModel) appendGroupLayoutEventually(ctx context.Context, pub *events.Publisher, agg events.Aggregate, event *corev1.Event) (events.StreamPosition, error) {
	subject := agg.SubjectFor(event)
	seq, err := pub.AppendEventually(ctx, subject, event)
	if err != nil {
		return events.StreamPosition{}, err
	}
	pos := events.SubjectPosition(subject, seq)
	if err := m.waitForGroupLayout(ctx, pos); err != nil {
		return pos, err
	}
	return pos, nil
}

func (m *RoomModel) appendTimelineEventually(ctx context.Context, pub *events.Publisher, agg events.Aggregate, event *corev1.Event) (events.StreamPosition, error) {
	subject := agg.SubjectFor(event)
	seq, err := pub.AppendEventually(ctx, subject, event)
	if err != nil {
		return events.StreamPosition{}, err
	}
	pos := events.SubjectPosition(subject, seq)
	if err := m.waitForTimeline(ctx, pos); err != nil {
		return pos, err
	}
	return pos, nil
}
