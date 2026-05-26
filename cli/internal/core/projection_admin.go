package core

import (
	"context"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
)

const (
	projectionMapEntryOverhead   int64 = 64
	projectionSliceEntryOverhead int64 = 24
)

// ProjectionAdminState is the operator-facing runtime state for one
// event-sourced projection.
type ProjectionAdminState struct {
	Name              string
	Subjects          []string
	Started           bool
	LastAppliedSeq    uint64
	MatchingStreamSeq uint64
	StreamLastSeq     uint64
	Lag               uint64
	EntryCount        int64
	EstimatedBytes    int64
	AverageEntryBytes int64
	Metrics           []ProjectionAdminMetric
}

type ProjectionAdminMetric struct {
	Name  string
	Value int64
	Bytes int64
}

// ProjectionAdminStates returns read-only projection diagnostics for the
// server-admin UI. It is intentionally on-demand; the byte counts walk
// in-memory projection state and are meant for operator pages, not hot paths.
func (c *ChattoCore) ProjectionAdminStates(ctx context.Context) ([]ProjectionAdminState, error) {
	info, err := c.storage.serverEvtStream.Info(ctx)
	if err != nil {
		return nil, err
	}
	streamLastSeq := info.State.LastSeq

	states := make([]ProjectionAdminState, 0, 8)
	add := func(name string, projector *events.Projector, entries int64, estimatedBytes int64, metrics []ProjectionAdminMetric) error {
		targetSeq, err := projector.CurrentTargetSeq(ctx)
		if err != nil {
			return err
		}
		lastApplied := projector.LastSeq()
		var lag uint64
		if targetSeq > lastApplied {
			lag = targetSeq - lastApplied
		}
		var avg int64
		if entries > 0 {
			avg = estimatedBytes / entries
		}
		states = append(states, ProjectionAdminState{
			Name:              name,
			Subjects:          projector.Subjects(),
			Started:           projector.Started(),
			LastAppliedSeq:    lastApplied,
			MatchingStreamSeq: targetSeq,
			StreamLastSeq:     streamLastSeq,
			Lag:               lag,
			EntryCount:        entries,
			EstimatedBytes:    estimatedBytes,
			AverageEntryBytes: avg,
			Metrics:           metrics,
		})
		return nil
	}

	for _, collect := range []func() error{
		func() error {
			entries, bytes, metrics := c.RoomCatalog.adminProjectionEstimate()
			return add("Room Catalog", c.RoomCatalogProjector, entries, bytes, metrics)
		},
		func() error {
			entries, bytes, metrics := c.RoomMembership.adminProjectionEstimate()
			return add("Room Membership", c.RoomMembershipProjector, entries, bytes, metrics)
		},
		func() error {
			entries, bytes, metrics := c.ServerConfig.adminProjectionEstimate()
			return add("Server Config", c.ServerConfigProjector, entries, bytes, metrics)
		},
		func() error {
			entries, bytes, metrics := c.RoomGroups.adminProjectionEstimate()
			return add("Room Groups", c.RoomGroupsProjector, entries, bytes, metrics)
		},
		func() error {
			entries, bytes, metrics := c.RoomLayout.adminProjectionEstimate()
			return add("Room Layout", c.RoomLayoutProjector, entries, bytes, metrics)
		},
		func() error {
			entries, bytes, metrics := c.RoomTimeline.adminProjectionEstimate()
			return add("Room Timeline", c.RoomTimelineProjector, entries, bytes, metrics)
		},
		func() error {
			entries, bytes, metrics := c.Threads.adminProjectionEstimate()
			return add("Threads", c.ThreadsProjector, entries, bytes, metrics)
		},
		func() error {
			entries, bytes, metrics := c.Reactions.adminProjectionEstimate()
			return add("Reactions", c.ReactionsProjector, entries, bytes, metrics)
		},
	} {
		if err := collect(); err != nil {
			return nil, err
		}
	}
	return states, nil
}

func (p *RoomCatalogProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var bytes int64
	var archived int64
	for id, room := range p.rooms {
		bytes += projectionMapEntryOverhead + int64(len(id)+len(room.name)+len(room.description)) + 8
		if room.archived {
			archived++
		}
	}
	return int64(len(p.rooms)), bytes, []ProjectionAdminMetric{
		{Name: "rooms", Value: int64(len(p.rooms)), Bytes: bytes},
		{Name: "archived_rooms", Value: archived, Bytes: 0},
	}
}

func (p *RoomMembershipProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var memberships, bytes int64
	for roomID, users := range p.byRoom {
		bytes += projectionMapEntryOverhead + int64(len(roomID))
		for userID := range users {
			memberships++
			bytes += projectionMapEntryOverhead + int64(len(userID))
		}
	}
	var userRooms int64
	for userID, rooms := range p.byUser {
		bytes += projectionMapEntryOverhead + int64(len(userID))
		for roomID := range rooms {
			userRooms++
			bytes += projectionMapEntryOverhead + int64(len(roomID))
		}
	}
	return memberships, bytes, []ProjectionAdminMetric{
		{Name: "rooms", Value: int64(len(p.byRoom)), Bytes: 0},
		{Name: "memberships_by_room", Value: memberships, Bytes: bytes / 2},
		{Name: "memberships_by_user", Value: userRooms, Bytes: bytes / 2},
	}
}

func (p *ServerConfigProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	if !p.seen {
		return 0, 0, []ProjectionAdminMetric{{Name: "configured", Value: 0}}
	}
	var bytes int64
	if p.cfg != nil {
		bytes = int64(proto.Size(p.cfg)) + projectionMapEntryOverhead
	}
	return 1, bytes, []ProjectionAdminMetric{{Name: "configured", Value: 1, Bytes: bytes}}
}

func (p *RoomGroupProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var bytes, roomRefs int64
	for id, group := range p.groups {
		groupBytes := projectionMapEntryOverhead + int64(len(id)+len(group.name)+len(group.description))
		for _, roomID := range group.roomIDs {
			roomRefs++
			groupBytes += projectionSliceEntryOverhead + int64(len(roomID))
		}
		bytes += groupBytes
	}
	return int64(len(p.groups)), bytes, []ProjectionAdminMetric{
		{Name: "groups", Value: int64(len(p.groups)), Bytes: bytes},
		{Name: "room_references", Value: roomRefs, Bytes: 0},
	}
}

func (p *RoomLayoutProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var bytes int64
	for _, groupID := range p.groupIDs {
		bytes += projectionSliceEntryOverhead + int64(len(groupID))
	}
	return int64(len(p.groupIDs)), bytes, []ProjectionAdminMetric{
		{Name: "ordered_groups", Value: int64(len(p.groupIDs)), Bytes: bytes},
	}
}

func (p *RoomTimelineProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var entries, rawBytes, messagePosts int64
	for _, roomEntries := range p.byRoom {
		var roomBytes int64
		for _, entry := range roomEntries {
			entries++
			eventBytes := timelineEntryEstimatedBytes(entry)
			roomBytes += eventBytes
			if entry != nil && entry.Event.GetMessagePosted() != nil {
				messagePosts++
			}
		}
		rawBytes += roomBytes
	}

	var eventIndexBytes int64
	for eventID := range p.byEventID {
		eventIndexBytes += projectionMapEntryOverhead + int64(len(eventID))
	}
	var latestBodyBytes int64
	for eventID, body := range p.latestBody {
		latestBodyBytes += projectionMapEntryOverhead + int64(len(eventID))
		if body != nil {
			latestBodyBytes += int64(proto.Size(body))
		}
	}
	var retractedBytes int64
	for eventID := range p.retractedFlags {
		retractedBytes += projectionMapEntryOverhead + int64(len(eventID))
	}
	var echoBytes, echoLinks int64
	for eventID, echoes := range p.echoLinks {
		echoBytes += projectionMapEntryOverhead + int64(len(eventID))
		for _, echoID := range echoes {
			echoLinks++
			echoBytes += projectionSliceEntryOverhead + int64(len(echoID))
		}
	}

	totalBytes := rawBytes + eventIndexBytes + latestBodyBytes + retractedBytes + echoBytes
	return entries, totalBytes, []ProjectionAdminMetric{
		{Name: "rooms", Value: int64(len(p.byRoom)), Bytes: 0},
		{Name: "timeline_entries", Value: entries, Bytes: rawBytes},
		{Name: "message_posts", Value: messagePosts, Bytes: 0},
		{Name: "event_id_index", Value: int64(len(p.byEventID)), Bytes: eventIndexBytes},
		{Name: "latest_body_index", Value: int64(len(p.latestBody)), Bytes: latestBodyBytes},
		{Name: "retracted_flags", Value: int64(len(p.retractedFlags)), Bytes: retractedBytes},
		{Name: "echo_links", Value: echoLinks, Bytes: echoBytes},
	}
}

func (p *ThreadProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var entries, rawBytes, replies int64
	for _, threadEntries := range p.byThread {
		var threadBytes int64
		for _, entry := range threadEntries {
			entries++
			eventBytes := timelineEntryEstimatedBytes(entry)
			threadBytes += eventBytes
			if entry != nil && entry.Event.GetMessagePosted() != nil {
				replies++
			}
		}
		rawBytes += threadBytes
	}
	var indexBytes int64
	for eventID, threadID := range p.messageToThread {
		indexBytes += projectionMapEntryOverhead + int64(len(eventID)+len(threadID))
	}
	totalBytes := rawBytes + indexBytes
	return entries, totalBytes, []ProjectionAdminMetric{
		{Name: "threads", Value: int64(len(p.byThread)), Bytes: 0},
		{Name: "thread_entries", Value: entries, Bytes: rawBytes},
		{Name: "replies", Value: replies, Bytes: 0},
		{Name: "message_to_thread_index", Value: int64(len(p.messageToThread)), Bytes: indexBytes},
	}
}

func (p *ReactionProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var active, emojiGroups, bytes int64
	for messageID, byEmoji := range p.byMessage {
		messageBytes := projectionMapEntryOverhead + int64(len(messageID))
		for emoji, byUser := range byEmoji {
			emojiGroups++
			messageBytes += projectionMapEntryOverhead + int64(len(emoji))
			for userID := range byUser {
				active++
				messageBytes += projectionMapEntryOverhead + int64(len(userID)) + 8
			}
		}
		bytes += messageBytes
	}
	seenBytes := int64(len(p.seen)) * projectionMapEntryOverhead
	bytes += seenBytes
	return active, bytes, []ProjectionAdminMetric{
		{Name: "messages", Value: int64(len(p.byMessage)), Bytes: 0},
		{Name: "emoji_groups", Value: emojiGroups, Bytes: 0},
		{Name: "active_reactions", Value: active, Bytes: bytes - seenBytes},
		{Name: "seen_event_ids", Value: int64(len(p.seen)), Bytes: seenBytes},
	}
}

func timelineEntryEstimatedBytes(entry *TimelineEntry) int64 {
	if entry == nil || entry.Event == nil {
		return projectionSliceEntryOverhead
	}
	return projectionSliceEntryOverhead + int64(proto.Size(entry.Event)) + 8
}
