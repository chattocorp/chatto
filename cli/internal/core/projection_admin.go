package core

import (
	"context"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
)

const (
	projectionMapEntryOverhead   int64 = 64
	projectionSliceEntryOverhead int64 = 24
	projectionIntIndexBytes      int64 = 8
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
	for _, u := range p.users {
		if u.timezone != nil {
			values++
		}
		if u.timeFormat != nil {
			values++
		}
		if u.serverLevel != nil {
			values++
		}
		values += int64(len(u.roomLevelByRoom))
	}
	subjects := int64(len(p.users))
	if p.server.serverName != "" ||
		p.server.description != "" ||
		p.server.welcomeMessage != "" ||
		p.server.motd != "" ||
		p.server.blockedUsernames != nil ||
		p.server.logo != nil ||
		p.server.banner != nil {
		subjects++
	}
	bytes := values * projectionMapEntryOverhead
	return values, bytes, []ProjectionAdminMetric{
		{Name: "subjects", Value: subjects, Bytes: 0},
		{Name: "values", Value: values, Bytes: bytes},
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
	entries := int64(len(p.entries))
	var rawBytes int64
	for i := range p.entries {
		rawBytes += timelineEntryEstimatedBytes(&p.entries[i])
	}

	var roomIndexBytes, roomEntryCount int64
	for _, refs := range p.byRoom {
		roomIndexBytes += projectionMapEntryOverhead + int64(cap(refs))*4
		roomEntryCount += int64(len(refs))
	}

	var messagePostIndexBytes, messagePosts int64
	for _, roomEntries := range p.messagePostsByRoom {
		messagePostIndexBytes += projectionMapEntryOverhead
		messagePosts += int64(len(roomEntries))
		messagePostIndexBytes += int64(cap(roomEntries)) * 4
	}

	var eventIDBytes int64
	for _, id := range p.eventIDs.values {
		eventIDBytes += int64(len(id))
	}
	eventIDBytes += int64(len(p.eventIDs.byValue))*projectionMapEntryOverhead + int64(cap(p.eventIDs.values))*16
	var roomIDBytes int64
	for _, id := range p.roomIDs.values {
		roomIDBytes += int64(len(id))
	}
	roomIDBytes += int64(len(p.roomIDs.byValue))*projectionMapEntryOverhead + int64(cap(p.roomIDs.values))*16
	var userIDBytes int64
	for _, id := range p.userIDs.values {
		userIDBytes += int64(len(id))
	}
	userIDBytes += int64(len(p.userIDs.byValue))*projectionMapEntryOverhead + int64(cap(p.userIDs.values))*16
	eventIndexBytes := eventIDBytes + int64(cap(p.entryByEvent))*4
	retainedEventIDs := p.replayGuard.retainedEventIDs()
	appliedEventIDsBytes := estimateStringSetBytes(retainedEventIDs)
	var bodyStateBytes, latestBodyBytes, latestBodies, supersededSeqBytes, supersededSeqs int64
	for eventIndex, state := range p.bodyStates {
		if !state.known {
			continue
		}
		if p.currentBodies[eventIndex] != nil {
			latestBodies++
			latestBodyBytes += int64(proto.Size(p.currentBodies[eventIndex]))
		}
		superseded := p.supersededBodySequences[timelineEventRef(eventIndex)]
		supersededSeqs += int64(len(superseded))
		supersededSeqBytes += int64(cap(superseded)) * 8
	}
	bodyStateBytes = int64(cap(p.bodyStates))*40 + int64(cap(p.currentBodies))*8 +
		int64(len(p.supersededBodySequences))*projectionMapEntryOverhead + supersededSeqBytes + latestBodyBytes
	messageFlagBytes := int64(cap(p.messageFlags))
	tombstonedAtBytes := int64(len(p.tombstonedAt)) * (projectionMapEntryOverhead + 4 + 24)
	shreddedAtBytes := int64(len(p.shreddedAt)) * (projectionMapEntryOverhead + 4 + 24)
	var echoBytes, echoLinks int64
	for _, echoes := range p.echoLinks {
		echoBytes += projectionMapEntryOverhead + int64(cap(echoes))*4
		echoLinks += int64(len(echoes))
	}
	shreddedUserBytes := int64(cap(p.shreddedUsers))
	var bucketBytes, bucketRefs, bucketRefBytes, residentBuckets, residentBucketPayloadBytes int64
	for key, bucket := range p.buckets {
		bucketBytes += projectionMapEntryOverhead + int64(len(key.roomID)) + 8 + 24 + 8 + 8 + 1 + 8 + 8 + 24
		bucketRefs += int64(bucket.referenceCount)
		bucketRefBytes += int64(cap(bucket.encodedRefs))
		bucketBytes += int64(cap(bucket.encodedRefs))
		if bucket.resident {
			residentBuckets++
			residentBucketPayloadBytes += bucket.residentBytes
		}
	}

	totalBytes := rawBytes + roomIndexBytes + messagePostIndexBytes + eventIndexBytes + roomIDBytes + userIDBytes +
		appliedEventIDsBytes + bodyStateBytes + messageFlagBytes +
		tombstonedAtBytes + shreddedAtBytes + echoBytes + shreddedUserBytes + bucketBytes
	return entries, totalBytes, []ProjectionAdminMetric{
		{Name: "rooms", Value: int64(len(p.byRoom)), Bytes: 0},
		{Name: "timeline_entries", Value: entries, Bytes: rawBytes},
		{Name: "room_entry_index", Value: roomEntryCount, Bytes: roomIndexBytes},
		{Name: "message_posts", Value: messagePosts, Bytes: 0},
		{Name: "message_posts_by_room_index", Value: messagePosts, Bytes: messagePostIndexBytes},
		{Name: "event_id_index", Value: int64(len(p.eventIDs.byValue)), Bytes: eventIndexBytes},
		{Name: "room_id_dictionary", Value: int64(len(p.roomIDs.byValue)), Bytes: roomIDBytes},
		{Name: "user_id_dictionary", Value: int64(len(p.userIDs.byValue)), Bytes: userIDBytes},
		{Name: "applied_event_ids", Value: int64(len(retainedEventIDs)), Bytes: appliedEventIDsBytes},
		{Name: "event_id_compatibility_mode", Value: p.replayGuard.compatibilityValue(), Bytes: 0},
		{Name: "body_state_index", Value: int64(len(p.bodyStates) - 1), Bytes: bodyStateBytes},
		{Name: "latest_bodies", Value: latestBodies, Bytes: latestBodyBytes},
		{Name: "timeline_buckets", Value: int64(len(p.buckets)), Bytes: bucketBytes},
		{Name: "resident_timeline_buckets", Value: residentBuckets, Bytes: 0},
		{Name: "resident_timeline_bucket_payloads", Value: residentBuckets, Bytes: residentBucketPayloadBytes},
		{Name: "cold_timeline_buckets", Value: int64(len(p.buckets)) - residentBuckets, Bytes: 0},
		{Name: "timeline_bucket_event_refs", Value: bucketRefs, Bytes: bucketRefBytes},
		{Name: "superseded_body_event_seqs", Value: supersededSeqs, Bytes: supersededSeqBytes},
		{Name: "message_flags", Value: int64(len(p.messageFlags) - 1), Bytes: messageFlagBytes},
		{Name: "tombstoned_at_index", Value: int64(len(p.tombstonedAt)), Bytes: tombstonedAtBytes},
		{Name: "shredded_at_index", Value: int64(len(p.shreddedAt)), Bytes: shreddedAtBytes},
		{Name: "echo_links", Value: echoLinks, Bytes: echoBytes},
		{Name: "shredded_users", Value: int64(len(p.shreddedUsers) - 1), Bytes: shreddedUserBytes},
	}
}

func (p *ThreadProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var entries, rawBytes, replies int64
	for _, threadEntries := range p.byThread {
		rawBytes += projectionMapEntryOverhead + int64(cap(threadEntries))*16
		for _, entry := range threadEntries {
			entries++
			if entry.Event != 0 {
				replies++
			}
		}
	}
	var eventIDBytes int64
	for _, id := range p.eventIDs.values {
		eventIDBytes += int64(len(id))
	}
	eventIDBytes += int64(len(p.eventIDs.byValue))*projectionMapEntryOverhead + int64(cap(p.eventIDs.values))*16
	indexBytes := int64(cap(p.messageToThread)) * 4
	replySummaryBytes := int64(cap(p.replySummaries)) * 24
	var roomIDBytes, userIDBytes int64
	for _, id := range p.roomIDs.values {
		roomIDBytes += int64(len(id))
	}
	for _, id := range p.userIDs.values {
		userIDBytes += int64(len(id))
	}
	roomIDBytes += int64(len(p.roomIDs.byValue))*projectionMapEntryOverhead + int64(cap(p.roomIDs.values))*16
	userIDBytes += int64(len(p.userIDs.byValue))*projectionMapEntryOverhead + int64(cap(p.userIDs.values))*16
	var threadSummaryBytes, summaryParticipants int64
	for _, summary := range p.summaryByThread {
		threadSummaryBytes += projectionMapEntryOverhead + 64
		if summary == nil {
			continue
		}
		if summary.lastReplyAt != nil {
			threadSummaryBytes += 24
		}
		summaryParticipants += int64(len(summary.participantIDs))
		threadSummaryBytes += int64(cap(summary.participantIDs)+cap(summary.participantCounts)) * 4
	}
	retainedEventIDs := p.replayGuard.retainedEventIDs()
	appliedEventIDsBytes := estimateStringSetBytes(retainedEventIDs)
	shreddedUserBytes := int64(cap(p.shreddedUsers))
	followStateBytes := int64(len(p.followState)) * (projectionMapEntryOverhead + 12 + 1)
	var followerBytes, followerRefs int64
	for _, followers := range p.followers {
		followerBytes += projectionMapEntryOverhead + 8
		followerRefs += int64(len(followers))
		followerBytes += int64(cap(followers)) * 4
	}
	var followedByUserBytes, followedRefs int64
	for _, followed := range p.followedByUser {
		followedByUserBytes += projectionMapEntryOverhead + 4
		followedRefs += int64(len(followed))
		followedByUserBytes += int64(cap(followed)) * 8
	}
	followBytes := followStateBytes + followerBytes + followedByUserBytes
	totalEntries := entries + int64(len(p.followState))
	totalBytes := rawBytes + eventIDBytes + roomIDBytes + userIDBytes + indexBytes + replySummaryBytes + threadSummaryBytes + appliedEventIDsBytes + shreddedUserBytes + followBytes
	return totalEntries, totalBytes, []ProjectionAdminMetric{
		{Name: "threads", Value: int64(len(p.byThread)), Bytes: 0},
		{Name: "thread_entries", Value: entries, Bytes: rawBytes},
		{Name: "replies", Value: replies, Bytes: 0},
		{Name: "event_id_dictionary", Value: int64(len(p.eventIDs.byValue)), Bytes: eventIDBytes},
		{Name: "room_id_dictionary", Value: int64(len(p.roomIDs.byValue)), Bytes: roomIDBytes},
		{Name: "user_id_dictionary", Value: int64(len(p.userIDs.byValue)), Bytes: userIDBytes},
		{Name: "message_to_thread_index", Value: int64(len(p.messageToThread) - 1), Bytes: indexBytes},
		{Name: "reply_summaries", Value: int64(len(p.replySummaries) - 1), Bytes: replySummaryBytes},
		{Name: "thread_summary_participants", Value: summaryParticipants, Bytes: threadSummaryBytes},
		{Name: "follow_states", Value: int64(len(p.followState)), Bytes: followStateBytes},
		{Name: "follower_refs", Value: followerRefs, Bytes: followerBytes},
		{Name: "followed_thread_refs", Value: followedRefs, Bytes: followedByUserBytes},
		{Name: "applied_event_ids", Value: int64(len(retainedEventIDs)), Bytes: appliedEventIDsBytes},
		{Name: "event_id_compatibility_mode", Value: p.replayGuard.compatibilityValue(), Bytes: 0},
		{Name: "shredded_users", Value: int64(len(p.shreddedUsers) - 1), Bytes: shreddedUserBytes},
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
	var roomSeqBytes int64
	for roomID := range p.roomSeq {
		roomSeqBytes += projectionMapEntryOverhead + int64(len(roomID)) + 8
	}
	var messageRoomBytes int64
	for messageID, roomID := range p.messageRoom {
		messageRoomBytes += projectionMapEntryOverhead + int64(len(messageID)+len(roomID))
	}
	var assetRoomBytes int64
	for assetID, roomID := range p.assetRoom {
		assetRoomBytes += projectionMapEntryOverhead + int64(len(assetID)+len(roomID))
	}
	retainedEventIDs := p.replayGuard.retainedEventIDs()
	seenBytes := estimateStringSetBytes(retainedEventIDs)
	bytes += roomSeqBytes + messageRoomBytes + assetRoomBytes + seenBytes
	return active, bytes, []ProjectionAdminMetric{
		{Name: "messages", Value: int64(len(p.byMessage)), Bytes: 0},
		{Name: "emoji_groups", Value: emojiGroups, Bytes: 0},
		{Name: "active_reactions", Value: active, Bytes: bytes - roomSeqBytes - messageRoomBytes - assetRoomBytes - seenBytes},
		{Name: "room_seq_index", Value: int64(len(p.roomSeq)), Bytes: roomSeqBytes},
		{Name: "message_room_index", Value: int64(len(p.messageRoom)), Bytes: messageRoomBytes},
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
	var users, purposes, epochs, active, bytes int64
	for userID, byPurpose := range p.byUserPurposeEpoch {
		users++
		bytes += projectionMapEntryOverhead + int64(len(userID))
		for _, byEpoch := range byPurpose {
			purposes++
			bytes += projectionMapEntryOverhead
			for _, event := range byEpoch {
				epochs++
				bytes += projectionMapEntryOverhead
				if event != nil {
					bytes += int64(proto.Size(event))
				}
			}
		}
	}
	var activeBytes int64
	for userID, byPurpose := range p.activeEpoch {
		activeBytes += projectionMapEntryOverhead + int64(len(userID))
		for range byPurpose {
			active++
			activeBytes += projectionMapEntryOverhead + 8
		}
	}
	retainedEventIDs := p.replayGuard.retainedEventIDs()
	seenBytes := estimateStringSetBytes(retainedEventIDs)
	bytes += activeBytes + seenBytes
	return epochs, bytes, []ProjectionAdminMetric{
		{Name: "users", Value: users, Bytes: 0},
		{Name: "purposes", Value: purposes, Bytes: 0},
		{Name: "dek_epochs", Value: epochs, Bytes: bytes - activeBytes - seenBytes},
		{Name: "active_epochs", Value: active, Bytes: activeBytes},
		{Name: "seen_event_ids", Value: int64(len(retainedEventIDs)), Bytes: seenBytes},
		{Name: "event_id_compatibility_mode", Value: p.replayGuard.compatibilityValue(), Bytes: 0},
	}
}

func timelineEntryEstimatedBytes(entry *TimelineEntry) int64 {
	if entry == nil {
		return projectionSliceEntryOverhead
	}
	bytes := projectionSliceEntryOverhead + 8 + 8 + 4 + 4 + 4 + 4 + 24 + 1
	if entry.Event != nil {
		bytes += int64(proto.Size(entry.Event))
	}
	return bytes
}

func estimateStringSetBytes(values map[string]struct{}) int64 {
	var bytes int64
	for value := range values {
		bytes += projectionMapEntryOverhead + int64(len(value))
	}
	return bytes
}
