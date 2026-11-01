package chatto

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestIndependentSubscriptionsAndFocusedSnapshots(t *testing.T) {
	s := &Server{state: emptyState("connected", 1)}
	key := TimelineKey{Room: "room"}
	s.state.Timelines[key] = TimelinePage{Messages: []Message{{ID: "message", Attachments: []Attachment{{ID: "asset"}}}}}
	a, cancelA := s.Subscribe()
	b, cancelB := s.Subscribe()
	defer cancelB()
	<-a
	<-b
	s.notify()
	s.notify()
	for _, ch := range []<-chan struct{}{a, b} {
		select {
		case <-ch:
		default:
			t.Fatal("subscriber missed change")
		}
		select {
		case <-ch:
			t.Fatal("signals were not coalesced")
		default:
		}
	}
	cancelA()
	cancelA()
	s.notify()
	if _, ok := <-a; ok {
		t.Fatal("cancel did not close subscription")
	}
	<-b
	if len(s.ReadState().Timelines) != 0 {
		t.Fatal("metadata read copied histories")
	}
	copy := s.ReadState(key)
	copy.Timelines[key].Messages[0].Attachments[0].ID = "changed"
	if s.State().Timelines[key].Messages[0].Attachments[0].ID != "asset" {
		t.Fatal("snapshot aliases store")
	}
	s.ReleaseTimeline(key)
	if len(s.State().Timelines) != 0 {
		t.Fatal("release retained history")
	}
}

func TestTimelineAccessOutsideDirectoryAndReleaseDuringRead(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	s, streams := testServerConnection(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
			<-finish
		}
		fmtJSON(w, `{"page":{"events":[{"messagePosted":{"message":{"id":"message"}}}]}}`)
	})
	stream := nextStream(t, streams)
	frame := initialFrame()
	frame.GetSnapshot().Rooms[0].Room.Archived = true
	sendFrame(t, stream.conn, frame)
	sendFrame(t, stream.conn, caughtFrame("ready", 2))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if !s.State().Rooms[0].Room.Archived {
		t.Fatal("SDK filtered archived room")
	}
	result := make(chan error, 1)
	go func() { _, err := s.LoadTimeline(ctx, "direct-link-room", "thread", ""); result <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("directory incorrectly blocked API access")
	}
	key := TimelineKey{Room: "direct-link-room", Root: "thread"}
	s.ReleaseTimeline(key)
	close(finish)
	if err := <-result; err == nil {
		t.Fatal("released request populated store")
	}
	if _, ok := s.State().Timelines[key]; ok {
		t.Fatal("released history returned")
	}
	page, err := s.LoadTimeline(ctx, key.Room, key.Root, "")
	if err != nil || len(page.Messages) != 1 {
		t.Fatal("subsequent authorized load failed", err)
	}
	s.Close()
	if _, err := s.LoadTimeline(ctx, key.Room, key.Root, ""); err == nil {
		t.Fatal("closed connection loaded data")
	}
}

func TestUnfilteredEventsAndCallerPagination(t *testing.T) {
	var calls int
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var input map[string]any
		_ = json.NewDecoder(r.Body).Decode(&input)
		if input["after"] != "cursor" || input["limit"] != float64(17) || input["threadRootEventId"] != "root" {
			t.Error("caller pagination lost")
		}
		fmtJSON(w, `{"page":{"events":[{"futureEvent":{"value":1}},{"roomCreated":{}}],"includes":{"futureResource":{"id":"x"}},"hasNewer":true}}`)
	}))
	defer s.Close()
	c, _ := New(s.URL, "token")
	p, err := c.TimelineEvents(context.Background(), "room", "root", TimelineOptions{After: "cursor", Limit: 17})
	if err != nil || len(p.Events) != 2 || len(p.Includes) == 0 || !p.HasNewer {
		t.Fatal("raw protocol data lost", err)
	}
	_, err = c.TimelineEvents(context.Background(), "room", "root", TimelineOptions{Before: "a", After: "b"})
	if err == nil || calls != 1 {
		t.Fatal("invalid cursor combination reached API")
	}
}

func TestDownloadNonImageWithCallerLimit(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if r.Header.Get("Authorization") != "" {
				t.Error("download leaked bearer token")
			}
			_, _ = w.Write([]byte("plain text"))
			return
		}
		var input map[string]any
		_ = json.NewDecoder(r.Body).Decode(&input)
		if _, ok := input["thumbnail"]; ok {
			t.Error("original request forced a thumbnail")
		}
		fmtJSON(w, `{"asset":{"id":"file","contentType":"text/plain","assetUrl":{"url":"/file"}}}`)
	}))
	defer s.Close()
	c, _ := New(s.URL, "token")
	data, err := c.DownloadAsset(context.Background(), "room", "file", AssetDownloadOptions{MaxBytes: 10})
	if err != nil || string(data) != "plain text" {
		t.Fatal("non-image download failed", err)
	}
	if _, err := c.DownloadAsset(context.Background(), "room", "file", AssetDownloadOptions{MaxBytes: 9}); err == nil {
		t.Fatal("caller limit ignored")
	}
	if _, err := c.DownloadAsset(context.Background(), "room", "file", AssetDownloadOptions{}); err == nil {
		t.Fatal("unbounded allocation allowed")
	}
}
