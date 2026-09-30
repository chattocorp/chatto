package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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

// scanMentions finds the @handle mentions in the text run source[start:stop]
// and calls add with each handle and the offset of its '@'. An '@' directly
// after a letter or digit, as in user@example.com, does not start a mention.
// A handle contains letters, digits, '_', and '-'; a '.' is allowed only
// between two handle characters. A handle ends at the end of the run, so
// Markdown syntax such as a closing emphasis delimiter is never part of it.
func scanMentions(source []byte, start, stop int, add func(handle string, offset int)) {
	for at := start; at < stop; at++ {
		if source[at] != '@' || (at > 0 && isMentionAlphanumeric(source[at-1])) {
			continue
		}

		end := at + 1
		for end < stop && isMentionHandleChar(source[end]) {
			end++
		}
		if end == at+1 {
			continue
		}
		for end+1 < stop && source[end] == '.' && isMentionHandleChar(source[end+1]) {
			end += 2
			for end < stop && isMentionHandleChar(source[end]) {
				end++
			}
		}

		add(string(source[at+1:end]), at)
		at = end - 1
	}
}

func isMentionAlphanumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func isMentionHandleChar(c byte) bool {
	return isMentionAlphanumeric(c) || c == '_' || c == '-'
}

var mentionMarkdown = goldmark.New(
	goldmark.WithParser(parser.NewParser(
		// Only the block syntax that the frontend renders: it disables
		// setext headings and thematic breaks, so a "---" line cannot end
		// a blockquote there.
		parser.WithBlockParsers(
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
			util.Prioritized(parser.NewEmphasisParser(), 500),
		),
		// No paragraph transformers: the frontend disables link reference
		// definitions, so [@alice]: url stays text with a mention there.
	)),
)

func mentionMarkdownSource(body string) string {
	// Chatto's message renderer disables Markdown backslash escapes in text,
	// so a backslash does not change emphasis, links, or code spans there.
	// Goldmark's inline loop hardcodes backslash escaping; doubling every
	// backslash makes goldmark read each original backslash as a literal
	// character. markdown-it still honors escapes inside link destinations
	// and titles; FDR-006 decision 10 accepts that rare difference.
	return strings.ReplaceAll(body, "\\", "\\\\")
}

// ExtractMentionUsernames extracts all unique @username mentions from a message body.
// Returns a slice of usernames (without the @ prefix) in the order they appear.
// Duplicate mentions are deduplicated. Mentions inside Markdown code spans,
// code blocks, blockquotes, links, and URLs are ignored.
//
// The bundled frontend renders mentions with the same rules (see
// apps/frontend/src/lib/markdownMentions.ts), so a message notifies exactly
// the handles that it shows as mentions. The shared cases in
// testdata/mentions/extraction.json test both implementations.
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
	urls := newMentionURLIndex(source)
	root := mentionMarkdown.Parser().Parse(text.NewReader(source))

	// Mentions are found after inline parsing, in runs of adjacent text
	// nodes, like the frontend's markdown-it rule. Goldmark splits text at
	// unmatched delimiters such as '_', so adjacent text segments are joined
	// before scanning. Every other node ends the current run.
	runStart, runStop := -1, -1
	flush := func() {
		if runStart >= 0 {
			scanMentions(source, runStart, runStop, func(handle string, offset int) {
				if !urls.contains(offset) {
					add(handle)
				}
			})
		}
		runStart, runStop = -1, -1
	}
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		switch node.Kind() {
		case ast.KindText:
			if entering {
				segment := node.(*ast.Text).Segment
				if segment.Start != runStop {
					flush()
					runStart = segment.Start
				}
				runStop = segment.Stop
			}
			return ast.WalkContinue, nil
		// The frontend disables images, so ![label](url) renders as "!"
		// and a link: the label is link text there too.
		case ast.KindCodeBlock, ast.KindFencedCodeBlock, ast.KindBlockquote, ast.KindCodeSpan,
			ast.KindLink, ast.KindAutoLink, ast.KindImage:
			flush()
			return ast.WalkSkipChildren, nil
		default:
			flush()
			return ast.WalkContinue, nil
		}
	})
	flush()

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

// RoomMentionResolution retains both the concrete recipients and each mention
// kind that selected them. This provenance is embedded in the durable message
// source fact so @here presence and overlapping handles are not re-evaluated
// later by notification materialization.
type RoomMentionResolution struct {
	RecipientIDs []string
	Mentions     []*evtv1.MessageMention
}

// ResolveRoomMentionKinds resolves @handles in a message to concrete
// room-member user IDs while retaining each typed mention's provenance.
// Handles share one namespace across users, roles, and virtual
// room-scoped broadcasts:
//   - @all: every current room member
//   - @here: current room members whose presence is not OFFLINE
//   - @pingable-role: current room members explicitly assigned that role
//   - @user: that user, if they are a current room member
//
// Invalid handles are silently ignored, matching existing @user behavior.
func (c *ChattoCore) ResolveRoomMentionKinds(ctx context.Context, kind RoomKind, roomID string, handles []string) (*RoomMentionResolution, error) {
	result := &RoomMentionResolution{}
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

	seenRecipients := make(map[string]struct{})
	seenMentions := make(map[string]struct{})
	add := func(userID string, causeKey string, mention *evtv1.MessageMention) {
		if userID == "" {
			return
		}
		if _, ok := roomMembers[userID]; !ok {
			return
		}
		if _, seen := seenRecipients[userID]; !seen {
			seenRecipients[userID] = struct{}{}
			result.RecipientIDs = append(result.RecipientIDs, userID)
		}
		mentionKey := userID + "\x00" + causeKey
		if _, duplicate := seenMentions[mentionKey]; duplicate {
			return
		}
		seenMentions[mentionKey] = struct{}{}
		mention.UserId = userID
		result.Mentions = append(result.Mentions, mention)
	}
	addMembers := func(candidates []string, causeKey string, cause func() *evtv1.MessageMention) {
		for _, userID := range candidates {
			add(userID, causeKey, cause())
		}
	}

	for _, handle := range handles {
		normalized := strings.ToLower(handle)
		switch normalized {
		case MentionHandleAll:
			for _, member := range members {
				if member != nil {
					add(member.UserId, "all", &evtv1.MessageMention{Cause: &evtv1.MessageMention_All{All: &evtv1.AllMessageMention{}}})
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
					add(member.UserId, "here", &evtv1.MessageMention{Cause: &evtv1.MessageMention_Here{Here: &evtv1.HereMessageMention{}}})
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
			addMembers(roleUsers, "role:"+normalized, func() *evtv1.MessageMention {
				return &evtv1.MessageMention{Cause: &evtv1.MessageMention_Role{Role: &evtv1.RoleMessageMention{RoleName: normalized}}}
			})
			continue
		}

		user, err := c.GetUserByLogin(ctx, handle)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("resolve user mention: %w", err)
		}
		add(user.Id, "direct", &evtv1.MessageMention{Cause: &evtv1.MessageMention_Direct{Direct: &evtv1.DirectUserMention{}}})
	}

	return result, nil
}

// ResolveRoomMentions is the compatibility view used by message rendering and
// legacy callers that only need the concrete recipient list.
func (c *ChattoCore) ResolveRoomMentions(ctx context.Context, kind RoomKind, roomID string, handles []string) ([]string, error) {
	resolved, err := c.ResolveRoomMentionKinds(ctx, kind, roomID, handles)
	if err != nil {
		return nil, err
	}
	return resolved.RecipientIDs, nil
}

// ResolveDirectRoomMentions resolves only direct @user handles to room-member
// user IDs. Role and virtual broadcast handles are intentionally ignored.
func (c *ChattoCore) ResolveDirectRoomMentions(ctx context.Context, kind RoomKind, roomID string, handles []string) ([]string, error) {
	resolved, err := c.ResolveRoomMentionKinds(ctx, kind, roomID, handles)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	for _, mention := range resolved.Mentions {
		if _, direct := mention.GetCause().(*evtv1.MessageMention_Direct); direct {
			seen[mention.GetUserId()] = struct{}{}
		}
	}
	userIDs := make([]string, 0, len(seen))
	for userID := range seen {
		userIDs = append(userIDs, userID)
	}
	sort.Strings(userIDs)
	return userIDs, nil
}
