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

	states := make([]ProjectionAdminState, 0, len(c.projections))
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

	for _, projection := range c.projections {
		entries, bytes, metrics := projection.estimate()
		if err := add(projection.name, projection.projector, entries, bytes, metrics); err != nil {
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
	totalEntries := int64(len(p.roles)) + assignments + int64(len(p.decisions))
	totalBytes := roleBytes + assignmentBytes + decisionBytes
	return totalEntries, totalBytes, []ProjectionAdminMetric{
		{Name: "roles", Value: int64(len(p.roles)), Bytes: roleBytes},
		{Name: "assignments", Value: assignments, Bytes: assignmentBytes},
		{Name: "permission_decisions", Value: int64(len(p.decisions)), Bytes: decisionBytes},
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
		if user.avatar != nil {
			userBytes += int64(proto.Size(user.avatar))
		}
		if len(user.passwordHash) > 0 {
			userBytes += projectionSliceEntryOverhead + int64(len(user.passwordHash))
		}
		for hash, email := range user.verifiedEmail {
			verifiedEmails++
			userBytes += projectionMapEntryOverhead + int64(len(hash)+len(email.Email)) + 8
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
	oidcBytes := int64(len(p.oidcIndex)) * projectionMapEntryOverhead
	for hash, userID := range p.oidcIndex {
		oidcBytes += int64(len(hash) + len(userID))
	}
	seenBytes := int64(len(p.eventIDSeen)) * projectionMapEntryOverhead
	bytes += loginBytes + emailBytes + oidcBytes + seenBytes
	return users, bytes, []ProjectionAdminMetric{
		{Name: "users", Value: users, Bytes: 0},
		{Name: "deleted_users", Value: deleted, Bytes: 0},
		{Name: "verified_emails", Value: verifiedEmails, Bytes: 0},
		{Name: "login_index", Value: int64(len(p.loginIndex)), Bytes: loginBytes},
		{Name: "email_index", Value: int64(len(p.emailIndex)), Bytes: emailBytes},
		{Name: "oidc_index", Value: int64(len(p.oidcIndex)), Bytes: oidcBytes},
		{Name: "seen_event_ids", Value: int64(len(p.eventIDSeen)), Bytes: seenBytes},
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
	seenBytes := int64(len(p.eventIDSeen)) * projectionMapEntryOverhead
	bytes += activeBytes + seenBytes
	return epochs, bytes, []ProjectionAdminMetric{
		{Name: "users", Value: users, Bytes: 0},
		{Name: "purposes", Value: purposes, Bytes: 0},
		{Name: "dek_epochs", Value: epochs, Bytes: bytes - activeBytes - seenBytes},
		{Name: "active_epochs", Value: active, Bytes: activeBytes},
		{Name: "seen_event_ids", Value: int64(len(p.eventIDSeen)), Bytes: seenBytes},
	}
}

func timelineEntryEstimatedBytes(entry *TimelineEntry) int64 {
	if entry == nil || entry.Event == nil {
		return projectionSliceEntryOverhead
	}
	return projectionSliceEntryOverhead + int64(proto.Size(entry.Event)) + 8
}
