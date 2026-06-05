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

func (p *RoomDirectoryProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	catalogEntries, catalogBytes, catalogMetrics := p.Catalog.adminProjectionEstimate()
	membershipEntries, membershipBytes, membershipMetrics := p.Membership.adminProjectionEstimate()
	metrics := make([]ProjectionAdminMetric, 0, len(catalogMetrics)+len(membershipMetrics))
	for _, metric := range catalogMetrics {
		metric.Name = "catalog_" + metric.Name
		metrics = append(metrics, metric)
	}
	for _, metric := range membershipMetrics {
		metric.Name = "membership_" + metric.Name
		metrics = append(metrics, metric)
	}
	return catalogEntries + membershipEntries, catalogBytes + membershipBytes, metrics
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

	var visibleEntries, visibleBytes int64
	for roomID, roomEntries := range p.visibleByRoom {
		visibleBytes += projectionMapEntryOverhead + int64(len(roomID))
		for range roomEntries {
			visibleEntries++
			visibleBytes += projectionSliceEntryOverhead
		}
	}
	var eventIndexBytes int64
	for eventID := range p.byEventID {
		eventIndexBytes += projectionMapEntryOverhead + int64(len(eventID))
	}
	appliedEventIDsBytes := estimateStringSetBytes(p.appliedEventIDs)
	var latestBodyBytes int64
	for eventID, body := range p.latestBody {
		latestBodyBytes += projectionMapEntryOverhead + int64(len(eventID))
		if body != nil {
			latestBodyBytes += int64(proto.Size(body))
		}
	}
	var bodyEventSeqsBytes, bodyEventSeqs int64
	for eventID, seqs := range p.bodyEventSeqs {
		bodyEventSeqsBytes += projectionMapEntryOverhead + int64(len(eventID))
		for range seqs {
			bodyEventSeqs++
			bodyEventSeqsBytes += projectionSliceEntryOverhead + 8
		}
	}
	var currentBodySeqBytes int64
	for eventID := range p.currentBodySeq {
		currentBodySeqBytes += projectionMapEntryOverhead + int64(len(eventID)) + 8
	}
	var retractedBytes int64
	for eventID := range p.retractedFlags {
		retractedBytes += projectionMapEntryOverhead + int64(len(eventID))
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
	var assetCreationBytes int64
	for assetID, event := range p.assetCreations {
		assetCreationBytes += projectionMapEntryOverhead + int64(len(assetID))
		if event != nil {
			assetCreationBytes += int64(proto.Size(event))
		}
	}
	var assetChildrenBytes, assetChildLinks int64
	for assetID, children := range p.assetChildren {
		assetChildrenBytes += projectionMapEntryOverhead + int64(len(assetID))
		for _, childID := range children {
			assetChildLinks++
			assetChildrenBytes += projectionSliceEntryOverhead + int64(len(childID))
		}
	}
	var videoManifestBytes int64
	for assetID, manifest := range p.videoManifests {
		videoManifestBytes += projectionMapEntryOverhead + int64(len(assetID))
		if manifest == nil {
			continue
		}
		if manifest.Started != nil {
			videoManifestBytes += int64(proto.Size(manifest.Started))
		}
		if manifest.Succeeded != nil {
			videoManifestBytes += int64(proto.Size(manifest.Succeeded))
		}
		if manifest.Failed != nil {
			videoManifestBytes += int64(proto.Size(manifest.Failed))
		}
	}
	var assetOwnerBytes int64
	for assetID, owner := range p.assetMessageOwner {
		assetOwnerBytes += projectionMapEntryOverhead + int64(len(assetID)+len(owner.roomID)+len(owner.messageEventID))
	}
	shreddedUserBytes := estimateStringSetBytes(p.shreddedUsers)

	totalBytes := rawBytes + visibleBytes + eventIndexBytes + appliedEventIDsBytes +
		latestBodyBytes + bodyEventSeqsBytes + currentBodySeqBytes + retractedBytes +
		hiddenEchoBytes + echoBytes + assetCreationBytes + assetChildrenBytes +
		videoManifestBytes + assetOwnerBytes + shreddedUserBytes
	return entries, totalBytes, []ProjectionAdminMetric{
		{Name: "rooms", Value: int64(len(p.byRoom)), Bytes: 0},
		{Name: "timeline_entries", Value: entries, Bytes: rawBytes},
		{Name: "visible_timeline_index", Value: visibleEntries, Bytes: visibleBytes},
		{Name: "message_posts", Value: messagePosts, Bytes: 0},
		{Name: "event_id_index", Value: int64(len(p.byEventID)), Bytes: eventIndexBytes},
		{Name: "applied_event_ids", Value: int64(len(p.appliedEventIDs)), Bytes: appliedEventIDsBytes},
		{Name: "latest_body_index", Value: int64(len(p.latestBody)), Bytes: latestBodyBytes},
		{Name: "body_event_seqs", Value: bodyEventSeqs, Bytes: bodyEventSeqsBytes},
		{Name: "current_body_seq_index", Value: int64(len(p.currentBodySeq)), Bytes: currentBodySeqBytes},
		{Name: "retracted_flags", Value: int64(len(p.retractedFlags)), Bytes: retractedBytes},
		{Name: "hidden_echoes", Value: int64(len(p.hiddenEchoes)), Bytes: hiddenEchoBytes},
		{Name: "echo_links", Value: echoLinks, Bytes: echoBytes},
		{Name: "asset_creations", Value: int64(len(p.assetCreations)), Bytes: assetCreationBytes},
		{Name: "asset_child_links", Value: assetChildLinks, Bytes: assetChildrenBytes},
		{Name: "video_manifests", Value: int64(len(p.videoManifests)), Bytes: videoManifestBytes},
		{Name: "asset_message_owner_index", Value: int64(len(p.assetMessageOwner)), Bytes: assetOwnerBytes},
		{Name: "shredded_users", Value: int64(len(p.shreddedUsers)), Bytes: shreddedUserBytes},
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
	appliedEventIDsBytes := estimateStringSetBytes(p.appliedEventIDs)
	shreddedUserBytes := estimateStringSetBytes(p.shreddedUsers)
	totalBytes := rawBytes + indexBytes + appliedEventIDsBytes + shreddedUserBytes
	return entries, totalBytes, []ProjectionAdminMetric{
		{Name: "threads", Value: int64(len(p.byThread)), Bytes: 0},
		{Name: "thread_entries", Value: entries, Bytes: rawBytes},
		{Name: "replies", Value: replies, Bytes: 0},
		{Name: "message_to_thread_index", Value: int64(len(p.messageToThread)), Bytes: indexBytes},
		{Name: "applied_event_ids", Value: int64(len(p.appliedEventIDs)), Bytes: appliedEventIDsBytes},
		{Name: "shredded_users", Value: int64(len(p.shreddedUsers)), Bytes: shreddedUserBytes},
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

func estimateStringSetBytes(values map[string]struct{}) int64 {
	var bytes int64
	for value := range values {
		bytes += projectionMapEntryOverhead + int64(len(value))
	}
	return bytes
}
