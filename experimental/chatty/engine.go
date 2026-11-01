package main

import (
	"context"
	"encoding/json"
	"errors"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// diskState is private atomic state for one account and one uncertain send.
// Passwords and timelines are not persisted. The process lock prevents two
// instances from rotating the same credentials or retrying the same send.
type diskState struct {
	Session           chatto.HumanSession
	Pending           *pendingSend
	LastRoom          string          // Last successfully loaded room, scoped to this account/server.
	CollapsedSections map[string]bool // Stable sidebar section IDs, scoped to this session.
}
type pendingSend struct{ Room, Root, Body, Key string }
type engine struct {
	mu             sync.Mutex
	ctx            context.Context
	path, lock     string
	state          diskState
	storeErr       error
	events         chan struct{}
	watchCancel    context.CancelFunc
	server         *chatto.Server
	wg             sync.WaitGroup
	invalidated    atomic.Bool
	sidebarMu      sync.Mutex
	sidebarPending map[string]bool
}

func newEngine(ctx context.Context) (*engine, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, errors.New("cannot locate session directory")
	}
	dir = filepath.Join(dir, "chatty")
	if os.MkdirAll(dir, 0700) != nil || os.Chmod(dir, 0700) != nil {
		return nil, errors.New("cannot create private session directory")
	}
	e := &engine{ctx: ctx, path: filepath.Join(dir, "session.json"), lock: filepath.Join(dir, "running.lock"), events: make(chan struct{}, 1)}
	f, err := os.OpenFile(e.lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, errors.New("another Chatty process or stale running.lock exists in the Chatty config directory")
	}
	_ = f.Close()
	data, err := os.ReadFile(e.path)
	if err == nil {
		err = json.Unmarshal(data, &e.state)
	}
	if err != nil && !os.IsNotExist(err) {
		e.close()
		return nil, errors.New("cannot read saved session")
	}
	return e, nil
}
func (e *engine) close() {
	e.mu.Lock()
	// Flush the latest UI state even when quit overtakes its save command.
	e.sidebarMu.Lock()
	dirty := e.sidebarPending != nil
	e.sidebarMu.Unlock()
	if dirty {
		_ = e.save()
	}
	if e.watchCancel != nil {
		e.watchCancel()
	}
	server := e.server
	e.mu.Unlock()
	if server != nil {
		server.Close()
	}
	e.wg.Wait()
	_ = os.Remove(e.lock)
}
func (e *engine) save() (saveErr error) {
	e.sidebarMu.Lock()
	if e.sidebarPending != nil {
		if e.state.Session.AccessToken != "" {
			e.state.CollapsedSections = e.sidebarPending
		}
		e.sidebarPending = nil
	}
	e.sidebarMu.Unlock()
	defer func() {
		if saveErr != nil {
			e.storeErr = saveErr
		}
	}()
	f, err := os.CreateTemp(filepath.Dir(e.path), ".session-*")
	if err != nil {
		return errors.New("cannot save session")
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(e.state)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil || os.Rename(f.Name(), e.path) != nil {
		return errors.New("cannot save session")
	}
	d, err := os.Open(filepath.Dir(e.path))
	if err != nil {
		return errors.New("cannot sync session")
	}
	defer d.Close()
	if d.Sync() != nil {
		return errors.New("cannot sync session")
	}
	return nil
}

// queueSidebar never waits on network work. Each flush reads the latest queued
// state, so out-of-order save commands cannot restore an older selection.
func (e *engine) queueSidebar(sections map[string]bool) {
	copy := make(map[string]bool, len(sections))
	for key, value := range sections {
		copy[key] = value
	}
	e.sidebarMu.Lock()
	e.sidebarPending = copy
	e.sidebarMu.Unlock()
}
func (e *engine) saveSidebar() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.save()
}

// client runs under mu. Refresh identity is durable before the rotation request.
func (e *engine) client() (*chatto.Client, error) {
	if err := e.ctx.Err(); err != nil {
		return nil, err
	}
	if e.storeErr != nil {
		return nil, e.storeErr
	}
	s := e.state.Session
	if s.AccessToken == "" {
		return nil, errors.New("sign in first")
	}
	if s.RefreshRequestID != "" || time.Until(s.AccessExpiresAt) < time.Minute {
		if time.Now().After(s.RefreshExpiresAt) {
			return nil, errors.New("session expired; sign out and sign in again")
		}
		e.state.Session.PrepareRefresh()
		if err := e.save(); err != nil {
			return nil, err
		}
		fresh, err := e.state.Session.Refresh(e.ctx)
		if err != nil {
			return nil, errors.New("session renewal failed; retry or sign in again")
		}
		e.state.Session = fresh
		if err = e.save(); err != nil {
			return nil, err
		}
	}
	return e.state.Session.Client()
}

type snapshot struct {
	ServerGeneration   uint64
	Ready, HasTimeline bool
	ConnectionStatus   string
	ServerName         string
	Live               bool
	Room, Root, Before string
	Rooms              []chatto.RoomEntry
	Groups             []chatto.RoomGroup
	Notifications      map[string]chatto.NotificationCount
	Viewer             chatto.Profile
	Page               chatto.TimelinePage
	RoomPage           chatto.TimelinePage
	Users              map[string]chatto.Profile
	Pending            *pendingSend
	Err                error
}

func (e *engine) load(room, root, before string) snapshot {
	s := e.connection()
	if err := s.WaitReady(e.ctx); err != nil {
		return snapshot{Room: room, Root: root, Before: before, Err: err}
	}
	e.mu.Lock()
	if room == "" {
		room = e.state.LastRoom
	}
	e.mu.Unlock()
	state := s.ReadState()
	state.Rooms = activeRooms(state.Rooms)
	found := false
	for _, entry := range state.Rooms {
		if entry.Room.ID == room {
			found = true
		}
	}
	if !found {
		room, root, before = "", "", ""
		if len(state.Rooms) > 0 {
			room = state.Rooms[0].Room.ID
		}
	}
	if room != "" {
		if _, err := s.LoadTimelineWithOptions(e.ctx, room, root, chatto.TimelineOptions{Before: before, Limit: 50}); err != nil {
			r := e.current(room, root)
			r.Before = before
			r.Err = err
			return r
		}
		if root != "" {
			if _, err := s.LoadTimelineWithOptions(e.ctx, room, "", chatto.TimelineOptions{Limit: 50}); err != nil {
				r := e.current(room, root)
				r.Err = err
				return r
			}
		}
	}
	r := e.current(room, root)
	r.Before = before
	e.mu.Lock()
	if e.state.LastRoom != room {
		e.state.LastRoom = room
		r.Err = e.save()
	}
	e.mu.Unlock()
	return r
}

// current copies SDK stores without issuing requests.
func (e *engine) current(room, root string) snapshot {
	e.mu.Lock()
	s := e.server
	var pending *pendingSend
	if e.state.Pending != nil {
		p := *e.state.Pending
		pending = &p
	}
	e.mu.Unlock()
	r := snapshot{Room: room, Root: root, Pending: pending, Users: map[string]chatto.Profile{}}
	if s == nil {
		return r
	}
	state := s.ReadState(chatto.TimelineKey{Room: room, Root: root}, chatto.TimelineKey{Room: room})
	r.ServerGeneration = state.Generation
	r.Ready = state.Ready
	r.ConnectionStatus = state.Status
	r.Live = state.Status == "connected"
	r.ServerName = state.Profile.Name
	r.Viewer = state.Viewer
	r.Rooms, r.Groups, r.Users = activeRooms(state.Rooms), state.Groups, state.Users
	r.Notifications = state.Notifications
	r.Page, r.HasTimeline = state.Timelines[chatto.TimelineKey{Room: room, Root: root}]
	r.RoomPage = state.Timelines[chatto.TimelineKey{Room: room}]
	if state.Status == "failed" {
		r.Err = state.Error
	}
	return r
}

type actionDone struct {
	Kind string
	Err  error
}

// activeRooms is Chatty's navigation policy, not an SDK visibility rule.
func activeRooms(rooms []chatto.RoomEntry) []chatto.RoomEntry {
	result := make([]chatto.RoomEntry, 0, len(rooms))
	for _, room := range rooms {
		if !room.Room.Archived {
			result = append(result, room)
		}
	}
	return result
}

func (e *engine) login(ctx context.Context, origin, login, password string, browser bool, show func(string) error) actionDone {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state.Session.AccessToken != "" {
		return actionDone{"login", errors.New("sign out before changing accounts")}
	}
	var s chatto.HumanSession
	var err error
	if browser {
		s, err = chatto.LoginBrowser(ctx, origin, show)
	} else {
		s, err = chatto.LoginPassword(ctx, origin, login, password)
	}
	if err == nil {
		e.state.Session = s
		err = e.save()
	}
	return actionDone{"login", err}
}
func (e *engine) send(room, root, body string, retry bool) actionDone {
	e.mu.Lock()
	defer e.mu.Unlock()
	c, err := e.client()
	if err != nil {
		return actionDone{"send", err}
	}
	if retry {
		if e.state.Pending == nil {
			return actionDone{"send", errors.New("no pending send")}
		}
	} else {
		if e.state.Pending != nil {
			return actionDone{"send", errors.New("resolve the earlier uncertain send first")}
		}
		e.state.Pending = &pendingSend{Room: room, Root: root, Body: body, Key: chatto.NewMessageKey()}
	}
	if err = e.save(); err != nil {
		return actionDone{"send", err}
	}
	p := e.state.Pending
	if err = c.Reply(e.ctx, p.Room, p.Root, "", p.Body, p.Key); err != nil {
		return actionDone{"send", errors.New("send not confirmed; Ctrl+R retries exactly, Alt+D discards the local attempt")}
	}
	e.state.Pending = nil
	return actionDone{"send", e.save()}
}
func (e *engine) discard() actionDone {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.state.Pending = nil
	return actionDone{"discard", e.save()}
}
func (e *engine) logout() actionDone {
	e.mu.Lock()
	if err := e.state.Session.Logout(e.ctx); err != nil {
		e.mu.Unlock()
		return actionDone{"logout", errors.New("server logout not confirmed; local session retained")}
	}
	e.state = diskState{}
	if e.watchCancel != nil {
		e.watchCancel()
	}
	s := e.server
	e.server = nil
	err := e.save()
	e.mu.Unlock()
	if s != nil {
		s.Close()
	}
	return actionDone{"logout", err}
}

// connection creates one SDK-owned projection. The host retains only credential
// persistence and forwards store notifications to Bubble Tea.
func (e *engine) connection() *chatto.Server {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.server != nil {
		return e.server
	}
	ctx, cancel := context.WithCancel(e.ctx)
	e.watchCancel = cancel
	s := chatto.Connect(ctx, chatto.ConnectionOptions{IncludeNotifications: true, Client: func(ctx context.Context, renew bool) (*chatto.Client, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if renew {
			e.state.Session.AccessExpiresAt = time.Time{}
		}
		return e.client()
	}})
	e.server = s
	updates, unsubscribe := s.Subscribe()
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer unsubscribe()
		var generation uint64
		for {
			select {
			case <-ctx.Done():
				return
			case <-updates:
			}
			state := s.ReadState()
			if state.Generation != generation {
				e.invalidated.Store(true)
				generation = state.Generation
			}
			select {
			case e.events <- struct{}{}:
			default:
			}
		}
	}()
	return s
}
