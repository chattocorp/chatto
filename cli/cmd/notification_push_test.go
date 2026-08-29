package cmd

import (
	"context"
	"errors"
	"hmans.de/chatto/internal/pb/chatto/core/notification/v1"
	"hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/internal/push"
	"hmans.de/chatto/internal/testutil"
)

type recordingNotificationPushSender struct {
	mu        sync.Mutex
	calls     int
	inputs    [][]*runtimestatev1.PushSubscription
	payload   []*push.Payload
	deadlines []time.Time
	results   func([]*runtimestatev1.PushSubscription) []*push.SendResult
}

func (s *recordingNotificationPushSender) SendToMany(ctx context.Context, subscriptions []*runtimestatev1.PushSubscription, payload *push.Payload) []*push.SendResult {
	return s.SendToManyMapped(ctx, subscriptions, func(*runtimestatev1.PushSubscription) *push.Payload {
		return payload
	})
}

func (s *recordingNotificationPushSender) SendToManyMapped(
	ctx context.Context,
	subscriptions []*runtimestatev1.PushSubscription,
	payloadFor func(*runtimestatev1.PushSubscription) *push.Payload,
) []*push.SendResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.inputs = append(s.inputs, subscriptions)
	if len(subscriptions) > 0 {
		copyPayload := *payloadFor(subscriptions[0])
		s.payload = append(s.payload, &copyPayload)
	}
	deadline, _ := ctx.Deadline()
	s.deadlines = append(s.deadlines, deadline)
	return s.results(subscriptions)
}

func setupNotificationPushTestCore(t *testing.T) (*core.ChattoCore, context.Context) {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	chattoCore, err := core.NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "notification-push-handler-test-secret",
		Assets:    config.AssetsConfig{SigningSecret: "notification-push-handler-test-assets"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	// Keep alert occurrences pending so these tests can exercise the production
	// provider handler directly. The durable worker retries ordinary transport
	// failures without resolving the occurrence.
	chattoCore.SetNotificationAlertHandler(func(context.Context, *notificationv1.NotificationOccurrence) error {
		return errors.New("hold notification alert for direct handler test")
	})
	runCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- chattoCore.Run(runCtx) }()
	t.Cleanup(func() {
		stop()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("ChattoCore.Run did not stop")
		}
	})
	if err := chattoCore.WaitForBoot(ctx); err != nil {
		t.Fatalf("WaitForBoot: %v", err)
	}
	return chattoCore, ctx
}

func notificationPushFixture(t *testing.T, messageCount int) (*core.ChattoCore, context.Context, *evtv1.User, *evtv1.User, *notificationv1.NotificationOccurrence) {
	t.Helper()
	chattoCore, ctx := setupNotificationPushTestCore(t)
	alice, err := chattoCore.CreateUser(ctx, core.SystemActorID, "push-handler-alice", "Alice", "password")
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	bob, err := chattoCore.CreateUser(ctx, core.SystemActorID, "push-handler-bob", "Bob", "password")
	if err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	room, _, err := chattoCore.FindOrCreateDM(ctx, alice.Id, []string{bob.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	var latest *evtv1.Event
	for index := range messageCount {
		latest, err = chattoCore.PostMessage(ctx, core.KindDM, room.Id, bob.Id, "push handler message", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage %d: %v", index, err)
		}
	}
	if err := chattoCore.NotificationOccurrences().WaitCurrent(ctx); err != nil {
		t.Fatalf("WaitCurrent: %v", err)
	}
	occurrences, err := chattoCore.NotificationOccurrences().List(ctx, alice.Id)
	if err != nil {
		t.Fatalf("List occurrences: %v", err)
	}
	for _, occurrence := range occurrences {
		if occurrence.GetSourceEventId() == latest.GetId() {
			return chattoCore, ctx, alice, bob, occurrence
		}
	}
	t.Fatalf("occurrence for %s was not materialized", latest.GetId())
	return nil, nil, nil, nil, nil
}

func TestNotificationAlertHandlerCompletesWhenAnyCurrentDeviceAccepts(t *testing.T) {
	chattoCore, ctx, alice, _, occurrence := notificationPushFixture(t, 2)
	endpoints := []string{
		"https://push.example.test/gone",
		"https://push.example.test/accepted",
		"https://push.example.test/failed",
	}
	for _, endpoint := range endpoints {
		if _, err := chattoCore.SavePushSubscription(ctx, alice.Id, endpoint, "key", "auth", "browser"); err != nil {
			t.Fatalf("SavePushSubscription %s: %v", endpoint, err)
		}
	}
	sender := &recordingNotificationPushSender{results: func(subscriptions []*runtimestatev1.PushSubscription) []*push.SendResult {
		results := make([]*push.SendResult, 0, len(subscriptions))
		for _, subscription := range subscriptions {
			if subscription.GetEndpoint() == endpoints[0] {
				results = append(results, &push.SendResult{Endpoint: subscription.GetEndpoint(), Gone: true})
			} else if subscription.GetEndpoint() == endpoints[1] {
				results = append(results, &push.SendResult{Endpoint: subscription.GetEndpoint(), Success: true})
			} else {
				results = append(results, &push.SendResult{Endpoint: subscription.GetEndpoint(), Error: errors.New("provider unavailable")})
			}
		}
		return results
	}}
	handler := notificationAlertHandler(chattoCore, config.ChattoConfig{
		Webserver: config.WebserverConfig{URL: "https://chat.example.test"},
	}, sender, log.New(io.Discard))

	if err := handler(ctx, occurrence); err != nil {
		t.Fatalf("notification alert handler: %v", err)
	}
	if sender.calls != 1 || len(sender.payload) != 1 || sender.payload[0].AppBadge != "2" {
		t.Fatalf("sender calls/payload = (%d, %+v), want one call with app badge 2", sender.calls, sender.payload)
	}
	if deadline := sender.payload[0].DeliveryDeadline; deadline.IsZero() || deadline.After(time.Now().Add(2*time.Minute)) {
		t.Fatalf("notification provider deadline = %v, want remaining immutable alert lifetime", deadline)
	}
	wantDeadline := core.NotificationAlertDeadline(occurrence)
	if parentDeadline, ok := ctx.Deadline(); ok && parentDeadline.Before(wantDeadline) {
		wantDeadline = parentDeadline
	}
	if len(sender.deadlines) != 1 || sender.deadlines[0].IsZero() || !sender.deadlines[0].Equal(wantDeadline) {
		t.Fatalf("notification provider context deadline = %v, want %v", sender.deadlines, wantDeadline)
	}
	remaining, err := chattoCore.GetUserPushSubscriptions(ctx, alice.Id)
	if err != nil {
		t.Fatalf("GetUserPushSubscriptions: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("remaining subscriptions = %+v, want accepted and transiently failed endpoints", remaining)
	}
	for _, subscription := range remaining {
		if subscription.GetEndpoint() == endpoints[0] {
			t.Fatalf("gone subscription remains: %+v", remaining)
		}
	}
}

func TestNotificationAlertHandlerRevalidatesDNDAndReadState(t *testing.T) {
	t.Run("do not disturb", func(t *testing.T) {
		chattoCore, ctx, alice, _, occurrence := notificationPushFixture(t, 1)
		if _, err := chattoCore.SavePushSubscription(ctx, alice.Id, "https://push.example.test/dnd", "key", "auth", "browser"); err != nil {
			t.Fatalf("SavePushSubscription: %v", err)
		}
		if err := chattoCore.SetPresence(ctx, alice.Id, core.PresenceStatusDoNotDisturb); err != nil {
			t.Fatalf("SetPresence: %v", err)
		}
		sender := &recordingNotificationPushSender{results: func([]*runtimestatev1.PushSubscription) []*push.SendResult {
			t.Fatal("DND handler contacted provider")
			return nil
		}}
		err := notificationAlertHandler(chattoCore, config.ChattoConfig{}, sender, log.New(io.Discard))(ctx, occurrence)
		if !errors.Is(err, core.ErrNotificationAlertSuppressed) || sender.calls != 0 {
			t.Fatalf("DND handler = (%v, %d calls), want suppressed without provider", err, sender.calls)
		}
	})

	t.Run("read before final delivery", func(t *testing.T) {
		chattoCore, ctx, alice, _, occurrence := notificationPushFixture(t, 1)
		if _, err := chattoCore.SavePushSubscription(ctx, alice.Id, "https://push.example.test/read", "key", "auth", "browser"); err != nil {
			t.Fatalf("SavePushSubscription: %v", err)
		}
		if _, err := chattoCore.NotificationOccurrences().MarkRead(ctx, alice.Id, occurrence.GetId()); err != nil {
			t.Fatalf("MarkRead: %v", err)
		}
		sender := &recordingNotificationPushSender{results: func([]*runtimestatev1.PushSubscription) []*push.SendResult {
			t.Fatal("read occurrence contacted provider")
			return nil
		}}
		err := notificationAlertHandler(chattoCore, config.ChattoConfig{}, sender, log.New(io.Discard))(ctx, occurrence)
		if !errors.Is(err, core.ErrNotificationAlertSuppressed) {
			t.Fatalf("read handler = %v, want suppressed", err)
		}
		if sender.calls != 0 {
			t.Fatalf("read handler provider calls = %d, want zero", sender.calls)
		}
	})

	t.Run("policy downgraded before final delivery", func(t *testing.T) {
		chattoCore, ctx, alice, _, occurrence := notificationPushFixture(t, 1)
		if _, err := chattoCore.SavePushSubscription(ctx, alice.Id, "https://push.example.test/policy", "key", "auth", "browser"); err != nil {
			t.Fatalf("SavePushSubscription: %v", err)
		}
		if _, err := chattoCore.NotificationPolicy().UpdateNotificationPolicy(
			ctx,
			alice.Id,
			"",
			&evtv1.NotificationDeliveryModes{DirectMessages: evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION.Enum()},
			&fieldmaskpb.FieldMask{Paths: []string{"direct_messages"}},
		); err != nil {
			t.Fatalf("UpdateNotificationPolicy: %v", err)
		}
		sender := &recordingNotificationPushSender{results: func([]*runtimestatev1.PushSubscription) []*push.SendResult {
			t.Fatal("policy-downgraded occurrence contacted provider")
			return nil
		}}
		err := notificationAlertHandler(chattoCore, config.ChattoConfig{}, sender, log.New(io.Discard))(ctx, occurrence)
		if !errors.Is(err, core.ErrNotificationAlertSuppressed) || sender.calls != 0 {
			t.Fatalf("policy-downgraded handler = (%v, %d calls), want suppressed without provider", err, sender.calls)
		}
	})
}

func TestNotificationAlertHandlerRejectsTransferredEndpoint(t *testing.T) {
	chattoCore, ctx, alice, bob, occurrence := notificationPushFixture(t, 1)
	endpoint := "https://push.example.test/transferred"
	if _, err := chattoCore.SavePushSubscription(ctx, alice.Id, endpoint, "alice-key", "alice-auth", "browser"); err != nil {
		t.Fatalf("SavePushSubscription alice: %v", err)
	}
	if _, err := chattoCore.SavePushSubscription(ctx, bob.Id, endpoint, "bob-key", "bob-auth", "browser"); err != nil {
		t.Fatalf("SavePushSubscription bob: %v", err)
	}
	sender := &recordingNotificationPushSender{results: func([]*runtimestatev1.PushSubscription) []*push.SendResult {
		t.Fatal("transferred endpoint contacted provider")
		return nil
	}}
	err := notificationAlertHandler(chattoCore, config.ChattoConfig{}, sender, log.New(io.Discard))(ctx, occurrence)
	if !errors.Is(err, core.ErrNotificationAlertSuppressed) || sender.calls != 0 {
		t.Fatalf("transferred handler = (%v, %d calls), want suppressed without provider", err, sender.calls)
	}
}
