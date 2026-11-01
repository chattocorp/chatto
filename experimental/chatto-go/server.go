package chatto

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/chattocorp/chatto/experimental/chatto-go/internal/wire"
	"google.golang.org/protobuf/encoding/protojson"
)

// ConnectionOptions supplies credentials for one fixed origin and account.
// Client may be called concurrently. renew requests session rotation after a
// server renewal instruction. Hosts own durable refresh-token storage.
type ConnectionOptions struct {
	// IncludeNotifications enables initial and live notification-count reads.
	IncludeNotifications bool
	Client               func(ctx context.Context, renew bool) (*Client, error)
}

// TimelineKey identifies a room timeline or a thread within it.
type TimelineKey struct{ Room, Root string }

// ServerState is an isolated copy of the current stores. Generation increases
// when a snapshot or privacy reset replaces them. Updates are coalesced; always
// read State after receiving a notification. Error contains no response bodies.
type ServerState struct {
	Ready              bool
	Status             string
	Error              error
	Generation         uint64
	Profile            ServerProfile
	Viewer             Profile
	Rooms              []RoomEntry
	Groups             []RoomGroup
	Users              map[string]Profile
	Timelines          map[TimelineKey]TimelinePage
	Notifications      map[string]NotificationCount
	NotificationExpiry time.Time
}

// Server owns an in-memory server projection and its resumable realtime link.
// It does not persist credentials, log data, or share stores with other servers.
// Large histories load on demand; Close cancels and joins its realtime worker.
type Server struct {
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{}
	updates       chan struct{}
	changed       chan struct{}
	subscribers   map[chan struct{}]struct{}
	timelineEpoch map[TimelineKey]uint64
	options       ConnectionOptions
	mu            sync.Mutex
	readMu        sync.Mutex // Serializes resource hydration with timeline merges.
	state         ServerState
	revision      uint64
	cursor        string
	endpoint      string
	viewerID      string
}

// Connect starts snapshot synchronization asynchronously. WaitReady waits for
// the first committed snapshot. A snapshot is not exposed before caught_up.
func Connect(ctx context.Context, options ConnectionOptions) *Server {
	ctx, cancel := context.WithCancel(ctx)
	s := &Server{ctx: ctx, cancel: cancel, done: make(chan struct{}), updates: make(chan struct{}, 1), changed: make(chan struct{}), options: options}
	s.state = emptyState("connecting", 0)
	go s.run()
	return s
}

func emptyState(status string, generation uint64) ServerState {
	return ServerState{Status: status, Generation: generation, Users: map[string]Profile{}, Timelines: map[TimelineKey]TimelinePage{}}
}

// Updates signals changed stores or connection status. It has one consumer;
// a host with multiple views can distribute the resulting State itself.
func (s *Server) Updates() <-chan struct{} { return s.updates }

// Subscribe creates an independent, coalesced change signal. An initial signal
// tells the consumer to read current state. Cancel is idempotent and closes the
// channel; consumers must cancel when finished, including after Server.Close.
func (s *Server) Subscribe() (<-chan struct{}, func()) {
	s.mu.Lock()
	ch := make(chan struct{}, 1)
	if s.subscribers == nil {
		s.subscribers = make(map[chan struct{}]struct{})
	}
	s.subscribers[ch] = struct{}{}
	ch <- struct{}{}
	s.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.subscribers, ch)
			close(ch)
			s.mu.Unlock()
		})
	}
}

func (s *Server) notify() {
	s.mu.Lock()
	if s.changed != nil {
		close(s.changed)
		s.changed = make(chan struct{})
	}
	for ch := range s.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()
	select {
	case s.updates <- struct{}{}:
	default:
	}
}

// Close is idempotent and waits for the realtime worker to release its socket.
func (s *Server) Close() { s.cancel(); <-s.done }

// State returns detached maps, slices, and nested room-group entries.
func (s *Server) State() ServerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneState(s.state)
}

// ReadState returns metadata and only the requested cached timelines, in one
// consistent detached snapshot. With no keys it copies no message histories.
func (s *Server) ReadState(keys ...TimelineKey) ServerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.state
	state.Timelines = make(map[TimelineKey]TimelinePage, len(keys))
	for _, key := range keys {
		if page, ok := s.state.Timelines[key]; ok {
			state.Timelines[key] = page
		}
	}
	return cloneState(state)
}

// ReleaseTimeline removes cached history and fences reads already in flight.
// Future explicit loads can populate it again. Hosts choose their cache policy.
func (s *Server) ReleaseTimeline(key TimelineKey) {
	s.readMu.Lock()
	s.mu.Lock()
	if s.timelineEpoch == nil {
		s.timelineEpoch = make(map[TimelineKey]uint64)
	}
	s.timelineEpoch[key]++
	delete(s.state.Timelines, key)
	s.mu.Unlock()
	s.readMu.Unlock()
	s.notify()
}

func clonePage(p TimelinePage) TimelinePage {
	p.Messages = append([]Message(nil), p.Messages...)
	for i := range p.Messages {
		p.Messages[i].Attachments = append([]Attachment(nil), p.Messages[i].Attachments...)
	}
	p.Users = cloneUsers(p.Users)
	return p
}
func cloneUsers(users map[string]Profile) map[string]Profile {
	copy := make(map[string]Profile, len(users))
	for id, u := range users {
		copy[id] = u
	}
	return copy
}
func cloneState(state ServerState) ServerState {
	counts := make(map[string]NotificationCount, len(state.Notifications))
	for id, count := range state.Notifications {
		counts[id] = count
	}
	state.Notifications = counts
	state.Rooms = append([]RoomEntry(nil), state.Rooms...)
	for i := range state.Rooms {
		state.Rooms[i].MemberUserIDs = append([]string(nil), state.Rooms[i].MemberUserIDs...)
	}
	state.Groups = append([]RoomGroup(nil), state.Groups...)
	for i := range state.Groups {
		state.Groups[i].Items = append([]RoomGroupItem(nil), state.Groups[i].Items...)
		for j, item := range state.Groups[i].Items {
			if item.Room != nil {
				room := *item.Room
				room.MemberUserIDs = append([]string(nil), room.MemberUserIDs...)
				state.Groups[i].Items[j].Room = &room
			}
			if item.SidebarLink != nil {
				link := *item.SidebarLink
				state.Groups[i].Items[j].SidebarLink = &link
			}
		}
	}
	state.Users = cloneUsers(state.Users)
	pages := make(map[TimelineKey]TimelinePage, len(state.Timelines))
	for key, page := range state.Timelines {
		pages[key] = clonePage(page)
	}
	state.Timelines = pages
	return state
}

// WaitReady waits for committed data, or reports a terminal failure/cancellation.
// It does not consume Updates, so the host can subscribe independently.
func (s *Server) WaitReady(ctx context.Context) error {
	for {
		s.mu.Lock()
		state := s.state
		changed := s.changed
		s.mu.Unlock()
		if state.Ready {
			return nil
		}
		if state.Status == "closed" || state.Status == "failed" {
			if state.Error != nil {
				return state.Error
			}
			return errors.New("server connection closed")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return errors.New("server connection ended")
		case <-changed:
		}
	}
}

// LoadTimeline fills a timeline store on first use, or merges an older page.
// Repeated reads of an already loaded timeline do not issue HTTP requests.
// Snapshot replacements and realtime changes fence in-flight HTTP responses.
func (s *Server) LoadTimeline(ctx context.Context, room, root, before string) (TimelinePage, error) {
	return s.LoadTimelineWithOptions(ctx, room, root, TimelineOptions{Before: before})
}

// LoadTimelineWithOptions loads messages with caller-selected pagination.
// An empty cursor reuses cached data. Access is decided by the server API,
// not by whether a room appears in the navigation directory.
func (s *Server) LoadTimelineWithOptions(ctx context.Context, room, root string, options TimelineOptions) (TimelinePage, error) {
	if err := options.validate(); err != nil {
		return TimelinePage{}, err
	}
	key := TimelineKey{room, root}
	s.mu.Lock()
	epoch := s.timelineEpoch[key]
	s.mu.Unlock()
	for attempt := 0; attempt < 4; attempt++ {
		s.mu.Lock()
		if !s.state.Ready || s.ctx.Err() != nil || s.timelineEpoch[key] != epoch {
			s.mu.Unlock()
			return TimelinePage{}, errors.New("connection not ready or timeline released")
		}
		if page, ok := s.state.Timelines[key]; ok && options.Before == "" && options.After == "" {
			page = clonePage(page)
			s.mu.Unlock()
			return page, nil
		}
		revision, cursor := s.revision, s.cursor
		s.mu.Unlock()
		c, err := s.client(ctx, false)
		if err != nil {
			return TimelinePage{}, err
		}
		copy := *c
		copy.minimumCursor = cursor
		requestCtx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(s.ctx, cancel)
		page, err := copy.TimelineWithOptions(requestCtx, room, root, options)
		stop()
		cancel()
		if err != nil {
			return TimelinePage{}, err
		}
		s.readMu.Lock()
		s.mu.Lock()
		if revision != s.revision || epoch != s.timelineEpoch[key] || s.ctx.Err() != nil {
			s.mu.Unlock()
			s.readMu.Unlock()
			continue
		}
		if old, ok := s.state.Timelines[key]; ok {
			page = mergeTimeline(old, page, options.Before != "")
			if options.After != "" {
				page.StartCursor, page.HasOlder = old.StartCursor, old.HasOlder
			}
		}
		s.state.Timelines[key] = page
		for id, u := range page.Users {
			s.state.Users[id] = u
		}
		page = clonePage(page)
		s.mu.Unlock()
		s.readMu.Unlock()
		s.notify()
		return page, nil
	}
	return TimelinePage{}, errors.New("timeline changed during read; retry")
}

func mergeTimeline(old, fresh TimelinePage, older bool) TimelinePage {
	byID := map[string]Message{}
	for _, m := range old.Messages {
		byID[m.ID] = m
	}
	for _, m := range fresh.Messages {
		byID[m.ID] = m
	}
	fresh.Messages = nil
	for _, m := range byID {
		if m.DeletedAt == "" {
			fresh.Messages = append(fresh.Messages, m)
		}
	}
	sort.Slice(fresh.Messages, func(i, j int) bool {
		a, b := fresh.Messages[i], fresh.Messages[j]
		at, _ := time.Parse(time.RFC3339Nano, a.CreatedAt)
		bt, _ := time.Parse(time.RFC3339Nano, b.CreatedAt)
		if at.Equal(bt) {
			return a.ID < b.ID
		}
		return at.Before(bt)
	})
	if older {
		fresh.EndCursor, fresh.HasNewer = old.EndCursor, old.HasNewer
	}
	users := cloneUsers(old.Users)
	for id, u := range fresh.Users {
		users[id] = u
	}
	fresh.Users = users
	return fresh
}

func (s *Server) client(ctx context.Context, renew bool) (*Client, error) {
	if s.options.Client == nil {
		return nil, errors.New("server connection requires a client provider")
	}
	c, err := s.options.Client(ctx, renew)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("client provider returned nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.endpoint != "" && s.endpoint != c.endpoint {
		return nil, errors.New("server origin changed")
	}
	s.endpoint = c.endpoint
	return c, nil
}

func snapshotState(snapshot *wire.Snapshot, viewer Profile, generation uint64) (ServerState, error) {
	data, err := protojson.Marshal(snapshot)
	if err != nil {
		return ServerState{}, err
	}
	var dto struct {
		Server     ServerProfile
		Rooms      []RoomEntry
		RoomGroups []RoomGroup
		Users      []struct{ User Profile }
	}
	if err = json.Unmarshal(data, &dto); err != nil {
		return ServerState{}, err
	}
	state := emptyState("connected", generation)
	state.Ready = true
	state.Profile = dto.Server
	state.Viewer = viewer
	state.Groups = dto.RoomGroups
	state.Rooms = dto.Rooms
	for _, u := range dto.Users {
		state.Users[u.User.ID] = u.User
		if u.User.ID == viewer.ID {
			state.Viewer = u.User
		}
	}
	if _, ok := state.Users[viewer.ID]; !ok {
		state.Users[viewer.ID] = viewer
	}
	return state, nil
}
