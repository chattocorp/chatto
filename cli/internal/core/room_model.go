package core

import (
	"context"
	"time"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

// RoomModel owns the room-derived projections and their projectors.
//
// ChattoCore is still the compatibility facade for most room APIs, but room
// write paths should use this type for projection readiness instead of naming
// individual projector fields. That keeps the "which projections must catch
// up?" knowledge with the room read models.
type RoomModel struct {
	directory   events.ProjectionHandle[*RoomDirectoryProjection]
	groupLayout events.ProjectionHandle[*RoomGroupLayoutProjection]
	timeline    events.ProjectionHandle[*RoomTimelineProjection]
	threads     events.ProjectionHandle[*ThreadProjection]
	reactions   events.ProjectionHandle[*ReactionProjection]
}

func newRoomModel(
	directory events.ProjectionHandle[*RoomDirectoryProjection],
	groupLayout events.ProjectionHandle[*RoomGroupLayoutProjection],
	timeline events.ProjectionHandle[*RoomTimelineProjection],
	threads events.ProjectionHandle[*ThreadProjection],
	reactions events.ProjectionHandle[*ReactionProjection],
) *RoomModel {
	return &RoomModel{
		directory:   directory,
		groupLayout: groupLayout,
		timeline:    timeline,
		threads:     threads,
		reactions:   reactions,
	}
}

func (m *RoomModel) waitForDirectory(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("room directory", m.directory.Projector()))
}

func (m *RoomModel) waitForGroupLayout(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("room group layout", m.groupLayout.Projector()))
}

func (m *RoomModel) waitForGroupLayoutCurrent(ctx context.Context, publisher *evtstream.Publisher) error {
	pos, err := publisher.LastSubjectPosition(ctx, evtstream.GroupSubjectFilter())
	if err != nil {
		return err
	}
	if pos.IsZero() {
		return nil
	}
	return m.waitForGroupLayout(ctx, pos)
}

func (m *RoomModel) waitForTimeline(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("room timeline", m.timeline.Projector()))
}

func (m *RoomModel) waitForThreads(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("threads", m.threads.Projector()))
}

func (m *RoomModel) waitForReactions(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("reactions", m.reactions.Projector()))
}

func (m *RoomModel) waitForReactionsCurrent(ctx context.Context, publisher *evtstream.Publisher, roomID string) error {
	pos, err := publisher.LastSubjectPosition(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
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
		waitForProjection("room directory", m.directory.Projector()),
		waitForProjection("room timeline", m.timeline.Projector()),
	)
}

func (m *RoomModel) waitForTimelineAndThreads(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos,
		waitForProjection("room timeline", m.timeline.Projector()),
		waitForProjection("threads", m.threads.Projector()),
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
	return m.directory.Projection().Catalog.Get(roomID)
}

func (m *RoomModel) roomsByKind(kind corev1.RoomKind) []*corev1.Room {
	return m.directory.Projection().Catalog.AllByKind(kind)
}

func (m *RoomModel) roomIDByName(name string) string {
	return m.directory.Projection().Catalog.FindByName(name)
}

func (m *RoomModel) nameClaimSnapshot(name, excludeRoomID string) RoomNameClaimSnapshot {
	return m.directory.Projection().Catalog.NameClaimSnapshot(name, excludeRoomID)
}

func (m *RoomModel) hasExplicitRoomMembership(roomID, userID string) bool {
	return m.directory.Projection().Membership.IsMember(roomID, userID)
}

func (m *RoomModel) explicitRoomIDsForUser(userID string) []string {
	return m.directory.Projection().Membership.Rooms(userID)
}

func (m *RoomModel) explicitRoomMemberIDs(roomID string) []string {
	return m.directory.Projection().Membership.Members(roomID)
}

func (m *RoomModel) roomGroup(groupID string) (*corev1.RoomGroup, bool) {
	return m.groupLayout.Projection().Groups.Get(groupID)
}

func (m *RoomModel) roomGroupSnapshot(groupID string) RoomGroupSnapshot {
	return m.groupLayout.Projection().Groups.Snapshot(groupID)
}

func (m *RoomModel) roomGroups() []*corev1.RoomGroup {
	return m.groupLayout.Projection().Groups.All()
}

func (m *RoomModel) roomGroupForRoom(roomID string) string {
	return m.groupLayout.Projection().Groups.GroupForRoom(roomID)
}

func (m *RoomModel) roomGroupForSidebarLink(linkID string) string {
	return m.groupLayout.Projection().Groups.GroupForSidebarLink(linkID)
}

func (m *RoomModel) roomGroupMoveSnapshot(roomID, targetGroupID string) RoomGroupMoveSnapshot {
	return m.groupLayout.Projection().Groups.MoveSnapshot(roomID, targetGroupID)
}

func (m *RoomModel) sidebarLinkMoveSnapshot(linkID, targetGroupID string) SidebarLinkMoveSnapshot {
	return m.groupLayout.Projection().Groups.SidebarLinkMoveSnapshot(linkID, targetGroupID)
}

func (m *RoomModel) roomLayoutOrder() []string {
	return m.groupLayout.Projection().Layout.Order()
}

func (m *RoomModel) waitForDirectoryCurrent(ctx context.Context, publisher *evtstream.Publisher) error {
	pos, err := publisher.LastSubjectPosition(ctx, evtstream.RoomSubjectFilter())
	if err != nil {
		return err
	}
	if pos.IsZero() {
		return nil
	}
	return m.waitForDirectory(ctx, pos)
}

func (m *RoomModel) activeRoomBan(roomID, userID string, now time.Time) (RoomBan, bool) {
	return m.directory.Projection().Bans.ActiveBan(roomID, userID, now)
}

func (m *RoomModel) activeRoomBans(roomID string, now time.Time) []RoomBan {
	return m.directory.Projection().Bans.ActiveRoomBans(roomID, now)
}

func (m *RoomModel) activeBans(now time.Time) []RoomBan {
	return m.directory.Projection().Bans.ActiveBans(now)
}

func (m *RoomModel) isRoomBanActive(roomID, userID string, now time.Time) bool {
	return m.directory.Projection().Bans.IsActive(roomID, userID, now)
}

func (m *RoomModel) hasTimeline() bool {
	return m != nil && m.timeline.Projection() != nil
}

func (m *RoomModel) timelineEntry(eventID string) (*TimelineEntry, bool) {
	return m.timeline.Projection().Get(eventID)
}

func (m *RoomModel) latestBody(eventID string) (*corev1.MessageBody, bool, bool) {
	return m.timeline.Projection().LatestBody(eventID)
}

func (m *RoomModel) currentRoomAttachmentMessages(roomID string) []projectedRoomAttachmentMessage {
	return m.timeline.Projection().CurrentRoomAttachmentMessages(roomID)
}

func (m *RoomModel) isEcho(eventID string) bool {
	return m.timeline.Projection().IsEcho(eventID)
}

func (m *RoomModel) isHiddenEcho(eventID string) bool {
	return m.timeline.Projection().IsHiddenEcho(eventID)
}

func (m *RoomModel) channelEchoEventID(eventID string) (string, bool) {
	return m.timeline.Projection().ChannelEchoEventID(eventID)
}

func (m *RoomModel) linkedChannelEchoEventID(eventID string) (string, bool) {
	return m.timeline.Projection().LinkedChannelEchoEventID(eventID)
}

func (m *RoomModel) messageHydrationState(eventID string) RoomTimelineMessageHydrationState {
	return m.timeline.Projection().MessageHydrationState(eventID)
}

func (m *RoomModel) linkedEventIDs(eventID string) []string {
	return m.timeline.Projection().LinkedEventIDs(eventID)
}

func (m *RoomModel) bodyEventSeqs(eventID string) ([]uint64, uint64, bool) {
	return m.timeline.Projection().BodyEventSeqs(eventID)
}

func (m *RoomModel) obsoleteBodyEventSeqs(eventID string) []uint64 {
	return m.timeline.Projection().ObsoleteBodyEventSeqs(eventID)
}

func (m *RoomModel) allObsoleteBodyEventSeqs() []uint64 {
	return m.timeline.Projection().AllObsoleteBodyEventSeqs()
}

func (m *RoomModel) messageTombstoned(eventID string) bool {
	return m.timeline.Projection().MessageTombstoned(eventID)
}

func (m *RoomModel) lastVisibleRoomEntry(roomID string, visible func(*corev1.Event) bool) (*TimelineEntry, bool) {
	return m.timeline.Projection().LastVisibleRoomEntry(roomID, visible)
}

func (m *RoomModel) lastRoomMessageEntry(roomID string) (*TimelineEntry, bool) {
	return m.timeline.Projection().LastRoomMessageEntry(roomID)
}

func (m *RoomModel) latestOriginalPostAt(roomID, actorID string) (time.Time, bool) {
	return m.timeline.Projection().LatestOriginalPostAt(roomID, actorID)
}

func (m *RoomModel) visibleRoomTimeline(roomID string, limit int, beforeStreamSeq uint64, visible func(*corev1.Event) bool) []*TimelineEntry {
	return m.timeline.Projection().VisibleRoomTimeline(roomID, limit, beforeStreamSeq, visible)
}

func (m *RoomModel) roomEventCount(roomID string) int {
	return m.timeline.Projection().RoomEventCount(roomID)
}

func (m *RoomModel) visibleRoomTimelineAfter(roomID string, limit int, afterStreamSeq uint64, visible func(*corev1.Event) bool) []*TimelineEntry {
	return m.timeline.Projection().VisibleRoomTimelineAfter(roomID, limit, afterStreamSeq, visible)
}

func (m *RoomModel) visibleRoomTimelineAround(roomID, eventID string, limit int) ([]*TimelineEntry, int, bool, bool, bool) {
	return m.timeline.Projection().VisibleRoomTimelineAround(roomID, eventID, limit)
}

func (m *RoomModel) threadExists(rootEventID string) bool {
	return m.threads.Projection().ThreadExists(rootEventID)
}

func (m *RoomModel) threadEvents(rootEventID string) []*TimelineEntry {
	refs := m.threads.Projection().ThreadEvents(rootEventID)
	if len(refs) == 0 {
		return nil
	}
	out := make([]*TimelineEntry, 0, len(refs))
	for _, ref := range refs {
		entry, ok := m.timeline.Projection().Get(ref.EventID)
		if !ok || entry == nil {
			continue
		}
		if ref.StreamSeq != 0 && entry.StreamSeq != ref.StreamSeq {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func (m *RoomModel) threadMetadata(rootEventID string) *ThreadMetadata {
	return m.threads.Projection().ThreadMetadata(rootEventID)
}

func (m *RoomModel) threadFollowState(userID, roomID, threadRootEventID string) ThreadFollowState {
	return m.threads.Projection().FollowState(userID, roomID, threadRootEventID)
}

func (m *RoomModel) threadFollowers(roomID, threadRootEventID string) []string {
	return m.threads.Projection().ThreadFollowers(roomID, threadRootEventID)
}

func (m *RoomModel) followedThreadsForUser(userID string) []threadFollowRef {
	return m.threads.Projection().FollowedThreadsForUser(userID)
}

func (m *RoomModel) reactionsForMessage(messageEventID string) []ReactionSummary {
	return m.reactions.Projection().Reactions(messageEventID)
}

func (m *RoomModel) reactionsBatch(eventIDs []string) map[string][]ReactionSummary {
	return m.reactions.Projection().ReactionsBatch(eventIDs)
}

func (m *RoomModel) hasReaction(messageEventID, emoji, userID string) bool {
	return m.reactions.Projection().HasReaction(messageEventID, emoji, userID)
}

func (m *RoomModel) reactionMutationSnapshot(roomID, messageEventID, emoji, userID string) ReactionMutationSnapshot {
	return m.reactions.Projection().ReactionMutationSnapshot(roomID, messageEventID, emoji, userID)
}

func (m *RoomModel) appendDirectoryEventually(ctx context.Context, pub *evtstream.Publisher, agg evtstream.Aggregate, event *corev1.Event) (events.StreamPosition, error) {
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

func (m *RoomModel) appendGroupLayout(ctx context.Context, pub *evtstream.Publisher, agg evtstream.Aggregate, event *corev1.Event) (events.StreamPosition, error) {
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

func (m *RoomModel) appendGroupLayoutEventually(ctx context.Context, pub *evtstream.Publisher, agg evtstream.Aggregate, event *corev1.Event) (events.StreamPosition, error) {
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

func (m *RoomModel) appendTimelineEventually(ctx context.Context, pub *evtstream.Publisher, agg evtstream.Aggregate, event *corev1.Event) (events.StreamPosition, error) {
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
