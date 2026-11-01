package chatto

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/chattocorp/chatto/experimental/chatto-go/internal/httpjson"
	"github.com/chattocorp/chatto/experimental/chatto-go/internal/wire"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

var errResnapshot = errors.New("server state requires a new snapshot")

func (s *Server) reset() {
	s.mu.Lock()
	s.state = emptyState("connecting", s.state.Generation+1)
	s.cursor = ""
	s.revision++
	s.mu.Unlock()
	s.notify()
}
func (s *Server) status(status string, err error) {
	s.mu.Lock()
	s.state.Status = status
	s.state.Error = err
	s.mu.Unlock()
	s.notify()
}

func (s *Server) run() {
	defer close(s.done)
	delay := time.Second
	renew := false
	for s.ctx.Err() == nil {
		c, err := s.client(s.ctx, renew)
		renew = false
		if err != nil {
			if s.ctx.Err() != nil {
				break
			}
			s.reset()
			s.status("failed", err)
			return
		}
		viewer, err := c.Viewer(s.ctx)
		retry := true
		var statusError httpjson.StatusError
		if errors.As(err, &statusError) && (statusError.Code == 401 || statusError.Code == 403) {
			retry = false
		}
		var hint time.Duration
		if err == nil {
			s.mu.Lock()
			previous := s.viewerID
			if previous == "" {
				s.viewerID = viewer.ID
			}
			s.mu.Unlock()
			if previous != "" && previous != viewer.ID {
				s.reset()
				s.status("failed", errors.New("server account changed"))
				return
			}
			retry, renew, hint, err = s.stream(c, viewer)
		}
		if s.ctx.Err() != nil {
			break
		}
		if !retry {
			s.reset()
			s.status("failed", err)
			return
		}
		if errors.Is(err, errResnapshot) {
			s.reset()
			delay = time.Second
			continue
		}
		s.mu.Lock()
		connected := s.state.Status == "connected"
		s.mu.Unlock()
		if connected {
			delay = time.Second
		}
		s.status("reconnecting", err)
		wait := max(hint, delay+time.Duration(rand.Int64N(int64(delay/2)+1)))
		timer := time.NewTimer(wait)
		select {
		case <-s.ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
		delay = min(30*time.Second, delay*2)
	}
	s.reset()
	s.status("closed", nil)
}

// Snapshot and replay frames are staged until caught_up. A failed handshake
// leaves the previous committed cursor intact, never publishing half a snapshot.
func (s *Server) stream(c *Client, viewer Profile) (retry, renew bool, hint time.Duration, result error) {
	origin := strings.TrimSuffix(c.endpoint, "/api/connect/chatto.api.v1.")
	endpoint := "ws" + strings.TrimPrefix(origin, "http") + "/api/realtime"
	conn, response, err := (&websocket.Dialer{HandshakeTimeout: 10 * time.Second}).DialContext(s.ctx, endpoint, nil)
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		retry = response == nil || response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests
		return retry, false, 0, errors.New("connect server realtime")
	}
	defer conn.Close()
	stop := context.AfterFunc(s.ctx, func() { conn.Close() })
	defer stop()
	conn.SetReadLimit(16 << 20)
	s.mu.Lock()
	cursor := s.cursor
	s.mu.Unlock()
	sub := &wire.Subscribe{ProtocolVersion: 4, BearerToken: &c.token, InitialState: 2}
	if cursor != "" {
		sub.ResumeCursor = &cursor
	}
	data, _ := proto.Marshal(sub)
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if conn.WriteMessage(websocket.BinaryMessage, data) != nil {
		return true, false, 0, errors.New("send server subscription")
	}
	var staged *wire.Snapshot
	var queued []*wire.Event
	caught := false
	seen := map[string]bool{}
	order := []string{}
	apply := func(e *wire.Event) error {
		if !seen[e.Id] {
			if err := s.applyEvent(c, e); err != nil {
				return err
			}
			seen[e.Id] = true
			order = append(order, e.Id)
			if len(order) > 2048 {
				delete(seen, order[0])
				order = order[1:]
			}
		}
		s.mu.Lock()
		if e.GetCursor() != "" {
			s.cursor = e.GetCursor()
		}
		s.mu.Unlock()
		return nil
	}
	for {
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return true, false, 0, errors.New("server realtime disconnected")
		}
		if kind != websocket.BinaryMessage {
			return false, false, 0, errors.New("unexpected realtime message type")
		}
		var frame wire.Frame
		if proto.Unmarshal(data, &frame) != nil {
			return false, false, 0, errors.New("invalid realtime frame")
		}
		if close := frame.GetClose(); close != nil {
			if close.Code == 6 || close.Code == 8 {
				return true, false, 0, errResnapshot
			}
			if close.RetryAfter != nil && close.RetryAfter.CheckValid() == nil {
				hint = max(0, close.RetryAfter.AsDuration())
			}
			return close.Reconnect || close.Code == 5, close.Code == 5, hint, errors.New("server closed realtime connection")
		}
		if snapshot := frame.GetSnapshot(); snapshot != nil {
			if caught || staged != nil {
				return false, false, 0, errors.New("unexpected realtime snapshot")
			}
			staged = snapshot
			continue
		}
		if boundary := frame.GetCaughtUp(); boundary != nil {
			if caught || (boundary.Recovery != 1 && boundary.Recovery != 2) || (boundary.Recovery == 2 && staged == nil) || (boundary.Recovery == 1 && (staged != nil || cursor == "")) {
				return false, false, 0, errors.New("invalid snapshot recovery boundary")
			}
			if err := s.commitRecovery(c, viewer, staged, queued, boundary.Cursor); err != nil {
				s.reset()
				return true, false, 0, err
			}
			caught = true
			queued = nil
			s.notify()
			continue
		}
		if heartbeat := frame.GetHeartbeat(); heartbeat != nil {
			if caught && heartbeat.Cursor != "" {
				s.mu.Lock()
				s.cursor = heartbeat.Cursor
				s.mu.Unlock()
			}
			if caught && s.options.IncludeNotifications {
				s.readMu.Lock()
				s.mu.Lock()
				expiry, cursor := s.state.NotificationExpiry, s.cursor
				s.mu.Unlock()
				var refreshErr error
				if !expiry.IsZero() && !time.Now().Before(expiry) {
					copy := *c
					copy.minimumCursor = cursor
					refreshErr = s.refreshNotifications(s.ctx, &copy)
					if refreshErr == nil {
						s.notify()
					}
				}
				s.readMu.Unlock()
				if refreshErr != nil {
					return true, false, 0, refreshErr
				}
			}
			continue
		}
		if event := frame.GetEvent(); event != nil {
			if !caught {
				queued = append(queued, event)
				if len(queued) > 1000 {
					return true, false, 0, errResnapshot
				}
				continue
			}
			if err := apply(event); err != nil {
				s.reset()
				return true, false, 0, err
			}
		}
	}
}

// Commit recovery as one store replacement, including all catch-up events.
// Resource reads happen against a private candidate; observers never see the
// snapshot without its intervening edits or a partly applied replay batch.
func (s *Server) commitRecovery(c *Client, viewer Profile, snapshot *wire.Snapshot, events []*wire.Event, cursor string) error {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	state := s.State()
	if snapshot != nil {
		var err error
		state, err = snapshotState(snapshot, viewer, state.Generation+1)
		if err != nil {
			return err
		}
	}
	candidate := &Server{ctx: s.ctx, state: state, options: s.options}
	seen := map[string]bool{}
	for _, event := range events {
		if !seen[event.Id] {
			if err := candidate.applyEvent(c, event); err != nil {
				return err
			}
			seen[event.Id] = true
		}
	}
	if snapshot != nil {
		var ids []string
		missing := map[string]bool{}
		for _, room := range candidate.state.Rooms {
			for _, id := range room.MemberUserIDs {
				if _, ok := candidate.state.Users[id]; !ok && !missing[id] {
					ids = append(ids, id)
					missing[id] = true
				}
			}
		}
		copy := *c
		copy.minimumCursor = cursor
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
		defer cancel()
		for len(ids) > 0 {
			n := min(100, len(ids))
			users, err := copy.Profiles(ctx, ids[:n])
			if err != nil {
				return err
			}
			for id, user := range users {
				candidate.state.Users[id] = user
			}
			ids = ids[n:]
		}
	}
	copy := *c
	copy.minimumCursor = cursor
	if err := candidate.refreshNotifications(s.ctx, &copy); err != nil {
		return err
	}
	s.mu.Lock()
	s.state = candidate.state
	s.state.Ready = true
	s.state.Status = "connected"
	s.state.Error = nil
	s.cursor = cursor
	s.revision++
	s.mu.Unlock()
	return nil
}

func (s *Server) applyEvent(base *Client, e *wire.Event) (result error) {
	if e == nil || e.Id == "" {
		return nil
	}
	switch e.Event.(type) {
	case *wire.Event_ViewerPermissionsChanged, *wire.Event_UserAccountDeleted, *wire.Event_UserAccountDeactivationChanged, *wire.Event_RoleDeleted, *wire.Event_RoleAssigned, *wire.Event_RoleRevoked, *wire.Event_RolePermissionsChanged, *wire.Event_RoomUniversalChanged:
		s.reset()
		return errResnapshot
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	defer func() {
		s.mu.Lock()
		s.revision++
		if result == nil && e.GetCursor() != "" {
			s.cursor = e.GetCursor()
		}
		s.mu.Unlock()
		s.notify()
	}()
	s.mu.Lock()
	s.revision++
	viewer := s.state.Viewer.ID
	s.mu.Unlock()
	c := *base
	c.minimumCursor = e.GetCursor()
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	room := ""
	switch event := e.Event.(type) {
	case *wire.Event_RoomDeleted:
		s.removeRoom(event.RoomDeleted.RoomId)
		return nil
	case *wire.Event_UserLeftRoom:
		if e.GetActorId() == viewer {
			s.reset()
			return errResnapshot
		}
		room = event.UserLeftRoom.RoomId
	case *wire.Event_RoomCreated:
		room = event.RoomCreated.RoomId
	case *wire.Event_RoomUpdated:
		room = event.RoomUpdated.RoomId
	case *wire.Event_RoomArchived:
		room = event.RoomArchived.RoomId
	case *wire.Event_RoomUnarchived:
		room = event.RoomUnarchived.RoomId
	case *wire.Event_UserJoinedRoom:
		room = event.UserJoinedRoom.RoomId
	case *wire.Event_RoomReadStateChanged:
		room = event.RoomReadStateChanged.RoomId
	case *wire.Event_NotificationUnreadStateChanged:
		if err := s.refreshNotifications(ctx, &c); err != nil {
			return err
		}
		room = event.NotificationUnreadStateChanged.RoomId
	case *wire.Event_NotificationOccurrencesChanged:
		return s.refreshNotifications(ctx, &c)
	case *wire.Event_ServerProfileChanged:
		profile, err := c.ServerProfile(ctx)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.state.Profile = profile
		s.mu.Unlock()
	case *wire.Event_RoomLayoutChanged:
		groups, err := c.RoomGroups(ctx)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.state.Groups = groups
		for _, group := range groups {
			for _, item := range group.Items {
				if item.Room != nil {
					s.upsertRoom(*item.Room)
				}
			}
		}
		s.mu.Unlock()
	case *wire.Event_UserProfileChanged:
		users, err := c.Profiles(ctx, []string{event.UserProfileChanged.UserId})
		if err != nil {
			return err
		}
		s.mu.Lock()
		for id, u := range users {
			s.state.Users[id] = u
			if id == viewer {
				s.state.Viewer = u
			}
		}
		s.mu.Unlock()
	case *wire.Event_MessagePosted:
		if err := s.refreshMessage(ctx, &c, event.MessagePosted.RoomId, e.Id, false); err != nil {
			return err
		}
	case *wire.Event_MessageEdited:
		if err := s.refreshMessage(ctx, &c, event.MessageEdited.RoomId, event.MessageEdited.MessageEventId, true); err != nil {
			return err
		}
	case *wire.Event_MessageRetracted:
		s.removeMessage(event.MessageRetracted.RoomId, event.MessageRetracted.MessageEventId)
	}
	if room != "" {
		var out struct{ Room RoomEntry }
		if err := c.call(ctx, "RoomDirectoryService/GetRoom", map[string]string{"roomId": room}, &out); err != nil {
			return err
		}
		if out.Room.Room.ID == "" {
			s.removeRoom(room)
		} else {
			s.mu.Lock()
			s.upsertRoom(out.Room)
			s.mu.Unlock()
		}
	}
	s.notify()
	return nil
}

// upsertRoom keeps directory rows and embedded group entries consistent under mu.
func (s *Server) upsertRoom(room RoomEntry) {
	found := false
	for i, entry := range s.state.Rooms {
		if entry.Room.ID == room.Room.ID {
			s.state.Rooms[i] = room
			found = true
		}
	}
	if !found {
		s.state.Rooms = append(s.state.Rooms, room)
	}
	for i := range s.state.Groups {
		for j, item := range s.state.Groups[i].Items {
			if item.Room != nil && item.Room.Room.ID == room.Room.ID {
				copy := room
				s.state.Groups[i].Items[j].Room = &copy
			}
		}
	}
}

func (s *Server) removeRoom(room string) {
	s.mu.Lock()
	delete(s.state.Notifications, room)
	rooms := s.state.Rooms[:0]
	for _, r := range s.state.Rooms {
		if r.Room.ID != room {
			rooms = append(rooms, r)
		}
	}
	s.state.Rooms = rooms
	for key := range s.state.Timelines {
		if key.Room == room {
			delete(s.state.Timelines, key)
		}
	}
	for i := range s.state.Groups {
		items := s.state.Groups[i].Items[:0]
		for _, item := range s.state.Groups[i].Items {
			if item.Room == nil || item.Room.Room.ID != room {
				items = append(items, item)
			}
		}
		s.state.Groups[i].Items = items
	}
	s.mu.Unlock()
	s.notify()
}
func (s *Server) removeMessage(room, id string) {
	s.mu.Lock()
	for key, page := range s.state.Timelines {
		if key.Room != room {
			continue
		}
		messages := page.Messages[:0]
		for _, m := range page.Messages {
			if m.ID != id {
				messages = append(messages, m)
			}
		}
		page.Messages = messages
		s.state.Timelines[key] = page
	}
	s.mu.Unlock()
	s.notify()
}
func (s *Server) refreshMessage(ctx context.Context, c *Client, room, id string, edited bool) error {
	if edited {
		s.removeMessage(room, id)
	}
	var out struct{ Message Message }
	if err := c.call(ctx, "MessageService/GetMessage", map[string]string{"roomId": room, "eventId": id}, &out); err != nil {
		return err
	}
	m := out.Message
	if m.ID == "" || m.DeletedAt != "" {
		return nil
	}
	s.mu.Lock()
	_, known := s.state.Users[m.ActorID]
	s.mu.Unlock()
	var users map[string]Profile
	if !known && m.ActorID != "" {
		var err error
		users, err = c.Profiles(ctx, []string{m.ActorID})
		if err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, u := range users {
		s.state.Users[id] = u
	}
	for key, page := range s.state.Timelines {
		if key.Room != room || (key.Root == "" && m.ThreadRootEventID != "") || (key.Root != "" && key.Root != m.ThreadRootID()) {
			continue
		}
		fresh := TimelinePage{Messages: []Message{m}, StartCursor: page.StartCursor, HasOlder: page.HasOlder}
		s.state.Timelines[key] = mergeTimeline(page, fresh, true)
	}
	return nil
}
