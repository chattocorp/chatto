package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type esBootVerificationReport struct {
	legacy       esLegacyCounts
	projected    esProjectedCounts
	eventCounts  map[string]int
	decodeErrors int
	warnings     []string
	problems     []string
}

type esLegacyCounts struct {
	rooms               int
	memberships         int
	roomGroups          int
	roomLayoutPresent   bool
	serverConfigPresent bool
	messages            int
	threadReplies       int
	reactions           int
	users               int
	verifiedEmails      int
	oidcSubjects        int
	encryptionKeys      int
	rbacKeys            int
}

type esProjectedCounts struct {
	rooms                  int
	channelRooms           int
	dmRooms                int
	membershipRooms        int
	memberships            int
	roomGroups             int
	roomLayoutGroups       int
	serverConfigConfigured bool
	timelineRooms          int
	timelineEntries        int
	messagePosts           int
	threads                int
	threadEntries          int
	threadReplies          int
	reactionMessages       int
	activeReactions        int
	users                  int
	verifiedEmails         int
	oidcSubjects           int
	rbacRoles              int
	rbacAssignments        int
	rbacDecisions          int
}

// logESBootVerification emits a structured summary of the ES import and
// projection state after the normal chatto run boot path has started
// projectors. It is intentionally part of the main process: embedded-NATS
// deployments cannot safely run a second verifier process over the same data
// directory.
func (c *ChattoCore) logESBootVerification(ctx context.Context) {
	startedAt := time.Now()
	report, err := c.buildESBootVerificationReport(ctx)
	if err != nil {
		c.logger.Warn("ES boot verification failed to build report", "error", err)
		return
	}

	c.evaluateESBootVerificationReport(report)
	c.logger.Info(
		"ES boot verification summary",
		"legacy_rooms", report.legacy.rooms,
		"projected_rooms", report.projected.rooms,
		"legacy_memberships", report.legacy.memberships,
		"projected_memberships", report.projected.memberships,
		"legacy_room_groups", report.legacy.roomGroups,
		"projected_room_groups", report.projected.roomGroups,
		"legacy_messages", report.legacy.messages,
		"projected_message_posts", report.projected.messagePosts,
		"legacy_thread_replies", report.legacy.threadReplies,
		"projected_thread_replies", report.projected.threadReplies,
		"legacy_reactions", report.legacy.reactions,
		"projected_active_reactions", report.projected.activeReactions,
		"legacy_users", report.legacy.users,
		"projected_users", report.projected.users,
		"legacy_verified_emails", report.legacy.verifiedEmails,
		"projected_verified_emails", report.projected.verifiedEmails,
		"legacy_oidc_subjects", report.legacy.oidcSubjects,
		"projected_oidc_subjects", report.projected.oidcSubjects,
		"legacy_rbac_keys", report.legacy.rbacKeys,
		"projected_rbac_roles", report.projected.rbacRoles,
		"projected_rbac_assignments", report.projected.rbacAssignments,
		"projected_rbac_decisions", report.projected.rbacDecisions,
		"server_config_legacy", report.legacy.serverConfigPresent,
		"server_config_projected", report.projected.serverConfigConfigured,
		"room_layout_legacy", report.legacy.roomLayoutPresent,
		"projected_room_layout_groups", report.projected.roomLayoutGroups,
		"evt_decode_errors", report.decodeErrors,
		"problem_count", len(report.problems),
		"warning_count", len(report.warnings),
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)

	c.logger.Info(
		"ES boot verification projection detail",
		"channel_rooms", report.projected.channelRooms,
		"dm_rooms", report.projected.dmRooms,
		"membership_rooms", report.projected.membershipRooms,
		"timeline_rooms", report.projected.timelineRooms,
		"timeline_entries", report.projected.timelineEntries,
		"threads", report.projected.threads,
		"thread_entries", report.projected.threadEntries,
		"thread_replies", report.projected.threadReplies,
		"reaction_messages", report.projected.reactionMessages,
		"verified_emails", report.projected.verifiedEmails,
		"oidc_subjects", report.projected.oidcSubjects,
	)

	c.logESEventCounts(report.eventCounts)
	for _, warning := range report.warnings {
		c.logger.Warn("ES boot verification warning", "warning", warning)
	}
	for _, problem := range report.problems {
		c.logger.Warn("ES boot verification problem", "problem", problem)
	}
	if len(report.problems) == 0 {
		c.logger.Info("ES boot verification passed")
	}
}

func (c *ChattoCore) buildESBootVerificationReport(ctx context.Context) (*esBootVerificationReport, error) {
	eventCounts, decodeErrors, err := c.countEVTEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("count EVT events: %w", err)
	}
	legacy, warnings, err := c.collectLegacyESCounts(ctx)
	if err != nil {
		return nil, err
	}
	return &esBootVerificationReport{
		legacy:       legacy,
		projected:    c.collectProjectedESCounts(),
		eventCounts:  eventCounts,
		decodeErrors: decodeErrors,
		warnings:     warnings,
	}, nil
}

func (c *ChattoCore) collectLegacyESCounts(ctx context.Context) (esLegacyCounts, []string, error) {
	var counts esLegacyCounts
	var warnings []string
	var err error

	counts.rooms, err = countKVKeys(ctx, c.storage.serverConfigKV, "room.channel.*", "room.dm.*")
	if err != nil {
		return counts, warnings, fmt.Errorf("count legacy rooms: %w", err)
	}
	counts.memberships, err = countKVKeys(ctx, c.storage.serverConfigKV, "room_membership.>")
	if err != nil {
		return counts, warnings, fmt.Errorf("count legacy memberships: %w", err)
	}
	counts.roomGroups, err = countKVKeys(ctx, c.storage.serverConfigKV, "room_group.*")
	if err != nil {
		return counts, warnings, fmt.Errorf("count legacy room groups: %w", err)
	}
	counts.roomLayoutPresent, err = kvKeyExists(ctx, c.storage.serverConfigKV, "room_layout")
	if err != nil {
		return counts, warnings, fmt.Errorf("check legacy room layout: %w", err)
	}
	counts.serverConfigPresent, err = kvKeyExists(ctx, c.storage.runtimeConfigKV, "config.instance")
	if err != nil {
		return counts, warnings, fmt.Errorf("check legacy server config: %w", err)
	}
	counts.messages, err = countStreamMessages(ctx, c.storage.serverEventsStream, []string{"server.room.*.*.msg.>"})
	if err != nil {
		return counts, warnings, fmt.Errorf("count legacy messages: %w", err)
	}
	counts.threadReplies, err = countStreamMessages(ctx, c.storage.serverEventsStream, []string{"server.room.*.*.msg.*.replies.>"})
	if err != nil {
		return counts, warnings, fmt.Errorf("count legacy thread replies: %w", err)
	}
	counts.reactions, err = countKVKeys(ctx, c.storage.serverReactionsKV)
	if err != nil {
		return counts, warnings, fmt.Errorf("count legacy reactions: %w", err)
	}
	counts.users, err = countLegacyUserRecords(ctx, c.storage.serverConfigKV)
	if err != nil {
		return counts, warnings, fmt.Errorf("count legacy users: %w", err)
	}
	counts.verifiedEmails, err = countKVKeys(ctx, c.storage.serverConfigKV, "verified_emails.*.*")
	if err != nil {
		return counts, warnings, fmt.Errorf("count legacy verified emails: %w", err)
	}
	counts.oidcSubjects, err = countKVKeys(ctx, c.storage.serverConfigKV, "user_by_oidc.*")
	if err != nil {
		return counts, warnings, fmt.Errorf("count legacy OIDC subjects: %w", err)
	}
	counts.encryptionKeys, err = countKVKeys(ctx, c.storage.encryptionKV)
	if err != nil {
		return counts, warnings, fmt.Errorf("count encryption keys: %w", err)
	}
	counts.rbacKeys, err = countKVKeys(ctx, c.storage.serverRBACKV)
	if err != nil {
		return counts, warnings, fmt.Errorf("count legacy RBAC keys: %w", err)
	}
	if counts.encryptionKeys == 0 {
		warnings = append(warnings, "ENCRYPTION_KEYS is empty; encrypted message bodies will not decrypt in local smoke tests")
	}
	return counts, warnings, nil
}

func (c *ChattoCore) collectProjectedESCounts() esProjectedCounts {
	membershipRooms, memberships := c.RoomMembership.Stats()
	timelineRooms, timelineEntries, messagePosts := c.RoomTimeline.Stats()
	threads, threadEntries, threadReplies := c.Threads.Stats()
	reactionMessages, activeReactions := c.Reactions.Stats()
	users, verifiedEmails, oidcSubjects := c.Users.Stats()
	rbacRoles, rbacAssignments, rbacDecisions := c.RBAC.CountStats()
	_, serverConfigConfigured := c.ServerConfig.Get()

	return esProjectedCounts{
		rooms:                  c.RoomCatalog.Count(),
		channelRooms:           len(c.RoomCatalog.AllByKind(corev1.RoomKind_ROOM_KIND_CHANNEL)),
		dmRooms:                len(c.RoomCatalog.AllByKind(corev1.RoomKind_ROOM_KIND_DM)),
		membershipRooms:        membershipRooms,
		memberships:            memberships,
		roomGroups:             c.RoomGroups.Count(),
		roomLayoutGroups:       len(c.RoomLayout.Order()),
		serverConfigConfigured: serverConfigConfigured,
		timelineRooms:          timelineRooms,
		timelineEntries:        timelineEntries,
		messagePosts:           messagePosts,
		threads:                threads,
		threadEntries:          threadEntries,
		threadReplies:          threadReplies,
		reactionMessages:       reactionMessages,
		activeReactions:        activeReactions,
		users:                  users,
		verifiedEmails:         verifiedEmails,
		oidcSubjects:           oidcSubjects,
		rbacRoles:              rbacRoles,
		rbacAssignments:        rbacAssignments,
		rbacDecisions:          rbacDecisions,
	}
}

func (c *ChattoCore) evaluateESBootVerificationReport(r *esBootVerificationReport) {
	compareAtLeast := func(name string, legacy, projected int) {
		if projected < legacy {
			r.problems = append(r.problems, fmt.Sprintf("%s: projected %d < legacy %d", name, projected, legacy))
		}
	}
	compareAtLeast("rooms", r.legacy.rooms, r.projected.rooms)
	compareAtLeast("memberships", r.legacy.memberships, r.projected.memberships)
	compareAtLeast("room groups", r.legacy.roomGroups, r.projected.roomGroups)
	compareAtLeast("messages", r.legacy.messages, r.projected.messagePosts)
	compareAtLeast("thread replies", r.legacy.threadReplies, r.projected.threadReplies)
	compareAtLeast("reactions", r.legacy.reactions, r.projected.activeReactions)
	compareAtLeast("users", r.legacy.users, r.projected.users)
	compareAtLeast("verified emails", r.legacy.verifiedEmails, r.projected.verifiedEmails)
	compareAtLeast("OIDC subjects", r.legacy.oidcSubjects, r.projected.oidcSubjects)

	if r.legacy.serverConfigPresent && !r.projected.serverConfigConfigured {
		r.problems = append(r.problems, "server config: legacy config.instance exists but projection is not configured")
	}
	if r.legacy.roomLayoutPresent && r.projected.roomGroups > 0 && r.projected.roomLayoutGroups == 0 {
		r.problems = append(r.problems, "room layout: legacy room_layout exists but projected layout ordering is empty")
	}
	if r.legacy.rbacKeys > 0 && r.projected.rbacRoles == 0 {
		r.problems = append(r.problems, "RBAC: projection has no roles")
	}
	if r.legacy.rbacKeys > 0 && r.projected.rbacDecisions == 0 {
		r.problems = append(r.problems, "RBAC: projection has no permission decisions")
	}
	if r.decodeErrors > 0 {
		r.problems = append(r.problems, fmt.Sprintf("EVT contains %d decode errors", r.decodeErrors))
	}
	sort.Strings(r.problems)
}

func (c *ChattoCore) countEVTEvents(ctx context.Context) (map[string]int, int, error) {
	consumer, err := c.storage.serverEvtStream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubjects:    []string{"evt.>"},
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 30 * time.Second,
	})
	if err != nil {
		return nil, 0, err
	}
	defer c.storage.serverEvtStream.DeleteConsumer(context.Background(), consumer.CachedInfo().Name)

	info, err := consumer.Info(ctx)
	if err != nil {
		return nil, 0, err
	}

	remaining := int(info.NumPending)
	counts := make(map[string]int)
	var decodeErrors int
	for remaining > 0 {
		batchSize := remaining
		if batchSize > 500 {
			batchSize = 500
		}
		msgs, err := consumer.Fetch(batchSize, jetstream.FetchMaxWait(10*time.Second))
		if err != nil {
			if errors.Is(err, jetstream.ErrNoMessages) {
				break
			}
			return nil, 0, err
		}
		fetched := 0
		for msg := range msgs.Messages() {
			fetched++
			var event corev1.Event
			if err := proto.Unmarshal(msg.Data(), &event); err != nil {
				decodeErrors++
				continue
			}
			eventType := events.EventTypeOf(&event)
			if eventType == "" {
				eventType = "unknown"
			}
			counts[eventType]++
		}
		if fetched == 0 {
			break
		}
		remaining -= fetched
	}
	return counts, decodeErrors, nil
}

func (c *ChattoCore) logESEventCounts(counts map[string]int) {
	if len(counts) == 0 {
		c.logger.Info("ES boot verification event counts", "counts", "none")
		return
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+fmt.Sprint(counts[key]))
	}
	c.logger.Info("ES boot verification event counts", "counts", strings.Join(parts, " "))
}

func countKVKeys(ctx context.Context, kv jetstream.KeyValue, filters ...string) (int, error) {
	var lister jetstream.KeyLister
	var err error
	if len(filters) == 0 {
		lister, err = kv.ListKeys(ctx)
	} else {
		lister, err = kv.ListKeysFiltered(ctx, filters...)
	}
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, nil
		}
		return 0, err
	}
	var count int
	for range lister.Keys() {
		count++
	}
	return count, nil
}

func countLegacyUserRecords(ctx context.Context, kv jetstream.KeyValue) (int, error) {
	lister, err := kv.ListKeysFiltered(ctx, "user.*")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, nil
		}
		return 0, err
	}
	var count int
	for key := range lister.Keys() {
		if len(strings.Split(key, ".")) == 2 {
			count++
		}
	}
	return count, nil
}

func kvKeyExists(ctx context.Context, kv jetstream.KeyValue, key string) (bool, error) {
	_, err := kv.Get(ctx, key)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, nil
	}
	return false, err
}

func countStreamMessages(ctx context.Context, stream jetstream.Stream, filters []string) (int, error) {
	consumer, err := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubjects:    filters,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 30 * time.Second,
	})
	if err != nil {
		return 0, err
	}
	defer stream.DeleteConsumer(context.Background(), consumer.CachedInfo().Name)

	info, err := consumer.Info(ctx)
	if err != nil {
		return 0, err
	}
	return int(info.NumPending), nil
}
