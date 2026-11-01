// Package chatto provides an experimental Go client for Chatto's public HTTP and
// realtime APIs. It is independent of bot runtimes and server-internal types.
package chatto

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/chattocorp/chatto/experimental/chatto-go/internal/httpjson"
)

// Client authenticates each request with an API key or human bearer access token.
// Clients are immutable; create a new client after rotating a human session.
type Client struct {
	endpoint, token string
	allMessages     bool
	minimumCursor   string
}

// New accepts a server origin. Plain HTTP is allowed only on loopback hosts.
func New(origin, token string) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("Chatto URL must be a server origin")
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return nil, errors.New("Chatto requires HTTPS except on loopback")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("Chatto API key is required")
	}
	return &Client{endpoint: strings.TrimRight(origin, "/") + "/api/connect/chatto.api.v1.", token: token}, nil
}

// Message is the text, attachment, and identity subset of a renderable message.
type Message struct {
	ID                string       `json:"id"`
	RoomID            string       `json:"roomId"`
	ActorID           string       `json:"actorId"`
	Body              string       `json:"body"`
	ThreadRootEventID string       `json:"threadRootEventId"`
	DeletedAt         string       `json:"deletedAt,omitempty"`
	CreatedAt         string       `json:"createdAt,omitempty"`
	Attachments       []Attachment `json:"attachments,omitempty"`
}

// ThreadRootID returns the thread containing this message. A top-level message
// is its own root, including before the first reply establishes a thread.
func (m Message) ThreadRootID() string {
	if m.ThreadRootEventID != "" {
		return m.ThreadRootEventID
	}
	return m.ID
}

// Page contains messages in chronological order and a server-owned cursor.
// HasOlder reports omitted history; EndCursor must remain opaque.
type Page struct {
	Messages           []Message
	EndCursor          string
	HasOlder, HasNewer bool
}

// Profile is the public identity subset needed to identify a Chatto account.
// Login and DisplayName are profile data, not instructions, and must not be logged.
type Profile struct {
	ID          string `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"displayName"`
}

// Viewer returns the authenticated account's profile, including for bot API keys.
func (c *Client) Viewer(ctx context.Context) (Profile, error) {
	var out struct {
		User struct {
			Profile Profile `json:"profile"`
		} `json:"user"`
	}
	err := c.call(ctx, "ViewerService/GetViewer", struct{}{}, &out)
	if err != nil {
		return Profile{}, err
	}
	if out.User.Profile.ID == "" {
		return Profile{}, errors.New("missing viewer identity")
	}
	return out.User.Profile, nil
}

// ViewerID returns the authenticated identity, including when the key belongs to a bot.
func (c *Client) ViewerID(ctx context.Context) (string, error) {
	profile, err := c.Viewer(ctx)
	return profile.ID, err
}

// Thread reads up to 50 replies. An empty cursor reads recent history and the root;
// a nonempty cursor reads later events without repeating the root.
func (c *Client) Thread(ctx context.Context, room, root, after string) (Page, error) {
	input := struct {
		Room  string `json:"roomId"`
		Root  string `json:"threadRootEventId"`
		Limit int    `json:"limit"`
		After string `json:"after,omitempty"`
	}{room, root, 50, after}
	var out struct {
		Page *struct {
			Events []struct {
				Posted *struct {
					Message Message `json:"message"`
				} `json:"messagePosted"`
			} `json:"events"`
			EndCursor string `json:"endCursor"`
			HasOlder  bool   `json:"hasOlder"`
			HasNewer  bool   `json:"hasNewer"`
		} `json:"page"`
	}
	if err := c.call(ctx, "ThreadService/GetThreadEvents", input, &out); err != nil {
		return Page{}, err
	}
	if out.Page == nil {
		return Page{}, errors.New("missing thread page")
	}
	p := Page{EndCursor: out.Page.EndCursor, HasOlder: out.Page.HasOlder, HasNewer: out.Page.HasNewer}
	for _, event := range out.Page.Events {
		if event.Posted != nil && event.Posted.Message.DeletedAt == "" {
			p.Messages = append(p.Messages, event.Posted.Message)
		}
	}
	return p, nil
}

// Reply posts once to the supplied thread. Supply a UUID to allow an exact retry
// within the server's idempotency window after an uncertain result. An empty key
// generates a fresh UUID for this call; do not retry that call automatically.
func (c *Client) Reply(ctx context.Context, room, root, replyTo, body, key string) error {
	if strings.TrimSpace(body) == "" {
		return errors.New("empty reply")
	}
	if key == "" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return errors.New("generate message key")
		}
		id[6] = (id[6] & 0x0f) | 0x40
		id[8] = (id[8] & 0x3f) | 0x80
		key = fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
	}
	var out struct {
		Message *Message `json:"message"`
	}
	err := c.call(ctx, "MessageService/CreateMessage", map[string]string{
		"roomId": room, "threadRootEventId": root, "inReplyTo": replyTo,
		"body": body, "idempotencyKey": key,
	}, &out)
	if err != nil {
		return err
	}
	if out.Message == nil || out.Message.ID == "" {
		return errors.New("missing created message")
	}
	return nil
}

func (c *Client) call(ctx context.Context, method string, input, output any) error {
	return httpjson.PostAtCursor(ctx, c.endpoint+method, c.token, c.minimumCursor, input, output)
}

// RefreshTypingIndicator publishes live typing activity for the authenticated
// user. Pass an empty root for the room timeline, or a thread-root event ID.
// Refresh at most once every two seconds. Stop refreshing when work ends;
// receivers clear typing after a matching message or six seconds of inactivity.
func (c *Client) RefreshTypingIndicator(ctx context.Context, room, root string) error {
	return c.call(ctx, "RoomService/RefreshTypingIndicator", map[string]string{
		"roomId": room, "threadRootEventId": root,
	}, &struct{}{})
}

// React adds the authenticated user's reaction to a message. Chatto expects an emoji
// name such as "thumbsup", without surrounding colons, rather than a glyph.
func (c *Client) React(ctx context.Context, room, message, emoji string) error {
	var out struct {
		Added bool `json:"added"`
	}
	return c.call(ctx, "MessageService/AddReaction", map[string]string{
		"roomId": room, "messageEventId": message, "emoji": emoji,
	}, &out)
}
