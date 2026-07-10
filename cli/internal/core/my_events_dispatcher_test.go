package core

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/config"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func TestMyEventsDispatcherUsesConstantSharedSubscriptions(t *testing.T) {
	core, nc := setupTestCore(t)

	select {
	case <-core.myEventsModel.dispatchReady:
	case <-time.After(5 * time.Second):
		t.Fatal("live event dispatcher did not become ready")
	}

	baselineNATSSubscriptions := nc.NumSubscriptions()
	core.PresenceHub.mu.Lock()
	baselinePresenceSubscriptions := len(core.PresenceHub.subscribers)
	core.PresenceHub.mu.Unlock()
	if baselinePresenceSubscriptions != 1 {
		t.Fatalf("PresenceHub subscriptions = %d, want one shared dispatcher subscription", baselinePresenceSubscriptions)
	}

	const streamCount = 3
	streamCancels := make([]context.CancelFunc, 0, streamCount)
	streamChannels := make([]<-chan EventEnvelope, 0, streamCount)
	for i := 0; i < streamCount; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		streamCancels = append(streamCancels, cancel)
		stream, err := core.StreamMyEventsWithOptions(ctx, "dispatcher-user", StreamMyEventsOptions{})
		if err != nil {
			t.Fatalf("StreamMyEventsWithOptions %d: %v", i, err)
		}
		streamChannels = append(streamChannels, stream)
	}

	if got := nc.NumSubscriptions(); got != baselineNATSSubscriptions {
		t.Fatalf("NATS subscriptions after %d streams = %d, want baseline %d", streamCount, got, baselineNATSSubscriptions)
	}
	core.PresenceHub.mu.Lock()
	gotPresenceSubscriptions := len(core.PresenceHub.subscribers)
	core.PresenceHub.mu.Unlock()
	if gotPresenceSubscriptions != baselinePresenceSubscriptions {
		t.Fatalf("PresenceHub subscriptions after %d streams = %d, want baseline %d", streamCount, gotPresenceSubscriptions, baselinePresenceSubscriptions)
	}

	core.myEventsModel.dispatchMu.Lock()
	gotDispatchSubscriptions := len(core.myEventsModel.dispatchSubscribers)
	core.myEventsModel.dispatchMu.Unlock()
	if gotDispatchSubscriptions != streamCount {
		t.Fatalf("dispatcher subscriptions = %d, want %d", gotDispatchSubscriptions, streamCount)
	}

	for _, cancel := range streamCancels {
		cancel()
	}
	for i, stream := range streamChannels {
		select {
		case _, ok := <-stream:
			if ok {
				t.Fatalf("stream %d emitted an event while closing", i)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("stream %d did not close", i)
		}
	}

	core.myEventsModel.dispatchMu.Lock()
	remainingDispatchSubscriptions := len(core.myEventsModel.dispatchSubscribers)
	core.myEventsModel.dispatchMu.Unlock()
	if remainingDispatchSubscriptions != 0 {
		t.Fatalf("dispatcher subscriptions after disconnect = %d, want 0", remainingDispatchSubscriptions)
	}
}

func TestMyEventsMemberRoomCacheSeedsExplicitAndUniversalMemberships(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	actor, err := core.CreateUser(ctx, SystemActorID, "cache-seed-actor", "Cache Seed Actor", "password123")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	viewer, err := core.CreateUser(ctx, SystemActorID, "cache-seed-viewer", "Cache Seed Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	explicitRoom, err := core.CreateRoom(ctx, actor.Id, KindChannel, "", "cache-seed-explicit", "")
	if err != nil {
		t.Fatalf("CreateRoom explicit: %v", err)
	}
	if _, err := core.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, explicitRoom.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}
	universalRoom, err := core.CreateRoom(ctx, actor.Id, KindChannel, "", "cache-seed-universal", "")
	if err != nil {
		t.Fatalf("CreateRoom universal: %v", err)
	}
	if _, err := core.SetRoomUniversal(ctx, actor.Id, KindChannel, universalRoom.Id, true); err != nil {
		t.Fatalf("SetRoomUniversal: %v", err)
	}
	privateRoom, err := core.CreateRoom(ctx, actor.Id, KindChannel, "", "cache-seed-private", "")
	if err != nil {
		t.Fatalf("CreateRoom private: %v", err)
	}
	dmRoom, _, err := core.FindOrCreateDM(ctx, viewer.Id, []string{actor.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}

	memberRooms := make(map[string]struct{})
	if err := core.myEventsModel.populateMemberRoomsCache(ctx, viewer.Id, memberRooms); err != nil {
		t.Fatalf("populateMemberRoomsCache: %v", err)
	}
	for label, roomID := range map[string]string{
		"explicit channel":  explicitRoom.Id,
		"universal channel": universalRoom.Id,
		"explicit DM":       dmRoom.Id,
	} {
		if _, ok := memberRooms[roomID]; !ok {
			t.Errorf("%s %q missing from member room cache", label, roomID)
		}
	}
	if _, ok := memberRooms[privateRoom.Id]; ok {
		t.Errorf("private non-member room %q present in member room cache", privateRoom.Id)
	}
}

func TestMyEventsMemberRoomCacheSkipsDeletedCatalogEntry(t *testing.T) {
	directory := NewRoomDirectoryProjection()
	directory.Membership.byUser["viewer"] = map[string]struct{}{
		"deleted-room":   {},
		"malformed-room": {},
		"valid-channel":  {},
	}
	directory.Catalog.rooms["malformed-room"] = &roomCatalogEntry{}
	directory.Catalog.rooms["valid-channel"] = &roomCatalogEntry{kind: corev1.RoomKind_ROOM_KIND_CHANNEL}
	core := &ChattoCore{
		RoomDirectory:  directory,
		RoomMembership: directory.Membership,
		RoomCatalog:    directory.Catalog,
		roomModel:      &RoomModel{directory: directory},
	}
	model := NewMyEventsModel(core)
	memberRooms := make(map[string]struct{})
	if err := model.populateMemberRoomsCache(context.Background(), "viewer", memberRooms); err != nil {
		t.Fatalf("populateMemberRoomsCache: %v", err)
	}
	if len(memberRooms) != 1 {
		t.Fatalf("member room cache = %v, want only valid channel", memberRooms)
	}
	if _, ok := memberRooms["valid-channel"]; !ok {
		t.Fatalf("member room cache = %v, want valid channel included", memberRooms)
	}
}

func TestMyEventsDispatcherSharesPreparedEvent(t *testing.T) {
	model := readyTestMyEventsDispatcher()
	sub1 := subscribeTestLiveDispatch(t, model)
	sub2 := subscribeTestLiveDispatch(t, model)

	prepared := &preparedLiveEvent{
		kind:     preparedPresence,
		envelope: NewHeartbeatEventEnvelope("event-id", timestamppb.Now()),
	}
	model.broadcastPreparedLiveEvent(prepared)

	if got := receivePreparedLiveEvent(t, sub1); got != prepared {
		t.Fatal("subscriber 1 did not receive the shared prepared event pointer")
	}
	if got := receivePreparedLiveEvent(t, sub2); got != prepared {
		t.Fatal("subscriber 2 did not receive the shared prepared event pointer")
	}
}

func TestMyEventsDispatcherDrainsPresenceIndependently(t *testing.T) {
	model := readyTestMyEventsDispatcher()
	dispatchSub := subscribeTestLiveDispatch(t, model)
	presenceUpdates := make(chan PresenceUpdate, 1)
	presenceSub := &PresenceSubscription{C: presenceUpdates, ch: presenceUpdates}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- model.runPresenceFanout(ctx, presenceSub) }()

	presenceUpdates <- PresenceUpdate{UserID: "presence-user", Status: PresenceStatusAway}
	prepared := receivePreparedLiveEvent(t, dispatchSub)
	changed := prepared.envelope.LiveEvent().GetPresenceChanged()
	if prepared.kind != preparedPresence || changed.GetStatus() != PresenceStatusAway {
		t.Fatalf("prepared presence event = kind %d, status %q", prepared.kind, changed.GetStatus())
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("presence fanout error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("presence fanout did not stop")
	}
}

func TestMyEventsDispatcherDisconnectsOnlyOverflowingSubscriber(t *testing.T) {
	model := readyTestMyEventsDispatcher()
	slow := subscribeTestLiveDispatch(t, model)
	fast := subscribeTestLiveDispatch(t, model)

	for i := 0; i < liveDispatchQueueSize; i++ {
		model.broadcastPreparedLiveEvent(&preparedLiveEvent{
			kind:     preparedPresence,
			envelope: NewHeartbeatEventEnvelope("queued-event", timestamppb.Now()),
		})
		<-fast.C
	}

	overflow := &preparedLiveEvent{
		kind:     preparedPresence,
		envelope: NewHeartbeatEventEnvelope("overflow-event", timestamppb.Now()),
	}
	model.broadcastPreparedLiveEvent(overflow)

	select {
	case <-slow.Done:
	case <-time.After(time.Second):
		t.Fatal("slow subscriber was not disconnected")
	}
	if slow.reason != liveDispatchSubscriberSlow {
		t.Fatalf("slow subscriber reason = %d, want %d", slow.reason, liveDispatchSubscriberSlow)
	}
	select {
	case <-fast.Done:
		t.Fatal("fast subscriber was disconnected with slow subscriber")
	default:
	}
	if got := receivePreparedLiveEvent(t, fast); got != overflow {
		t.Fatal("fast subscriber did not receive overflow event")
	}
	if got := model.Metrics().SlowDisconnects; got != 1 {
		t.Fatalf("slow disconnect metric = %d, want 1", got)
	}
}

func TestMyEventsDispatcherSharedGapResetsCurrentSubscribers(t *testing.T) {
	model := readyTestMyEventsDispatcher()
	sub1 := subscribeTestLiveDispatch(t, model)
	sub2 := subscribeTestLiveDispatch(t, model)

	if got := model.resetDispatchSubscribers(liveDispatchSourceGap, true); got != 2 {
		t.Fatalf("reset subscriber count = %d, want 2", got)
	}
	for i, sub := range []*liveDispatchSubscription{sub1, sub2} {
		select {
		case <-sub.Done:
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d was not reset", i)
		}
		if sub.reason != liveDispatchSourceGap {
			t.Fatalf("subscriber %d reason = %d, want %d", i, sub.reason, liveDispatchSourceGap)
		}
	}
	if got := model.Metrics().SlowDisconnects; got != 2 {
		t.Fatalf("slow disconnect metric = %d, want 2", got)
	}

	replacement := subscribeTestLiveDispatch(t, model)
	select {
	case <-replacement.Done:
		t.Fatal("replacement subscriber started closed")
	default:
	}
}

func TestMyEventsDispatcherSourceGapDiscardsQueuedMessagesBeforeReplacement(t *testing.T) {
	model := readyTestMyEventsDispatcher()
	staleSubscriber := subscribeTestLiveDispatch(t, model)
	msgChan := make(chan *nats.Msg, 3)
	msgChan <- &nats.Msg{Subject: "live.evt.room.R1.user_left_room"}
	msgChan <- &nats.Msg{Subject: "live.evt.room.R1.user_joined_room"}

	disconnected, discarded := model.resetDispatchSubscribersAfterSourceGap(msgChan)
	if disconnected != 1 || discarded != 2 {
		t.Fatalf("source gap reset = (%d disconnected, %d discarded), want (1, 2)", disconnected, discarded)
	}
	select {
	case <-staleSubscriber.Done:
	case <-time.After(time.Second):
		t.Fatal("pre-gap subscriber was not reset")
	}

	replacement := subscribeTestLiveDispatch(t, model)
	select {
	case msg := <-msgChan:
		t.Fatalf("replacement could observe stale message %q", msg.Subject)
	default:
	}
	select {
	case <-replacement.Done:
		t.Fatal("replacement subscriber started closed")
	default:
	}
}

func TestMyEventsDispatcherRejectsSubscribersWhileNATSSourceIsUnavailable(t *testing.T) {
	model := readyTestMyEventsDispatcher()
	current := subscribeTestLiveDispatch(t, model)

	disconnected, discarded := model.markDispatchSourceUnavailable(nil)
	if disconnected != 1 || discarded != 0 {
		t.Fatalf("source unavailable reset = (%d disconnected, %d discarded), want (1, 0)", disconnected, discarded)
	}
	select {
	case <-current.Done:
	case <-time.After(time.Second):
		t.Fatal("current subscriber was not reset")
	}
	if _, err := model.subscribeToLiveDispatch(context.Background()); err == nil || !strings.Contains(err.Error(), "temporarily unavailable") {
		t.Fatalf("subscribe while source unavailable error = %v", err)
	}

	model.markDispatchSourceAvailable()
	replacement := subscribeTestLiveDispatch(t, model)
	select {
	case <-replacement.Done:
		t.Fatal("replacement subscriber started closed")
	default:
	}
}

func TestMyEventsDispatcherStopsWhenSharedNATSSubscriptionsClose(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	core, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "dispatcher-source-close-test",
		Assets: config.AssetsConfig{
			SigningSecret: "dispatcher-source-close-signing-secret",
		},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	core.PresenceHub.readyOnce.Do(func() { close(core.PresenceHub.ready) })

	done := make(chan error, 1)
	go func() { done <- core.myEventsModel.Run(ctx) }()
	select {
	case <-core.myEventsModel.dispatchReady:
	case <-ctx.Done():
		t.Fatal("dispatcher did not become ready")
	}

	nc.Close()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "shared live") {
			t.Fatalf("dispatcher error = %v, want shared subscription closure", err)
		}
	case <-ctx.Done():
		t.Fatal("dispatcher remained live after NATS subscriptions closed")
	}
}

func BenchmarkMyEventsPopulateMemberRoomsCache(b *testing.B) {
	for _, roomCount := range []int{0, 1_000, 10_000} {
		b.Run("rooms_"+strconv.Itoa(roomCount), func(b *testing.B) {
			directory := NewRoomDirectoryProjection()
			groups := NewRoomGroupProjection()
			const userID = "benchmark-user"
			rooms := make(map[string]struct{}, roomCount)
			for i := 0; i < roomCount; i++ {
				roomID := "room-" + strconv.Itoa(i)
				directory.Catalog.rooms[roomID] = &roomCatalogEntry{
					name:        "Benchmark Room " + strconv.Itoa(i),
					description: "Representative room description for connection allocation measurement",
					kind:        corev1.RoomKind_ROOM_KIND_CHANNEL,
				}
				rooms[roomID] = struct{}{}
			}
			directory.Membership.byUser[userID] = rooms
			core := &ChattoCore{
				RoomDirectory:  directory,
				RoomMembership: directory.Membership,
				RoomCatalog:    directory.Catalog,
				RoomGroups:     groups,
				roomModel:      &RoomModel{directory: directory},
			}
			model := NewMyEventsModel(core)

			b.ReportAllocs()
			for b.Loop() {
				memberRooms := make(map[string]struct{})
				if err := model.populateMemberRoomsCache(context.Background(), userID, memberRooms); err != nil {
					b.Fatalf("populateMemberRoomsCache: %v", err)
				}
				if len(memberRooms) != roomCount {
					b.Fatalf("member room count = %d, want %d", len(memberRooms), roomCount)
				}
			}
		})
	}
}

func readyTestMyEventsDispatcher() *MyEventsModel {
	model := NewMyEventsModel(&ChattoCore{})
	model.finishDispatchStartup(nil)
	return model
}

func subscribeTestLiveDispatch(t *testing.T, model *MyEventsModel) *liveDispatchSubscription {
	t.Helper()
	sub, err := model.subscribeToLiveDispatch(context.Background())
	if err != nil {
		t.Fatalf("subscribeToLiveDispatch: %v", err)
	}
	t.Cleanup(func() { model.unsubscribeFromLiveDispatch(sub) })
	return sub
}

func receivePreparedLiveEvent(t *testing.T, sub *liveDispatchSubscription) *preparedLiveEvent {
	t.Helper()
	select {
	case event := <-sub.C:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for prepared event")
		return nil
	}
}
