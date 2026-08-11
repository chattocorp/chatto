package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	MentionHandleAll  = "all"
	MentionHandleHere = "here"
)

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
	return IsVirtualMentionHandle(normalized) || c.rbacModel.roleExists(normalized)
}

func (c *ChattoCore) roleNameConflictsWithMentionHandle(roleName string) bool {
	normalized := strings.ToLower(roleName)
	if IsVirtualMentionHandle(normalized) {
		return true
	}
	return c.userModel.loginExists(roleName)
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

// RoomMentionResolution retains both the concrete recipients and why each
// recipient matched. The reason provenance is embedded in the durable message
// source fact so @here presence and overlapping handles are not re-evaluated
// later by notification materialization.
type RoomMentionResolution struct {
	RecipientIDs  []string
	ReasonsByUser map[string][]corev1.NotificationReason
}

// ResolveRoomMentionReasons resolves @handles in a message to concrete
// room-member user IDs and distinct Notifications 2.0 causes. Handles share
// one namespace across users, roles, and virtual
// room-scoped broadcasts:
//   - @all: every current room member
//   - @here: current room members whose presence is not OFFLINE
//   - @pingable-role: current room members explicitly assigned that role
//   - @user: that user, if they are a current room member
//
// Invalid handles are silently ignored, matching existing @user behavior.
func (c *ChattoCore) ResolveRoomMentionReasons(ctx context.Context, kind RoomKind, roomID string, handles []string) (*RoomMentionResolution, error) {
	result := &RoomMentionResolution{ReasonsByUser: make(map[string][]corev1.NotificationReason)}
	if len(handles) == 0 {
		return result, nil
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

	seen := make(map[string]map[corev1.NotificationReason]struct{})
	add := func(userID string, reason corev1.NotificationReason) {
		if userID == "" {
			return
		}
		if _, ok := roomMembers[userID]; !ok {
			return
		}
		if seen[userID] == nil {
			seen[userID] = make(map[corev1.NotificationReason]struct{})
			result.RecipientIDs = append(result.RecipientIDs, userID)
		}
		if _, duplicate := seen[userID][reason]; duplicate {
			return
		}
		seen[userID][reason] = struct{}{}
		result.ReasonsByUser[userID] = append(result.ReasonsByUser[userID], reason)
	}
	addMembers := func(candidates []string, reason corev1.NotificationReason) {
		for _, userID := range candidates {
			add(userID, reason)
		}
	}

	for _, handle := range handles {
		normalized := strings.ToLower(handle)
		switch normalized {
		case MentionHandleAll:
			for _, member := range members {
				if member != nil {
					add(member.UserId, corev1.NotificationReason_NOTIFICATION_REASON_ALL)
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
					return nil, fmt.Errorf("resolve @here presence: %w", err)
				}
				if status != PresenceStatusOffline {
					add(member.UserId, corev1.NotificationReason_NOTIFICATION_REASON_HERE)
				}
			}
			continue
		case RoleEveryone:
			// The implicit RBAC everyone role is intentionally not a mention
			// handle. Use @all for room-wide broadcast semantics.
			continue
		}

		if role, ok := c.rbacModel.role(normalized); ok {
			if !role.GetPingable() {
				continue
			}
			roleUsers, err := c.GetRoleUsers(ctx, normalized)
			if err != nil {
				if errors.Is(err, ErrRoleNotFound) {
					continue
				}
				return nil, fmt.Errorf("resolve role mention: %w", err)
			}
			addMembers(roleUsers, corev1.NotificationReason_NOTIFICATION_REASON_ROLE_MENTION)
			continue
		}

		user, err := c.GetUserByLogin(ctx, handle)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("resolve user mention: %w", err)
		}
		add(user.Id, corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION)
	}

	return result, nil
}

// ResolveRoomMentions is the compatibility view used by message rendering and
// legacy callers that only need the concrete recipient list.
func (c *ChattoCore) ResolveRoomMentions(ctx context.Context, kind RoomKind, roomID string, handles []string) ([]string, error) {
	resolved, err := c.ResolveRoomMentionReasons(ctx, kind, roomID, handles)
	if err != nil {
		return nil, err
	}
	return resolved.RecipientIDs, nil
}

// ResolveDirectRoomMentions resolves only direct @user handles to room-member
// user IDs. Role and virtual broadcast handles are intentionally ignored.
func (c *ChattoCore) ResolveDirectRoomMentions(ctx context.Context, kind RoomKind, roomID string, handles []string) ([]string, error) {
	resolved, err := c.ResolveRoomMentionReasons(ctx, kind, roomID, handles)
	if err != nil {
		return nil, err
	}
	userIDs := make([]string, 0, len(resolved.RecipientIDs))
	for _, userID := range resolved.RecipientIDs {
		for _, reason := range resolved.ReasonsByUser[userID] {
			if reason == corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION {
				userIDs = append(userIDs, userID)
				break
			}
		}
	}

	return userIDs, nil
}
