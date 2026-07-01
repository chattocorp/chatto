package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	MentionHandleAll  = "all"
	MentionHandleHere = "here"

	LargeMentionNotificationThreshold = 10
)

// MentionConfirmationRequiredError is returned when a message would notify a
// large number of people and the caller has not explicitly confirmed the send.
type MentionConfirmationRequiredError struct {
	RecipientCount int
}

func (e *MentionConfirmationRequiredError) Error() string {
	return fmt.Sprintf("mention confirmation required for %d recipients", e.RecipientCount)
}

// IsVirtualMentionHandle reports whether a handle is owned by Chatto rather
// than by a user or role. Handles are matched case-insensitively.
func IsVirtualMentionHandle(handle string) bool {
	switch strings.ToLower(handle) {
	case MentionHandleAll, MentionHandleHere:
		return true
	default:
		return false
	}
}

func (c *ChattoCore) loginConflictsWithMentionHandle(login string) bool {
	normalized := strings.ToLower(login)
	return IsVirtualMentionHandle(normalized) || c.RBAC.RoleExists(normalized)
}

func (c *ChattoCore) roleNameConflictsWithMentionHandle(roleName string) bool {
	normalized := strings.ToLower(roleName)
	if IsVirtualMentionHandle(normalized) {
		return true
	}
	return c.Users.LoginExists(roleName)
}

func (c *ChattoCore) requireLoginMentionHandleAvailable(login string) error {
	availability := c.mentionables.Availability(login, nil)
	if availability.Available {
		return nil
	}
	if availability.OwnerKind == mentionableOwnerUser {
		return ErrLoginAlreadyTaken
	}
	return ErrUsernameBlocked
}

func (c *ChattoCore) requireRoleMentionHandleAvailable(roleName string) error {
	if c.mentionables.Availability(roleName, nil).Available {
		return nil
	}
	return ErrRoleAlreadyExists
}

var mentionNodeKind = ast.NewNodeKind("Mention")

type mentionNode struct {
	ast.BaseInline
	Username string
}

func (n *mentionNode) Kind() ast.NodeKind {
	return mentionNodeKind
}

func (n *mentionNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Username": n.Username,
	}, nil)
}

type mentionInlineParser struct{}

func (p mentionInlineParser) Trigger() []byte {
	return []byte{'@'}
}

func (p mentionInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, segment := block.PeekLine()
	if len(line) < 2 || line[0] != '@' {
		return nil
	}

	source := block.Source()
	if segment.Start > 0 && isMentionAlphanumeric(source[segment.Start-1]) {
		return nil
	}

	stop := 1
	for stop < len(line) && isMentionHandleChar(line[stop]) {
		stop++
	}
	if stop == 1 {
		return nil
	}
	for stop < len(line) && line[stop] == '.' {
		next := stop + 1
		if next >= len(line) || !isMentionHandleChar(line[next]) {
			break
		}
		stop = next + 1
		for stop < len(line) && isMentionHandleChar(line[stop]) {
			stop++
		}
	}

	username := string(line[1:stop])
	block.Advance(stop)
	return &mentionNode{Username: username}
}

func isMentionAlphanumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func isMentionHandleChar(c byte) bool {
	return isMentionAlphanumeric(c) || c == '_' || c == '-'
}

var mentionMarkdown = goldmark.New(
	goldmark.WithParser(parser.NewParser(
		parser.WithBlockParsers(
			util.Prioritized(parser.NewSetextHeadingParser(), 100),
			util.Prioritized(parser.NewThematicBreakParser(), 200),
			util.Prioritized(parser.NewListParser(), 300),
			util.Prioritized(parser.NewListItemParser(), 400),
			util.Prioritized(parser.NewCodeBlockParser(), 500),
			util.Prioritized(parser.NewATXHeadingParser(), 600),
			util.Prioritized(parser.NewFencedCodeBlockParser(), 700),
			util.Prioritized(parser.NewBlockquoteParser(), 800),
			util.Prioritized(parser.NewParagraphParser(), 1000),
		),
		parser.WithInlineParsers(
			util.Prioritized(parser.NewCodeSpanParser(), 100),
			util.Prioritized(parser.NewLinkParser(), 200),
			util.Prioritized(parser.NewAutoLinkParser(), 300),
			util.Prioritized(mentionInlineParser{}, 400),
			util.Prioritized(parser.NewEmphasisParser(), 500),
		),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)),
)

func mentionMarkdownSource(body string) string {
	// Chatto's message renderer disables Markdown backslash escapes, so
	// \` still participates in code-span parsing and \@alice still contains
	// a visible mention boundary. Goldmark's inline loop hardcodes backslash
	// escaping, so normalize just those cases for mention extraction.
	body = strings.ReplaceAll(body, "\\`", "`")
	return strings.ReplaceAll(body, "\\@", "\\\\@")
}

// ExtractMentionUsernames extracts all unique @username mentions from a message body.
// Returns a slice of usernames (without the @ prefix) in the order they appear.
// Duplicate mentions are deduplicated. Mentions inside Markdown code spans,
// code blocks, and blockquotes are ignored.
func ExtractMentionUsernames(body string) []string {
	if !strings.Contains(body, "@") {
		return nil
	}

	// Deduplicate while preserving order
	seen := make(map[string]bool)
	var usernames []string

	add := func(username string) {
		if username == "" {
			return
		}
		if seen[username] {
			return
		}
		seen[username] = true
		usernames = append(usernames, username)
	}

	source := []byte(mentionMarkdownSource(body))
	root := mentionMarkdown.Parser().Parse(text.NewReader(source))
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node.Kind() {
		case ast.KindCodeBlock, ast.KindFencedCodeBlock, ast.KindBlockquote:
			return ast.WalkSkipChildren, nil
		case mentionNodeKind:
			add(node.(*mentionNode).Username)
		}

		return ast.WalkContinue, nil
	})

	return usernames
}

// ResolveMentions takes a list of usernames and resolves them to user IDs.
// Invalid usernames are silently ignored.
// Returns a slice of valid user IDs.
func (c *ChattoCore) ResolveMentions(ctx context.Context, usernames []string) ([]string, error) {
	if len(usernames) == 0 {
		return nil, nil
	}

	var userIDs []string
	for _, username := range usernames {
		// Look up user by login (case-insensitive). Every authenticated user
		// is implicitly a server member post-#330, so no further gate.
		user, err := c.GetUserByLogin(ctx, username)
		if err != nil {
			continue
		}

		userIDs = append(userIDs, user.Id)
	}

	return userIDs, nil
}

// ResolveRoomMentions resolves @handles in a message to concrete room-member
// user IDs. Handles share one namespace across users, roles, and virtual
// room-scoped broadcasts:
//   - @all: every current room member
//   - @here: current room members whose presence is not OFFLINE
//   - @pingable-role: current room members explicitly assigned that role
//   - @user: that user, if they are a current room member
//
// Invalid handles are silently ignored, matching existing @user behavior.
func (c *ChattoCore) ResolveRoomMentions(ctx context.Context, kind RoomKind, roomID string, handles []string) ([]string, error) {
	if len(handles) == 0 {
		return nil, nil
	}

	members, err := c.GetRoomMembersList(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	roomMembers := make(map[string]struct{}, len(members))
	for _, member := range members {
		if member != nil && member.UserId != "" {
			roomMembers[member.UserId] = struct{}{}
		}
	}

	seen := make(map[string]struct{})
	userIDs := make([]string, 0, len(handles))
	add := func(userID string) {
		if userID == "" {
			return
		}
		if _, ok := roomMembers[userID]; !ok {
			return
		}
		if _, ok := seen[userID]; ok {
			return
		}
		seen[userID] = struct{}{}
		userIDs = append(userIDs, userID)
	}
	addMembers := func(candidates []string) {
		for _, userID := range candidates {
			add(userID)
		}
	}

	for _, handle := range handles {
		normalized := strings.ToLower(handle)
		switch normalized {
		case MentionHandleAll:
			for _, member := range members {
				if member != nil {
					add(member.UserId)
				}
			}
			continue
		case MentionHandleHere:
			for _, member := range members {
				if member == nil {
					continue
				}
				status, err := c.GetUserPresence(ctx, member.UserId)
				if err != nil {
					c.logger.Warn("Failed to get presence for @here mention",
						"user_id", member.UserId,
						"room_id", roomID,
						"error", err)
					continue
				}
				if status != PresenceStatusOffline {
					add(member.UserId)
				}
			}
			continue
		case RoleEveryone:
			// The implicit RBAC everyone role is intentionally not a mention
			// handle. Use @all for room-wide broadcast semantics.
			continue
		}

		if role, ok := c.RBAC.GetRole(normalized); ok {
			if !role.GetPingable() {
				continue
			}
			roleUsers, err := c.GetRoleUsers(ctx, normalized)
			if err != nil {
				if err != ErrRoleNotFound {
					c.logger.Warn("Failed to resolve role mention",
						"role", normalized,
						"room_id", roomID,
						"error", err)
				}
				continue
			}
			addMembers(roleUsers)
			continue
		}

		user, err := c.GetUserByLogin(ctx, handle)
		if err != nil {
			continue
		}
		add(user.Id)
	}

	return userIDs, nil
}

// ResolveDirectRoomMentions resolves only direct @user handles to room-member
// user IDs. Role and virtual broadcast handles are intentionally ignored.
func (c *ChattoCore) ResolveDirectRoomMentions(ctx context.Context, kind RoomKind, roomID string, handles []string) ([]string, error) {
	if len(handles) == 0 {
		return nil, nil
	}

	members, err := c.GetRoomMembersList(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	roomMembers := make(map[string]struct{}, len(members))
	for _, member := range members {
		if member != nil && member.UserId != "" {
			roomMembers[member.UserId] = struct{}{}
		}
	}

	seen := make(map[string]struct{})
	userIDs := make([]string, 0, len(handles))
	for _, handle := range handles {
		normalized := strings.ToLower(handle)
		if IsVirtualMentionHandle(normalized) || normalized == RoleEveryone {
			continue
		}
		if _, ok := c.RBAC.GetRole(normalized); ok {
			continue
		}

		user, err := c.GetUserByLogin(ctx, handle)
		if err != nil {
			continue
		}
		if _, ok := roomMembers[user.Id]; !ok {
			continue
		}
		if _, ok := seen[user.Id]; ok {
			continue
		}
		seen[user.Id] = struct{}{}
		userIDs = append(userIDs, user.Id)
	}

	return userIDs, nil
}

func (c *ChattoCore) mentionNotificationRecipientCount(ctx context.Context, roomID, authorID string, mentionedUserIDs []string) (int, error) {
	count := 0
	for _, mentionedUserID := range mentionedUserIDs {
		if mentionedUserID == "" || mentionedUserID == authorID {
			continue
		}
		level, err := c.GetEffectiveNotificationLevel(ctx, mentionedUserID, roomID)
		if err != nil {
			return 0, err
		}
		if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			continue
		}
		count++
	}
	return count, nil
}

// MentionNotificationRecipientCountForBody returns how many people would
// receive a notification from the @mentions in body after room membership,
// author/self, dedupe, and room-mute filtering are applied.
func (c *ChattoCore) MentionNotificationRecipientCountForBody(ctx context.Context, kind RoomKind, roomID, authorID, body string) (int, error) {
	handles := ExtractMentionUsernames(body)
	mentionedUserIDs, err := c.ResolveRoomMentions(ctx, kind, roomID, handles)
	if err != nil {
		return 0, err
	}
	return c.mentionNotificationRecipientCount(ctx, roomID, authorID, mentionedUserIDs)
}

// notifyMentionedUsers creates persistent notifications for all mentioned users.
// This is best-effort - failures are logged but don't affect message posting.
//
// inThread is the thread root event ID when the mention is on a message inside
// a thread, or empty string for room-level messages. The frontend uses this to
// route notification clicks directly into the thread pane.
func (c *ChattoCore) notifyMentionedUsers(ctx context.Context, kind RoomKind, roomID, authorID, eventID, inThread string, mentionedUserIDs, directMentionedUserIDs []string) []string {
	directMentioned := make(map[string]struct{}, len(directMentionedUserIDs))
	for _, userID := range directMentionedUserIDs {
		if userID != "" {
			directMentioned[userID] = struct{}{}
		}
	}

	var newlyAutoFollowed []string
	for _, mentionedUserID := range mentionedUserIDs {
		// Don't notify the author if they mentioned themselves
		if mentionedUserID == authorID {
			continue
		}

		// Skip if user has muted this room
		level, err := c.GetEffectiveNotificationLevel(ctx, mentionedUserID, roomID)
		if err != nil {
			c.logger.Warn("Failed to get notification level for mention check, continuing",
				"user_id", mentionedUserID, "error", err)
		} else if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			continue
		}
		// Create persistent notification (for bell icon and notification center)
		// This also publishes NotificationCreatedEvent for real-time updates
		created, createErr := c.CreateNotification(ctx, mentionedUserID, authorID, &corev1.Notification{
			Notification: &corev1.Notification_Mention{
				Mention: &corev1.MentionNotification{
					RoomId:   roomID,
					EventId:  eventID,
					InThread: inThread,
				},
			},
		})
		if createErr != nil {
			c.logger.Warn("Failed to create mention notification",
				"mentioned_user_id", mentionedUserID,
				"author_id", authorID,
				"kind", kind,
				"room_id", roomID,
				"error", createErr)
			continue
		}
		if created == nil {
			continue
		}
		if inThread != "" {
			if _, ok := directMentioned[mentionedUserID]; ok {
				followed, err := c.FollowThreadIfNeverSet(ctx, kind, mentionedUserID, roomID, inThread, corev1.ThreadFollowSource_THREAD_FOLLOW_SOURCE_DIRECT_MENTION)
				if err != nil {
					c.logger.Warn("Failed to auto-follow thread for mentioned user",
						"mentioned_user_id", mentionedUserID,
						"kind", kind,
						"room_id", roomID,
						"thread_root_event_id", inThread,
						"error", err)
				} else if followed {
					newlyAutoFollowed = append(newlyAutoFollowed, mentionedUserID)
					c.logger.Debug("Auto-followed thread for mentioned user",
						"mentioned_user_id", mentionedUserID,
						"kind", kind,
						"room_id", roomID,
						"thread_root_event_id", inThread)
				}
			}
		}
		if c.suppressesNotificationAlertsForPresence(ctx, mentionedUserID) {
			continue
		}

		// Publish live mention event for room-level indicator real-time update.
		// Clients hydrate space, room, and user names from projected reads.
		mentionEvent := newLiveEvent(authorID, &corev1.LiveEvent{
			Event: &corev1.LiveEvent_MentionNotification{
				MentionNotification: &corev1.MentionNotificationEvent{
					RoomId:            roomID,
					MentionedByUserId: authorID,
				},
			},
		})
		subject := subjects.LiveSyncUserEvent(mentionedUserID, "mentioned")
		if err := c.publishLiveEvent(ctx, subject, mentionEvent); err != nil {
			c.logger.Warn("Failed to publish mention live event",
				"mentioned_user_id", mentionedUserID,
				"error", err)
		}

		c.logger.Debug("Created mention notification",
			"mentioned_user_id", mentionedUserID,
			"author_id", authorID,
			"kind", kind,
			"room_id", roomID)
	}
	return newlyAutoFollowed
}
