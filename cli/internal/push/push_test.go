package push

import (
	"context"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/charmbracelet/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/config"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestNewSender(t *testing.T) {
	logger := log.New(nil)

	t.Run("returns nil when not configured", func(t *testing.T) {
		cfg := config.PushConfig{}
		sender := NewSender(cfg, logger)
		if sender != nil {
			t.Error("Expected nil sender when not configured")
		}
	})

	t.Run("returns nil when enabled but missing keys", func(t *testing.T) {
		cfg := config.PushConfig{
			Enabled: true,
			// Missing VAPID keys
		}
		sender := NewSender(cfg, logger)
		if sender != nil {
			t.Error("Expected nil sender when keys missing")
		}
	})

	t.Run("returns sender when fully configured", func(t *testing.T) {
		cfg := config.PushConfig{
			Enabled:         true,
			VAPIDPublicKey:  "test-public-key",
			VAPIDPrivateKey: "test-private-key",
			VAPIDSubject:    "mailto:test@example.com",
		}
		sender := NewSender(cfg, logger)
		if sender == nil {
			t.Error("Expected non-nil sender when configured")
		}
	})
}

func TestPayloadMarshal(t *testing.T) {
	t.Run("marshals all fields", func(t *testing.T) {
		payload := &Payload{
			Title:          "Test Title",
			Body:           "Test Body",
			Icon:           "/icons/icon.png",
			Badge:          "/icons/badge.png",
			Tag:            "test-tag",
			NotificationID: "notif-123",
			URL:            "/chat/room/123",
		}

		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		// Unmarshal and verify
		var result map[string]interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if result["title"] != "Test Title" {
			t.Errorf("Expected title 'Test Title', got %v", result["title"])
		}
		if result["notificationId"] != "notif-123" {
			t.Errorf("Expected notificationId 'notif-123', got %v", result["notificationId"])
		}
		if result["url"] != "/chat/room/123" {
			t.Errorf("Expected url '/chat/room/123', got %v", result["url"])
		}
	})

	t.Run("omits empty optional fields", func(t *testing.T) {
		payload := &Payload{
			Title: "Test Title",
			Body:  "Test Body",
		}

		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if _, ok := result["icon"]; ok {
			t.Error("Expected icon to be omitted when empty")
		}
		if _, ok := result["notificationId"]; ok {
			t.Error("Expected notificationId to be omitted when empty")
		}
	})
}

func TestNormalizeVAPIDSubject(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		want    string
	}{
		{
			name:    "strips mailto prefix",
			subject: "mailto:admin@example.com",
			want:    "admin@example.com",
		},
		{
			name:    "keeps bare email",
			subject: "admin@example.com",
			want:    "admin@example.com",
		},
		{
			name:    "keeps https URL",
			subject: "https://example.com/push-contact",
			want:    "https://example.com/push-contact",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeVAPIDSubject(tt.subject); got != tt.want {
				t.Fatalf("normalizeVAPIDSubject(%q) = %q, want %q", tt.subject, got, tt.want)
			}
		})
	}
}

func TestBuildPayloadFromNotification(t *testing.T) {
	baseURL := "https://chatto.example.com"

	t.Run("builds DM message payload without context", func(t *testing.T) {
		notif := &corev1.Notification{
			Id:          "notif-123",
			RecipientId: "user-1",
			ActorId:     "user-2",
			CreatedAt:   timestamppb.Now(),
			Notification: &corev1.Notification_DmMessage{
				DmMessage: &corev1.DMMessageNotification{
					RoomId:  "dm-room-456",
					EventId: "event-789",
				},
			},
		}

		payload := BuildPayloadFromNotification(notif, "Alice", baseURL, nil)

		if payload.Title != "@Alice sent you a new DM" {
			t.Errorf("Expected '@Alice sent you a new DM', got %s", payload.Title)
		}
		if payload.Body != "" {
			t.Errorf("Expected empty body, got %s", payload.Body)
		}
		if payload.Tag != "dm-event-789" {
			t.Errorf("Expected tag 'dm-event-789', got %s", payload.Tag)
		}
		if payload.URL != "https://chatto.example.com/chat/-/dm-room-456" {
			t.Errorf("Expected URL for DM room, got %s", payload.URL)
		}
		if payload.NotificationID != "notif-123" {
			t.Errorf("Expected notificationId 'notif-123', got %s", payload.NotificationID)
		}
	})

	t.Run("builds DM message payload with preview", func(t *testing.T) {
		notif := &corev1.Notification{
			Id: "notif-123",
			Notification: &corev1.Notification_DmMessage{
				DmMessage: &corev1.DMMessageNotification{
					RoomId: "dm-room-456",
				},
			},
		}
		ctx := &PayloadContext{MessagePreview: "Hey, how are you?"}

		payload := BuildPayloadFromNotification(notif, "Alice", baseURL, ctx)

		if payload.Title != "@Alice sent you a new DM" {
			t.Errorf("Expected '@Alice sent you a new DM', got %s", payload.Title)
		}
		if payload.Body != "Hey, how are you?" {
			t.Errorf("Expected 'Hey, how are you?', got %s", payload.Body)
		}
	})

	t.Run("builds mention payload without context", func(t *testing.T) {
		notif := &corev1.Notification{
			Id: "notif-456",
			Notification: &corev1.Notification_Mention{
				Mention: &corev1.MentionNotification{
					RoomId:  "room-2",
					EventId: "event-3",
				},
			},
		}

		payload := BuildPayloadFromNotification(notif, "Bob", baseURL, nil)

		if payload.Title != "@Bob mentioned you" {
			t.Errorf("Expected '@Bob mentioned you', got %s", payload.Title)
		}
		if payload.Body != "" {
			t.Errorf("Expected empty body, got %s", payload.Body)
		}
		if payload.URL != "https://chatto.example.com/chat/-/room-2?highlight=event-3" {
			t.Errorf("Expected URL with highlight param, got %s", payload.URL)
		}
	})

	t.Run("builds mention payload with room name and preview", func(t *testing.T) {
		notif := &corev1.Notification{
			Id: "notif-456",
			Notification: &corev1.Notification_Mention{
				Mention: &corev1.MentionNotification{
					RoomId:  "room-2",
					EventId: "event-3",
				},
			},
		}
		ctx := &PayloadContext{MessagePreview: "Hey @Bob check this out", RoomName: "general"}

		payload := BuildPayloadFromNotification(notif, "Alice", baseURL, ctx)

		if payload.Title != "@Alice mentioned you in #general" {
			t.Errorf("Expected '@Alice mentioned you in #general', got %s", payload.Title)
		}
		if payload.Body != "Hey @Bob check this out" {
			t.Errorf("Expected 'Hey @Bob check this out', got %s", payload.Body)
		}
	})

	t.Run("builds mention payload without event ID", func(t *testing.T) {
		notif := &corev1.Notification{
			Id: "notif-789",
			Notification: &corev1.Notification_Mention{
				Mention: &corev1.MentionNotification{
					RoomId: "room-2",
					// No EventId
				},
			},
		}

		payload := BuildPayloadFromNotification(notif, "Charlie", baseURL, nil)

		if payload.URL != "https://chatto.example.com/chat/-/room-2" {
			t.Errorf("Expected URL without event param, got %s", payload.URL)
		}
	})

	t.Run("builds room-level reply payload without context", func(t *testing.T) {
		notif := &corev1.Notification{
			Id: "notif-abc",
			Notification: &corev1.Notification_Reply{
				Reply: &corev1.ReplyNotification{
					RoomId:      "room-y",
					EventId:     "reply-event",
					InReplyToId: "root-event",
					// InThread empty — room-level reply
				},
			},
		}

		payload := BuildPayloadFromNotification(notif, "Diana", baseURL, nil)

		if payload.Title != "@Diana replied to you" {
			t.Errorf("Expected '@Diana replied to you', got %s", payload.Title)
		}
		if payload.Body != "" {
			t.Errorf("Expected empty body, got %s", payload.Body)
		}
		if payload.Tag != "reply-reply-event" {
			t.Errorf("Expected tag 'reply-reply-event', got %s", payload.Tag)
		}
		// Room-level reply navigates to room with highlight
		if payload.URL != "https://chatto.example.com/chat/-/room-y?highlight=reply-event" {
			t.Errorf("Expected URL for room with highlight, got %s", payload.URL)
		}
	})

	t.Run("builds thread reply payload without context", func(t *testing.T) {
		notif := &corev1.Notification{
			Id: "notif-abc",
			Notification: &corev1.Notification_Reply{
				Reply: &corev1.ReplyNotification{
					RoomId:      "room-y",
					EventId:     "reply-event",
					InReplyToId: "mid-thread-msg", // The specific message replied to (not the root)
					InThread:    "thread-root",    // The actual thread root
				},
			},
		}

		payload := BuildPayloadFromNotification(notif, "Diana", baseURL, nil)

		if payload.Title != "@Diana replied to you" {
			t.Errorf("Expected '@Diana replied to you', got %s", payload.Title)
		}
		// Thread reply: navigate to thread root and highlight the replied-to message
		expectedURL := "https://chatto.example.com/chat/-/room-y/thread-root?highlight=mid-thread-msg"
		if payload.URL != expectedURL {
			t.Errorf("Expected URL %s, got %s", expectedURL, payload.URL)
		}
	})

	t.Run("builds reply payload with preview", func(t *testing.T) {
		notif := &corev1.Notification{
			Id: "notif-abc",
			Notification: &corev1.Notification_Reply{
				Reply: &corev1.ReplyNotification{
					RoomId:      "room-y",
					EventId:     "reply-event",
					InReplyToId: "root-event",
				},
			},
		}
		ctx := &PayloadContext{MessagePreview: "Thanks for the update!"}

		payload := BuildPayloadFromNotification(notif, "Diana", baseURL, ctx)

		if payload.Title != "@Diana replied to you" {
			t.Errorf("Expected '@Diana replied to you', got %s", payload.Title)
		}
		if payload.Body != "Thanks for the update!" {
			t.Errorf("Expected 'Thanks for the update!', got %s", payload.Body)
		}
	})

	t.Run("builds reply payload with room name and preview", func(t *testing.T) {
		notif := &corev1.Notification{
			Id: "notif-abc",
			Notification: &corev1.Notification_Reply{
				Reply: &corev1.ReplyNotification{
					RoomId:      "room-y",
					EventId:     "reply-event",
					InReplyToId: "root-event",
				},
			},
		}
		ctx := &PayloadContext{MessagePreview: "Thanks for the update!", RoomName: "general"}

		payload := BuildPayloadFromNotification(notif, "Diana", baseURL, ctx)

		if payload.Title != "@Diana replied to you in #general" {
			t.Errorf("Expected '@Diana replied to you in #general', got %s", payload.Title)
		}
		if payload.Body != "Thanks for the update!" {
			t.Errorf("Expected 'Thanks for the update!', got %s", payload.Body)
		}
	})

	t.Run("builds default payload for unknown type", func(t *testing.T) {
		notif := &corev1.Notification{
			Id: "notif-unknown",
			// No notification type set
		}

		payload := BuildPayloadFromNotification(notif, "Unknown", baseURL, nil)

		if payload.Title != "New notification" {
			t.Errorf("Expected 'New notification', got %s", payload.Title)
		}
		if payload.Body != "You have a new notification" {
			t.Errorf("Unexpected body: %s", payload.Body)
		}
	})

	t.Run("sets icon and badge URLs", func(t *testing.T) {
		notif := &corev1.Notification{
			Id: "notif-icons",
			Notification: &corev1.Notification_DmMessage{
				DmMessage: &corev1.DMMessageNotification{RoomId: "room"},
			},
		}

		payload := BuildPayloadFromNotification(notif, "Test", baseURL, nil)

		expectedIcon := "https://chatto.example.com/icons/icon-192.png"
		if payload.Icon != expectedIcon {
			t.Errorf("Expected icon %s, got %s", expectedIcon, payload.Icon)
		}
		if payload.Badge != expectedIcon {
			t.Errorf("Expected badge %s, got %s", expectedIcon, payload.Badge)
		}
	})

	t.Run("truncates long message preview", func(t *testing.T) {
		notif := &corev1.Notification{
			Id: "notif-long",
			Notification: &corev1.Notification_DmMessage{
				DmMessage: &corev1.DMMessageNotification{RoomId: "room"},
			},
		}
		// Create a preview longer than maxPreviewLength
		longPreview := "This is a very long message that exceeds the maximum preview length and should be truncated with an ellipsis at the end to fit within the allowed characters"
		ctx := &PayloadContext{MessagePreview: longPreview}

		payload := BuildPayloadFromNotification(notif, "Test", baseURL, ctx)

		// Body should be truncated (just the preview, no prefix)
		if len(payload.Body) > maxPreviewLength+3 { // +3 for ellipsis
			t.Errorf("Body too long: %d chars", len(payload.Body))
		}
		if !strings.HasSuffix(payload.Body, "…") {
			t.Errorf("Expected body to end with ellipsis, got %s", payload.Body)
		}
	})
}

func TestNotificationTag(t *testing.T) {
	t.Run("returns DM tag with event ID", func(t *testing.T) {
		notif := &corev1.Notification{
			Notification: &corev1.Notification_DmMessage{
				DmMessage: &corev1.DMMessageNotification{RoomId: "room-123", EventId: "event-abc"},
			},
		}
		tag := NotificationTag(notif)
		if tag != "dm-event-abc" {
			t.Errorf("Expected 'dm-event-abc', got %s", tag)
		}
	})

	t.Run("returns mention tag with event ID", func(t *testing.T) {
		notif := &corev1.Notification{
			Notification: &corev1.Notification_Mention{
				Mention: &corev1.MentionNotification{RoomId: "room-456", EventId: "event-def"},
			},
		}
		tag := NotificationTag(notif)
		if tag != "mention-event-def" {
			t.Errorf("Expected 'mention-event-def', got %s", tag)
		}
	})

	t.Run("returns reply tag with event ID", func(t *testing.T) {
		notif := &corev1.Notification{
			Notification: &corev1.Notification_Reply{
				Reply: &corev1.ReplyNotification{RoomId: "room-789", EventId: "event-ghi"},
			},
		}
		tag := NotificationTag(notif)
		if tag != "reply-event-ghi" {
			t.Errorf("Expected 'reply-event-ghi', got %s", tag)
		}
	})

	t.Run("returns empty for unknown type", func(t *testing.T) {
		notif := &corev1.Notification{}
		tag := NotificationTag(notif)
		if tag != "" {
			t.Errorf("Expected empty string, got %s", tag)
		}
	})
}

func TestTruncatePreview(t *testing.T) {
	t.Run("returns short text unchanged", func(t *testing.T) {
		text := "Hello world"
		result := truncatePreview(text)
		if result != text {
			t.Errorf("Expected '%s', got '%s'", text, result)
		}
	})

	t.Run("truncates at word boundary", func(t *testing.T) {
		// Create text just over the limit
		text := "This is a test message that is slightly longer than one hundred characters and should be truncated properly"
		result := truncatePreview(text)

		if len(result) > maxPreviewLength+3 { // +3 for ellipsis rune
			t.Errorf("Result too long: %d chars", len(result))
		}
		if !strings.HasSuffix(result, "…") {
			t.Errorf("Expected ellipsis at end")
		}
	})
}

func TestSendResult(t *testing.T) {
	t.Run("result fields", func(t *testing.T) {
		result := &SendResult{
			Endpoint: "https://push.example.com/endpoint",
			Success:  true,
			Error:    nil,
			Gone:     false,
		}

		if result.Endpoint != "https://push.example.com/endpoint" {
			t.Error("Endpoint not set correctly")
		}
		if !result.Success {
			t.Error("Success should be true")
		}
		if result.Gone {
			t.Error("Gone should be false")
		}
	})
}

func TestSend(t *testing.T) {
	t.Run("sends compact encrypted request", func(t *testing.T) {
		var bodyLen int
		var contentEncoding string
		var ttl string
		var readErr error

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body []byte
			body, readErr = io.ReadAll(r.Body)
			if readErr != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			bodyLen = len(body)
			contentEncoding = r.Header.Get("Content-Encoding")
			ttl = r.Header.Get("TTL")
			w.WriteHeader(http.StatusCreated)
		}))
		defer server.Close()

		sender := newTestSender(t, server.Client())
		result := sender.Send(context.Background(), newTestPushSubscription(t, server.URL), &Payload{
			Title: "Test",
			Body:  "Test body",
		})

		if result.Error != nil {
			t.Fatalf("Send error: %v", result.Error)
		}
		if readErr != nil {
			t.Fatalf("ReadAll request body: %v", readErr)
		}
		if !result.Success {
			t.Fatal("expected success")
		}
		if bodyLen != int(pushRecordSize) {
			t.Fatalf("request body length = %d, want %d", bodyLen, pushRecordSize)
		}
		if bodyLen >= 4096 {
			t.Fatalf("request body length = %d, want under 4096", bodyLen)
		}
		if contentEncoding != "aes128gcm" {
			t.Fatalf("Content-Encoding = %q, want aes128gcm", contentEncoding)
		}
		if ttl != "86400" {
			t.Fatalf("TTL = %q, want 86400", ttl)
		}
	})

	t.Run("includes provider response body for non-gone failures", func(t *testing.T) {
		tests := []struct {
			name       string
			statusCode int
			body       string
		}{
			{name: "apple forbidden", statusCode: http.StatusForbidden, body: "invalid VAPID subject"},
			{name: "mozilla too large", statusCode: http.StatusRequestEntityTooLarge, body: "payload too large"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tt.statusCode)
					_, _ = w.Write([]byte(tt.body))
				}))
				defer server.Close()

				sender := newTestSender(t, server.Client())
				result := sender.Send(context.Background(), newTestPushSubscription(t, server.URL), &Payload{
					Title: "Test",
				})

				if result.Error == nil {
					t.Fatal("expected error")
				}
				if result.Gone {
					t.Fatal("expected non-gone failure")
				}
				if !strings.Contains(result.Error.Error(), tt.body) {
					t.Fatalf("error %q does not contain provider body %q", result.Error, tt.body)
				}
				if !strings.Contains(result.Error.Error(), strconv.Itoa(tt.statusCode)) {
					t.Fatalf("error %q does not contain status %d", result.Error, tt.statusCode)
				}
			})
		}
	})

	t.Run("marks missing and gone subscriptions as gone", func(t *testing.T) {
		tests := []struct {
			name       string
			statusCode int
		}{
			{name: "not found", statusCode: http.StatusNotFound},
			{name: "gone", statusCode: http.StatusGone},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tt.statusCode)
					_, _ = w.Write([]byte("subscription is gone"))
				}))
				defer server.Close()

				sender := newTestSender(t, server.Client())
				result := sender.Send(context.Background(), newTestPushSubscription(t, server.URL), &Payload{
					Title: "Test",
				})

				if result.Error == nil {
					t.Fatal("expected error")
				}
				if !result.Gone {
					t.Fatal("expected gone result")
				}
			})
		}
	})
}

// TestSendToMany tests the SendToMany method with invalid subscription material.
func TestSendToMany(t *testing.T) {
	// Create a mock push service that returns various statuses
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch callCount {
		case 1:
			w.WriteHeader(http.StatusCreated) // Success
		case 2:
			w.WriteHeader(http.StatusGone) // Subscription expired
		case 3:
			w.WriteHeader(http.StatusInternalServerError) // Server error
		}
	}))
	defer server.Close()

	t.Run("SendToMany returns results for each subscription", func(t *testing.T) {
		logger := log.New(nil)
		cfg := config.PushConfig{
			Enabled:         true,
			VAPIDPublicKey:  "BNJxJYL1iGKBrQaZHCbYoCHjFzHY3JLNbN6jl0GCvYpJxQKmJe_J6_yNKLlZj3xBLt0EZLsHx2XUcJDBj5vWVKY",
			VAPIDPrivateKey: "LxZ3z5E3D3X3Y3Z3A3B3C3D3E3F3G3H3I3J3K3L3M3N",
			VAPIDSubject:    "mailto:test@example.com",
		}
		sender := NewSender(cfg, logger)
		if sender == nil {
			t.Skip("Sender creation failed (expected with test keys)")
		}

		subscriptions := []*corev1.PushSubscription{
			{Endpoint: server.URL + "/1", P256Dh: "key1", Auth: "auth1"},
			{Endpoint: server.URL + "/2", P256Dh: "key2", Auth: "auth2"},
		}
		payload := &Payload{Title: "Test", Body: "Test body"}

		results := sender.SendToMany(context.Background(), subscriptions, payload)

		if len(results) != len(subscriptions) {
			t.Errorf("Expected %d results, got %d", len(subscriptions), len(results))
		}
	})
}

func newTestSender(t *testing.T, client webpush.HTTPClient) *Sender {
	t.Helper()

	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}

	sender := NewSender(config.PushConfig{
		Enabled:         true,
		VAPIDPublicKey:  publicKey,
		VAPIDPrivateKey: privateKey,
		VAPIDSubject:    "mailto:test@example.com",
	}, log.New(nil))
	if sender == nil {
		t.Fatal("expected configured sender")
	}
	sender.httpClient = client
	return sender
}

func newTestPushSubscription(t *testing.T, endpoint string) *corev1.PushSubscription {
	t.Helper()

	_, x, y, err := elliptic.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatalf("Read auth: %v", err)
	}

	return &corev1.PushSubscription{
		Endpoint: endpoint,
		P256Dh:   base64.RawURLEncoding.EncodeToString(elliptic.Marshal(elliptic.P256(), x, y)),
		Auth:     base64.RawURLEncoding.EncodeToString(auth),
	}
}
