package core

import (
	"context"
	"unsafe"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/pkg/events"
)

const (
	projectionMapEntryOverhead   int64 = 64
	projectionSliceEntryOverhead int64 = 24
	projectionIntIndexBytes      int64 = 8
	// projectionCompactMapEntryOverhead approximates the per-entry cost of a
	// map with small pointer-free keys, excluding the key itself: control
	// bytes, load-factor slack, and value padding.
	projectionCompactMapEntryOverhead int64 = 16
)

// ProjectionAdminState is the operator-facing runtime state for one
// event-sourced projection.
type ProjectionAdminState struct {
	Key               string
	Name              string
	Subjects          []string
	Started           bool
	StartupComplete   bool
	StartupDuration   float64
	StartupMessages   uint64
	LastAppliedSeq    uint64
	MatchingStreamSeq uint64
	StreamLastSeq     uint64
	Lag               uint64
	Failed            bool
	FailedSeq         uint64
	Failure           string
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

	states := make([]ProjectionAdminState, 0, len(c.projections))
	add := func(key string, name string, projector *events.Projector, entries int64, estimatedBytes int64, metrics []ProjectionAdminMetric) error {
		targetSeq, err := projector.CurrentTargetSeq(ctx)
		if err != nil {
			return err
		}
		status := projector.Status()
		lastApplied := status.LastSeq
		var lag uint64
		if targetSeq > lastApplied {
			lag = targetSeq - lastApplied
		}
		var avg int64
		if entries > 0 {
			avg = estimatedBytes / entries
		}
		states = append(states, ProjectionAdminState{
			Key:               key,
			Name:              name,
			Subjects:          projector.Subjects(),
			Started:           status.Started,
			StartupComplete:   status.StartupComplete,
			StartupDuration:   status.StartupDuration.Seconds(),
			StartupMessages:   status.StartupMessages,
			LastAppliedSeq:    lastApplied,
			MatchingStreamSeq: targetSeq,
			StreamLastSeq:     streamLastSeq,
			Lag:               lag,
			Failed:            status.Failed,
			FailedSeq:         status.FailedSeq,
			Failure:           status.Failure,
			EntryCount:        entries,
			EstimatedBytes:    estimatedBytes,
			AverageEntryBytes: avg,
			Metrics:           metrics,
		})
		return nil
	}

	for _, projection := range c.projections {
		entries, bytes, metrics := projection.estimate()
		if err := add(projection.key, projection.name, projection.projector, entries, bytes, metrics); err != nil {
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

func (p *RoomDirectoryProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	catalogEntries, catalogBytes, catalogMetrics := p.Catalog.adminProjectionEstimate()
	membershipEntries, membershipBytes, membershipMetrics := p.Membership.adminProjectionEstimate()
	banEntries, banBytes, banMetrics := p.Bans.adminProjectionEstimate()
	metrics := make([]ProjectionAdminMetric, 0, len(catalogMetrics)+len(membershipMetrics)+len(banMetrics))
	for _, metric := range catalogMetrics {
		metric.Name = "catalog_" + metric.Name
		metrics = append(metrics, metric)
	}
	for _, metric := range membershipMetrics {
		metric.Name = "membership_" + metric.Name
		metrics = append(metrics, metric)
	}
	for _, metric := range banMetrics {
		metric.Name = "bans_" + metric.Name
		metrics = append(metrics, metric)
	}
	return catalogEntries + membershipEntries + banEntries, catalogBytes + membershipBytes + banBytes, metrics
}

func (p *RoomBanProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var bans, bytes int64
	for roomID, users := range p.byRoom {
		bytes += projectionMapEntryOverhead + int64(len(roomID))
		for userID, ban := range users {
			bans++
			bytes += projectionMapEntryOverhead + int64(len(userID)+len(ban.EventID)+len(ban.ModeratorID)+len(ban.Reason)) + 32
		}
	}
	return bans, bytes, []ProjectionAdminMetric{
		{Name: "active_bans", Value: bans, Bytes: bytes},
		{Name: "rooms_with_bans", Value: int64(len(p.byRoom)), Bytes: 0},
	}
}

func (p *ConfigProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var values int64
	var notificationPolicyValues, notificationPolicyPayloadBytes, neighborPayloadBytes int64
	if p.server.serverName != "" {
		values++
	}
	if p.server.description != "" {
		values++
	}
	if p.server.welcomeMessage != "" {
		values++
	}
	if p.server.motd != "" {
		values++
	}
	if p.server.blockedUsernames != nil {
		values++
	}
	if p.server.logo != nil {
		values++
	}
	if p.server.banner != nil {
		values++
	}
	values += int64(len(p.server.neighbors)) * 3
	for _, neighbor := range p.server.neighbors {
		neighborPayloadBytes += projectionMapEntryOverhead + int64(len(neighbor.ID)+len(neighbor.Origin)+len(neighbor.Revision))
	}
	for _, u := range p.users {
		if u.timezone != nil {
			values++
		}
		if u.timeFormat != nil {
			values++
		}
		serverPolicyValues := notificationDeliveryModeFieldCount(u.serverModes)
		values += serverPolicyValues
		notificationPolicyValues += serverPolicyValues
		if u.serverModes != nil {
			notificationPolicyPayloadBytes += int64(proto.Size(u.serverModes))
		}
		for groupID, modes := range u.roomGroupModesByGroup {
			policyValues := notificationDeliveryModeFieldCount(modes)
			values += policyValues
			notificationPolicyValues += policyValues
			notificationPolicyPayloadBytes += projectionMapEntryOverhead + int64(len(groupID)) + int64(proto.Size(modes))
		}
		for roomID, modes := range u.roomModesByRoom {
			policyValues := notificationDeliveryModeFieldCount(modes)
			values += policyValues
			notificationPolicyValues += policyValues
			notificationPolicyPayloadBytes += projectionMapEntryOverhead + int64(len(roomID)) + int64(proto.Size(modes))
		}
	}
	subjects := int64(len(p.users))
	if p.server.serverName != "" ||
		p.server.description != "" ||
		p.server.welcomeMessage != "" ||
		p.server.motd != "" ||
		p.server.blockedUsernames != nil ||
		p.server.logo != nil ||
		p.server.banner != nil ||
		len(p.server.neighbors) > 0 {
		subjects++
	}
	bytes := values*projectionMapEntryOverhead + notificationPolicyPayloadBytes + neighborPayloadBytes
	return values, bytes, []ProjectionAdminMetric{
		{Name: "subjects", Value: subjects, Bytes: 0},
		{Name: "values", Value: values, Bytes: bytes},
		{Name: "notification_policy_values", Value: notificationPolicyValues, Bytes: notificationPolicyValues*projectionMapEntryOverhead + notificationPolicyPayloadBytes},
		{Name: "neighbors", Value: int64(len(p.server.neighbors)), Bytes: neighborPayloadBytes},
	}
}

func (p *RBACProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var roleBytes int64
	for name, role := range p.roles {
		roleBytes += projectionMapEntryOverhead + int64(len(name))
		if role != nil {
			roleBytes += int64(proto.Size(role))
		}
	}
	var assignmentBytes, assignments int64
	for userID, roles := range p.assignments {
		assignmentBytes += projectionMapEntryOverhead + int64(len(userID))
		for roleName := range roles {
			assignments++
			assignmentBytes += projectionMapEntryOverhead + int64(len(roleName))
		}
	}
	var decisionBytes int64
	for key, decision := range p.decisions {
		decisionBytes += projectionMapEntryOverhead + int64(len(key.scope)+len(key.scopeID)+len(key.subject)+len(key.permission)+len(decision))
	}
	retainedEventIDs := p.replayGuard.retainedEventIDs()
	retainedEventIDsBytes := estimateStringSetBytes(retainedEventIDs)
	totalEntries := int64(len(p.roles)) + assignments + int64(len(p.decisions))
	totalBytes := roleBytes + assignmentBytes + decisionBytes + retainedEventIDsBytes
	return totalEntries, totalBytes, []ProjectionAdminMetric{
		{Name: "roles", Value: int64(len(p.roles)), Bytes: roleBytes},
		{Name: "assignments", Value: assignments, Bytes: assignmentBytes},
		{Name: "permission_decisions", Value: int64(len(p.decisions)), Bytes: decisionBytes},
		{Name: "seen_event_ids", Value: int64(len(retainedEventIDs)), Bytes: retainedEventIDsBytes},
		{Name: "event_id_compatibility_mode", Value: p.replayGuard.compatibilityValue(), Bytes: 0},
	}
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

func (p *RoomGroupLayoutProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	groupEntries, groupBytes, groupMetrics := p.Groups.adminProjectionEstimate()
	layoutEntries, layoutBytes, layoutMetrics := p.Layout.adminProjectionEstimate()
	metrics := make([]ProjectionAdminMetric, 0, len(groupMetrics)+len(layoutMetrics))
	for _, metric := range groupMetrics {
		metric.Name = "groups_" + metric.Name
		metrics = append(metrics, metric)
	}
	for _, metric := range layoutMetrics {
		metric.Name = "layout_" + metric.Name
		metrics = append(metrics, metric)
	}
	return groupEntries + layoutEntries, groupBytes + layoutBytes, metrics
}

func (p *RoomTimelineProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var entries int64
	var roomIndexBytes int64
	for roomID, roomEntries := range p.byRoom {
		roomIndexBytes += projectionMapEntryOverhead + int64(len(roomID)) + int64(cap(roomEntries))*4
		entries += int64(len(roomEntries))
	}
	rawBytes := int64(cap(p.entries)) * int64(unsafe.Sizeof(timelineRow{}))
	sharedIDBytes := p.rooms.estimatedBytes() + p.users.estimatedBytes()

	var messagePostIndexBytes, messagePosts int64
	for roomID, roomEntries := range p.messagePostsByRoom {
		messagePostIndexBytes += projectionMapEntryOverhead + int64(len(roomID))
		messagePosts += int64(len(roomEntries))
		messagePostIndexBytes += int64(cap(roomEntries)) * 4
	}

	eventIndexBytes := int64(cap(p.rowByEvent)) * 4
	var indexedEvents int64
	for _, row := range p.rowByEvent {
		if row != 0 {
			indexedEvents++
		}
	}
	var eventIDBytes int64
	if !p.sharedEventIDs {
		eventIDBytes = p.eventIDs.estimatedBytes()
	}
	retainedEventIDs := p.replayGuard.retainedEventIDs()
	appliedEventIDsBytes := estimateStringSetBytes(retainedEventIDs)
	var bodyStateBytes, activeBodyReferenceBytes, activeBodyReferences, supersededSeqBytes, supersededSeqs int64
	bodyStateBytes = int64(cap(p.bodyStates))*int64(unsafe.Sizeof(timelineBodyState{})) + p.bodyEventIDs.retainedBytes()
	countBody := func(state timelineBodyState) {
		if state.currentSequence == 0 {
			return
		}
		if state.active() {
			activeBodyReferences++
			activeBodyReferenceBytes += int64(unsafe.Sizeof(state)) + int64(state.currentEventID.length())
		}
	}
	for _, state := range p.bodyStates {
		countBody(state)
	}
	for eventID, state := range p.orphanBodyStates {
		bodyStateBytes += projectionMapEntryOverhead + int64(len(eventID)) + int64(unsafe.Sizeof(timelineBodyState{}))
		countBody(state)
	}
	for _, history := range p.bodyHistory {
		bytes := projectionMapEntryOverhead + 4 + int64(unsafe.Sizeof(history)) + int64(cap(history))*8
		bodyStateBytes += bytes
		supersededSeqs += int64(len(history))
		supersededSeqBytes += bytes
	}
	for eventID, history := range p.orphanBodyHistory {
		bytes := projectionMapEntryOverhead + int64(len(eventID)) + int64(unsafe.Sizeof(history)) + int64(cap(history))*8
		bodyStateBytes += bytes
		supersededSeqs += int64(len(history))
		supersededSeqBytes += bytes
	}
	var retractedBytes int64
	for eventID := range p.retractedFlags {
		retractedBytes += projectionMapEntryOverhead + int64(len(eventID))
	}
	var tombstonedAtBytes int64
	for eventID := range p.tombstonedAt {
		tombstonedAtBytes += projectionMapEntryOverhead + int64(len(eventID)) + 24
	}
	var shreddedAtBytes int64
	for userID := range p.shreddedAt {
		shreddedAtBytes += projectionMapEntryOverhead + int64(len(userID)) + 24
	}
	hiddenEchoBytes := estimateStringSetBytes(p.hiddenEchoes)
	var echoBytes, echoLinks int64
	for eventID, echoes := range p.echoLinks {
		echoBytes += projectionMapEntryOverhead + int64(len(eventID))
		for _, echoID := range echoes {
			echoLinks++
			echoBytes += projectionSliceEntryOverhead + int64(len(echoID))
		}
	}
	shreddedUserBytes := estimateStringSetBytes(p.shreddedUsers)
	var pinnedMessageBytes, pinnedMessages int64
	for roomID, pins := range p.pinnedMessagesByRoom {
		pinnedMessageBytes += projectionMapEntryOverhead + int64(len(roomID))
		for messageID, pin := range pins {
			pinnedMessages++
			pinnedMessageBytes += projectionMapEntryOverhead + int64(len(messageID)+len(pin.PinEventID)+len(pin.RoomID)+len(pin.MessageEventID)) + 8
		}
	}
	var latestPinBytes int64
	for roomID, latest := range p.latestPinByRoom {
		latestPinBytes += projectionMapEntryOverhead + int64(len(roomID)+len(latest.PinEventID)) + 8
	}

	totalBytes := rawBytes + sharedIDBytes + roomIndexBytes + messagePostIndexBytes + eventIndexBytes + eventIDBytes +
		appliedEventIDsBytes + bodyStateBytes + retractedBytes +
		tombstonedAtBytes + shreddedAtBytes + hiddenEchoBytes + echoBytes + shreddedUserBytes +
		pinnedMessageBytes + latestPinBytes
	return entries, totalBytes, []ProjectionAdminMetric{
		{Name: "rooms", Value: int64(len(p.byRoom)), Bytes: 0},
		{Name: "timeline_entries", Value: entries, Bytes: rawBytes},
		{Name: "shared_room_user_ids", Value: int64(p.rooms.len() + p.users.len()), Bytes: sharedIDBytes},
		{Name: "room_timeline_index", Value: entries, Bytes: roomIndexBytes},
		{Name: "message_posts", Value: messagePosts, Bytes: 0},
		{Name: "message_posts_by_room_index", Value: messagePosts, Bytes: messagePostIndexBytes},
		{Name: "event_id_index", Value: indexedEvents, Bytes: eventIndexBytes},
		// Thread replies are indexed by event ID but are not room-visible rows.
		{Name: "event_id_retained_entries", Value: max(indexedEvents-entries, 0), Bytes: 0},
		{Name: "private_event_ids", Value: privateEventIDCount(p.sharedEventIDs, p.eventIDs), Bytes: eventIDBytes},
		{Name: "applied_event_ids", Value: int64(len(retainedEventIDs)), Bytes: appliedEventIDsBytes},
		{Name: "event_id_compatibility_mode", Value: p.replayGuard.compatibilityValue(), Bytes: 0},
		{Name: "body_state_index", Value: int64(len(p.bodyStates) + len(p.orphanBodyStates)), Bytes: bodyStateBytes},
		{Name: "active_body_references", Value: activeBodyReferences, Bytes: activeBodyReferenceBytes},
		{Name: "superseded_body_event_seqs", Value: supersededSeqs, Bytes: supersededSeqBytes},
		{Name: "retracted_flags", Value: int64(len(p.retractedFlags)), Bytes: retractedBytes},
		{Name: "tombstoned_at_index", Value: int64(len(p.tombstonedAt)), Bytes: tombstonedAtBytes},
		{Name: "shredded_at_index", Value: int64(len(p.shreddedAt)), Bytes: shreddedAtBytes},
		{Name: "hidden_echoes", Value: int64(len(p.hiddenEchoes)), Bytes: hiddenEchoBytes},
		{Name: "echo_links", Value: echoLinks, Bytes: echoBytes},
		{Name: "shredded_users", Value: int64(len(p.shreddedUsers)), Bytes: shreddedUserBytes},
		{Name: "pinned_messages", Value: pinnedMessages, Bytes: pinnedMessageBytes},
		{Name: "latest_pin_by_room", Value: int64(len(p.latestPinByRoom)), Bytes: latestPinBytes},
	}
}

func (p *ThreadProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var entries, rawBytes, replies int64
	for _, threadEntries := range p.byThread {
		rawBytes += projectionCompactMapEntryOverhead + 4 + projectionSliceEntryOverhead + int64(cap(threadEntries))*int64(unsafe.Sizeof(threadEntry{}))
		for _, entry := range threadEntries {
			entries++
			if entry.event != 0 {
				replies++
			}
		}
	}
	replyBytes := int64(len(p.replyRoots)) * (projectionCompactMapEntryOverhead + 8)
	var threadSummaryBytes, summaryParticipants int64
	for _, summary := range p.summaryByThread {
		threadSummaryBytes += projectionCompactMapEntryOverhead + 12 + int64(unsafe.Sizeof(threadSummary{}))
		if summary == nil {
			continue
		}
		summaryParticipants += int64(len(summary.participants))
		threadSummaryBytes += int64(cap(summary.participants))*int64(unsafe.Sizeof(threadParticipant{})) + int64(len(summary.participantIndex))*(projectionCompactMapEntryOverhead+12)
	}
	retainedEventIDs := p.replayGuard.retainedEventIDs()
	appliedEventIDsBytes := estimateStringSetBytes(retainedEventIDs)
	shreddedUserBytes := estimateStringSetBytes(p.shreddedUsers)
	followStateBytes := int64(len(p.followState)) * (projectionCompactMapEntryOverhead + int64(unsafe.Sizeof(threadFollowKey{})) + 1)
	var followerBytes, followerRefs int64
	for _, followers := range p.followers {
		followerRefs += int64(len(followers))
		followerBytes += projectionCompactMapEntryOverhead + int64(unsafe.Sizeof(threadFollowTarget{})) + projectionSliceEntryOverhead + int64(cap(followers))*4
	}
	var followedByUserBytes, followedRefs int64
	for _, followed := range p.followedByUser {
		followedRefs += int64(len(followed))
		followedByUserBytes += projectionCompactMapEntryOverhead + 4 + projectionSliceEntryOverhead + int64(cap(followed))*int64(unsafe.Sizeof(threadFollowTarget{}))
	}
	channelRoomBytes := estimateStringSetBytes(p.channelRooms)
	idTableBytes := p.principalIDs.estimatedBytes()
	if !p.sharedEventIDs {
		idTableBytes += p.eventIDs.estimatedBytes()
	}
	var messageRefs int64
	for _, ref := range p.messageRefs {
		if ref.room != 0 {
			messageRefs++
		}
	}
	messageRefBytes := int64(len(p.messageRefs)) * int64(unsafe.Sizeof(threadMessageRef{}))
	interactionBytes := int64(len(p.interactions)) * (int64(unsafe.Sizeof(threadInteractionKey{})) + 4 + projectionCompactMapEntryOverhead)
	followBytes := followStateBytes + followerBytes + followedByUserBytes
	totalEntries := entries + int64(len(p.followState)) + messageRefs + int64(len(p.interactions))
	totalBytes := rawBytes + replyBytes + threadSummaryBytes + appliedEventIDsBytes + shreddedUserBytes + followBytes + channelRoomBytes + idTableBytes + messageRefBytes + interactionBytes
	return totalEntries, totalBytes, []ProjectionAdminMetric{
		{Name: "threads", Value: int64(len(p.byThread)), Bytes: 0},
		{Name: "thread_entries", Value: entries, Bytes: rawBytes},
		{Name: "replies", Value: replies, Bytes: 0},
		{Name: "reply_summaries", Value: int64(len(p.replyRoots)), Bytes: replyBytes},
		{Name: "thread_summary_participants", Value: summaryParticipants, Bytes: threadSummaryBytes},
		{Name: "follow_states", Value: int64(len(p.followState)), Bytes: followStateBytes},
		{Name: "follower_refs", Value: followerRefs, Bytes: followerBytes},
		{Name: "followed_thread_refs", Value: followedRefs, Bytes: followedByUserBytes},
		{Name: "channel_rooms", Value: int64(len(p.channelRooms)), Bytes: channelRoomBytes},
		{Name: "interned_ids", Value: int64(p.principalIDs.len()) + privateEventIDCount(p.sharedEventIDs, p.eventIDs), Bytes: idTableBytes},
		{Name: "message_thread_refs", Value: messageRefs, Bytes: messageRefBytes},
		{Name: "interaction_refs", Value: int64(len(p.interactions)), Bytes: interactionBytes},
		{Name: "applied_event_ids", Value: int64(len(retainedEventIDs)), Bytes: appliedEventIDsBytes},
		{Name: "event_id_compatibility_mode", Value: p.replayGuard.compatibilityValue(), Bytes: 0},
		{Name: "shredded_users", Value: int64(len(p.shreddedUsers)), Bytes: shreddedUserBytes},
	}
}

func (p *ReactionProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var active, emojiGroups, bytes int64
	for _, reactions := range p.byMessage {
		bytes += projectionCompactMapEntryOverhead + 4 + projectionSliceEntryOverhead
		active += int64(len(reactions))
		bytes += int64(cap(reactions)) * int64(unsafe.Sizeof(reactionProjectionEntry{}))
		// Reactions are sorted by emoji, so each emoji change starts a group.
		for i, reaction := range reactions {
			bytes += int64(len(reaction.source))
			if i == 0 || reactions[i-1].emoji != reaction.emoji {
				emojiGroups++
			}
		}
	}
	var roomSeqBytes int64
	for roomID := range p.roomSeq {
		roomSeqBytes += projectionMapEntryOverhead + int64(len(roomID)) + 8
	}
	idTableBytes := p.ids.estimatedBytes()
	if !p.sharedEventIDs {
		idTableBytes += p.messages.estimatedBytes()
	}
	messageRoomBytes := int64(len(p.messageRooms))*4 + int64(len(p.echoOriginal))*(projectionCompactMapEntryOverhead+8)
	var assetRoomBytes int64
	for assetID, roomID := range p.assetRoom {
		assetRoomBytes += projectionMapEntryOverhead + int64(len(assetID)+len(roomID))
	}
	retainedEventIDs := p.replayGuard.retainedEventIDs()
	seenBytes := estimateStringSetBytes(retainedEventIDs)
	reactionBytes := bytes
	bytes += roomSeqBytes + idTableBytes + messageRoomBytes + assetRoomBytes + seenBytes
	return active, bytes, []ProjectionAdminMetric{
		{Name: "messages", Value: int64(len(p.byMessage)), Bytes: 0},
		{Name: "emoji_groups", Value: emojiGroups, Bytes: 0},
		{Name: "active_reactions", Value: active, Bytes: reactionBytes},
		{Name: "room_seq_index", Value: int64(len(p.roomSeq)), Bytes: roomSeqBytes},
		{Name: "interned_ids", Value: int64(p.ids.len()) + privateEventIDCount(p.sharedEventIDs, p.messages), Bytes: idTableBytes},
		{Name: "message_room_index", Value: int64(len(p.messageRooms)), Bytes: messageRoomBytes},
		{Name: "asset_room_index", Value: int64(len(p.assetRoom)), Bytes: assetRoomBytes},
		{Name: "seen_event_ids", Value: int64(len(retainedEventIDs)), Bytes: seenBytes},
		{Name: "event_id_compatibility_mode", Value: p.replayGuard.compatibilityValue(), Bytes: 0},
	}
}

func (p *UserProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var users, deleted, verifiedEmails, bytes int64
	for userID, user := range p.users {
		userBytes := projectionMapEntryOverhead + int64(len(userID))
		if user == nil {
			bytes += userBytes
			continue
		}
		if user.deleted {
			deleted++
		} else if user.user != nil {
			users++
		}
		if user.user != nil {
			userBytes += int64(proto.Size(user.user))
		}
		for _, pii := range []*projectedUserPII{user.login, user.displayName} {
			if pii != nil {
				userBytes += int64(len(pii.eventID)+len(pii.eventType)+len(pii.purpose)) + int64(proto.Size(pii.encrypted))
			}
		}
		if user.avatar != nil {
			userBytes += int64(proto.Size(user.avatar))
		}
		for hash, email := range user.verifiedEmail {
			verifiedEmails++
			userBytes += projectionMapEntryOverhead + int64(len(hash)) + 8
			if email.pii != nil {
				userBytes += int64(len(email.pii.eventID)+len(email.pii.eventType)+len(email.pii.purpose)) + int64(proto.Size(email.pii.encrypted))
			}
		}
		if user.preferences != nil {
			userBytes += int64(proto.Size(user.preferences))
		}
		bytes += userBytes
	}
	loginBytes := int64(len(p.loginIndex)) * projectionMapEntryOverhead
	for login, userID := range p.loginIndex {
		loginBytes += int64(len(login) + len(userID))
	}
	emailBytes := int64(len(p.emailIndex)) * projectionMapEntryOverhead
	for hash, userID := range p.emailIndex {
		emailBytes += int64(len(hash) + len(userID))
	}
	retainedEventIDs := p.replayGuard.retainedEventIDs()
	seenBytes := estimateStringSetBytes(retainedEventIDs)
	bytes += loginBytes + emailBytes + seenBytes
	return users, bytes, []ProjectionAdminMetric{
		{Name: "users", Value: users, Bytes: 0},
		{Name: "deleted_users", Value: deleted, Bytes: 0},
		{Name: "verified_emails", Value: verifiedEmails, Bytes: 0},
		{Name: "login_index", Value: int64(len(p.loginIndex)), Bytes: loginBytes},
		{Name: "email_index", Value: int64(len(p.emailIndex)), Bytes: emailBytes},
		{Name: "seen_event_ids", Value: int64(len(retainedEventIDs)), Bytes: seenBytes},
		{Name: "event_id_compatibility_mode", Value: p.replayGuard.compatibilityValue(), Bytes: 0},
	}
}

func (p *UserAuthProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var active, credentials, identities, consents, bytes int64
	for userID, user := range p.users {
		userBytes := projectionMapEntryOverhead + int64(len(userID))
		if user == nil {
			bytes += userBytes
			continue
		}
		if !user.deleted {
			active++
		}
		if len(user.passwordHash) > 0 {
			credentials++
			userBytes += projectionSliceEntryOverhead + int64(len(user.passwordHash))
		}
		for hash, identity := range user.externalIdentities {
			identities++
			userBytes += projectionMapEntryOverhead + int64(len(hash)+len(identity.ProviderID)+len(identity.ProviderType)+len(identity.Issuer)+len(identity.Subject)+len(identity.SubjectHash))
		}
		for origin := range user.oauthConsent {
			consents++
			userBytes += projectionMapEntryOverhead + int64(len(origin))
		}
		bytes += userBytes
	}
	indexBytes := int64(len(p.identityIndex)) * projectionMapEntryOverhead
	for hash, userID := range p.identityIndex {
		indexBytes += int64(len(hash) + len(userID))
	}
	retainedEventIDs := p.replayGuard.retainedEventIDs()
	seenBytes := estimateStringSetBytes(retainedEventIDs)
	bytes += indexBytes + seenBytes
	return active, bytes, []ProjectionAdminMetric{
		{Name: "active_accounts", Value: active, Bytes: 0},
		{Name: "password_credentials", Value: credentials, Bytes: 0},
		{Name: "external_identities", Value: identities, Bytes: 0},
		{Name: "oauth_consents", Value: consents, Bytes: 0},
		{Name: "external_identity_index", Value: int64(len(p.identityIndex)), Bytes: indexBytes},
		{Name: "seen_event_ids", Value: int64(len(retainedEventIDs)), Bytes: seenBytes},
		{Name: "event_id_compatibility_mode", Value: p.replayGuard.compatibilityValue(), Bytes: 0},
	}
}

func (p *ContentKeyProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	users := make(map[uint32]struct{})
	bytes := p.users.estimatedBytes()
	for id, record := range p.keys {
		users[id.user] = struct{}{}
		bytes += projectionCompactMapEntryOverhead + int64(unsafe.Sizeof(id)+unsafe.Sizeof(record))
		bytes += int64(len(record.contentKeyRef) + len(record.wrappingKeyRef) + cap(record.wrappingMetadata))
	}
	for algorithm := range p.algorithms {
		bytes += projectionMapEntryOverhead + int64(len(algorithm))
	}
	activeBytes := int64(len(p.activeEpoch)) * (projectionCompactMapEntryOverhead + int64(unsafe.Sizeof(contentKeyPurposeID{})) + 4)
	retainedEventIDs := p.replayGuard.retainedEventIDs()
	seenBytes := estimateStringSetBytes(retainedEventIDs)
	epochs := int64(len(p.keys))
	total := bytes + activeBytes + seenBytes
	return epochs, total, []ProjectionAdminMetric{
		{Name: "users", Value: int64(len(users)), Bytes: 0},
		{Name: "purposes", Value: int64(len(p.activeEpoch)), Bytes: 0},
		{Name: "dek_epochs", Value: epochs, Bytes: bytes},
		{Name: "active_epochs", Value: int64(len(p.activeEpoch)), Bytes: activeBytes},
		{Name: "seen_event_ids", Value: int64(len(retainedEventIDs)), Bytes: seenBytes},
		{Name: "event_id_compatibility_mode", Value: p.replayGuard.compatibilityValue(), Bytes: 0},
	}
}

// privateEventIDCount returns the number of IDs in a component-owned event ID
// table. A shared table is counted once by the ServerContentView estimate.
func privateEventIDCount(shared bool, table *eventIDTable) int64 {
	if shared {
		return 0
	}
	return int64(table.len())
}

func estimateStringSetBytes(values map[string]struct{}) int64 {
	var bytes int64
	for value := range values {
		bytes += projectionMapEntryOverhead + int64(len(value))
	}
	return bytes
}
