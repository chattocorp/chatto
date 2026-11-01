package chatto

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
)

// ServerProfile is the server's configured display identity.
type ServerProfile struct {
	Name string `json:"name"`
}

// ServerProfile reads the authenticated public server profile.
func (c *Client) ServerProfile(ctx context.Context) (ServerProfile, error) {
	var out struct {
		Profile ServerProfile `json:"profile"`
	}
	err := c.call(ctx, "ServerService/GetServerProfile", struct{}{}, &out)
	return out.Profile, err
}

// Room is public directory metadata. Kind is the protocol enum name.
type Room struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Archived    bool   `json:"archived"`
}

// RoomEntry includes the viewer's membership, attention and DM participants.
type RoomEntry struct {
	Room        Room `json:"room"`
	ViewerState struct {
		IsMember  bool `json:"isMember"`
		HasUnread bool `json:"hasUnread"`
	} `json:"viewerState"`
	MemberUserIDs []string `json:"memberUserIds"`
}

// RoomPage is a live offset page, not a stable snapshot.
type RoomPage struct {
	Rooms []RoomEntry `json:"rooms"`
	Page  struct {
		HasMore bool `json:"hasMore"`
	} `json:"page"`
}

// RoomGroup preserves the server's sidebar order. Items can contain a room or
// a sidebar link; clients that do not support links can skip those entries.
type RoomGroup struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Items []RoomGroupItem `json:"items"`
}

// RoomGroupItem is one entry in the ordered room-group navigation snapshot.
type RoomGroupItem struct {
	Room        *RoomEntry   `json:"room,omitempty"`
	SidebarLink *SidebarLink `json:"sidebarLink,omitempty"`
}

// SidebarLink is navigation metadata, not an instruction to open its URL.
type SidebarLink struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	URL   string `json:"url"`
}

// RoomGroups reads the finite, viewer-filtered channel navigation snapshot.
// Both groups and their items retain server order; DMs come from Rooms instead.
func (c *Client) RoomGroups(ctx context.Context) ([]RoomGroup, error) {
	var out struct {
		Groups []RoomGroup `json:"groups"`
	}
	err := c.call(ctx, "RoomDirectoryService/ListRoomGroups", struct{}{}, &out)
	return out.Groups, err
}

// Rooms lists up to 100 visible rooms starting at offset. Follow HasMore; room
// membership may change between pages, so consumers must deduplicate by ID.
func (c *Client) Rooms(ctx context.Context, offset int) (RoomPage, error) {
	return c.ListRooms(ctx, offset, 100)
}

// ListRooms selects the offset and page size. A zero limit uses the server
// default. Results retain protocol metadata; callers choose display filters.
func (c *Client) ListRooms(ctx context.Context, offset, limit int) (RoomPage, error) {
	var out RoomPage
	if offset < 0 || limit < 0 {
		return out, errors.New("negative room pagination value")
	}
	err := c.call(ctx, "RoomDirectoryService/ListRooms", map[string]any{"page": map[string]int{"offset": offset, "limit": limit}}, &out)
	return out, err
}

// TimelinePage contains chronological messages and opaque paging cursors.
// Non-message lifecycle events are omitted. Users hydrate author labels.
type TimelinePage struct {
	Messages               []Message
	Users                  map[string]Profile
	StartCursor, EndCursor string
	HasOlder, HasNewer     bool
}

// Timeline reads recent room messages, or a thread when root is nonempty.
// Pass the prior StartCursor as before to read older history in the same timeline.
func (c *Client) Timeline(ctx context.Context, room, root, before string) (TimelinePage, error) {
	return c.TimelineWithOptions(ctx, room, root, TimelineOptions{Before: before})
}

// TimelineOptions selects one cursor direction and page size. Zero Limit uses
// the server default. Cursors are opaque and belong to their source timeline.
type TimelineOptions struct {
	Before, After string
	Limit         int
}

func (o TimelineOptions) validate() error {
	if o.Before != "" && o.After != "" {
		return errors.New("choose before or after, not both")
	}
	if o.Limit < 0 || o.Limit > 500 {
		return errors.New("timeline limit must be between 0 and 500")
	}
	return nil
}

// EventPage preserves all protocol timeline events and included resources,
// including event kinds this SDK does not yet model. It applies no UI filter.
type EventPage struct {
	Events      []json.RawMessage `json:"events"`
	Includes    json.RawMessage   `json:"includes"`
	StartCursor string            `json:"startCursor"`
	EndCursor   string            `json:"endCursor"`
	HasOlder    bool              `json:"hasOlder"`
	HasNewer    bool              `json:"hasNewer"`
}

// TimelineEvents reads room or thread events without a message-only projection.
func (c *Client) TimelineEvents(ctx context.Context, room, root string, options TimelineOptions) (EventPage, error) {
	if err := options.validate(); err != nil {
		return EventPage{}, err
	}
	input := map[string]any{"roomId": room}
	if options.Limit != 0 {
		input["limit"] = options.Limit
	}
	method := "RoomService/GetRoomEvents"
	if root != "" {
		input["threadRootEventId"] = root
		method = "ThreadService/GetThreadEvents"
	}
	if options.Before != "" {
		input["before"] = options.Before
	}
	if options.After != "" {
		input["after"] = options.After
	}
	var out struct {
		Page *EventPage `json:"page"`
	}
	if err := c.call(ctx, method, input, &out); err != nil {
		return EventPage{}, err
	}
	if out.Page == nil {
		return EventPage{}, errors.New("missing timeline page")
	}
	return *out.Page, nil
}

// TimelineWithOptions projects live messages and author profiles from an event
// page. Use TimelineEvents when lifecycle or unknown events are needed.
func (c *Client) TimelineWithOptions(ctx context.Context, room, root string, options TimelineOptions) (TimelinePage, error) {
	raw, err := c.TimelineEvents(ctx, room, root, options)
	if err != nil {
		return TimelinePage{}, err
	}
	var includes struct {
		Users map[string]Profile `json:"users"`
	}
	if len(raw.Includes) > 0 {
		if err := json.Unmarshal(raw.Includes, &includes); err != nil {
			return TimelinePage{}, err
		}
	}
	p := TimelinePage{StartCursor: raw.StartCursor, EndCursor: raw.EndCursor, HasOlder: raw.HasOlder, HasNewer: raw.HasNewer, Users: includes.Users}
	for _, data := range raw.Events {
		var e struct {
			CreatedAt     string `json:"createdAt"`
			MessagePosted *struct {
				Message Message `json:"message"`
			} `json:"messagePosted"`
		}
		if err := json.Unmarshal(data, &e); err != nil {
			return TimelinePage{}, err
		}
		if e.MessagePosted != nil && e.MessagePosted.Message.DeletedAt == "" {
			m := e.MessagePosted.Message
			if m.CreatedAt == "" {
				m.CreatedAt = e.CreatedAt
			}
			p.Messages = append(p.Messages, m)
		}
	}
	return p, nil
}

// Profiles resolves public user profiles in one batch of at most 100 IDs.
func (c *Client) Profiles(ctx context.Context, ids []string) (map[string]Profile, error) {
	out := map[string]Profile{}
	if len(ids) == 0 {
		return out, nil
	}
	if len(ids) > 100 {
		return nil, errors.New("at most 100 user IDs per batch")
	}
	var response struct {
		Users []struct {
			Profile Profile `json:"user"`
		} `json:"users"`
	}
	if err := c.call(ctx, "UserService/BatchGetUsers", map[string]any{"userIds": ids}, &response); err != nil {
		return nil, err
	}
	for _, u := range response.Users {
		out[u.Profile.ID] = u.Profile
	}
	return out, nil
}

func uuidString(b []byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// NewMessageKey creates a UUID to retain with a pending send. Use the same key
// only for an exact retry of the same message after an uncertain result.
func NewMessageKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return uuidString(b)
}
