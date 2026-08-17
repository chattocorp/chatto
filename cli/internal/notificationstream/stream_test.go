package notificationstream

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func TestPublisherStoresCanonicalEventWithPhysicalTTL(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:        "NOTIFICATIONS_TEST",
		Subjects:    Subjects(),
		Storage:     jetstream.FileStorage,
		AllowMsgTTL: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	event := &corev1.NotificationEvent{
		Id:          "NE1",
		RecipientId: "U1",
		OccurredAt:  timestamppb.New(now),
		ExpiresAt:   timestamppb.New(now.Add(time.Hour)),
		Event: &corev1.NotificationEvent_Read{Read: &corev1.NotificationRead{
			NotificationId: "N1",
			ReadAt:         timestamppb.New(now),
		}},
	}
	publisher := NewPublisher(js, stream, time.Hour, nil)
	publisher.now = func() time.Time { return now }
	position, err := publisher.AppendEventually(ctx, event)
	if err != nil {
		t.Fatalf("AppendEventually: %v", err)
	}
	if position.SubjectFilter != ReadSubject || position.Seq == 0 {
		t.Fatalf("position = %+v, want read subject and non-zero sequence", position)
	}
	stored, err := stream.GetMsg(ctx, position.Seq)
	if err != nil {
		t.Fatal(err)
	}
	want, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Subject != ReadSubject || !bytes.Equal(stored.Data, want) {
		t.Fatalf("stored record = subject %q data %x", stored.Subject, stored.Data)
	}
	if got := stored.Header.Get("Nats-TTL"); got != (2 * time.Hour).String() {
		t.Fatalf("Nats-TTL = %q, want %q", got, (2 * time.Hour).String())
	}
}

func TestPublisherRejectsExpiredAndIncompleteEvents(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	publisher := &Publisher{retentionGrace: time.Hour, now: func() time.Time { return now }}
	ctx := context.Background()

	if _, err := publisher.AppendEventually(ctx, &corev1.NotificationEvent{}); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("incomplete event error = %v, want ErrInvalidEvent", err)
	}
	expired := &corev1.NotificationEvent{
		Id: "NE1", RecipientId: "U1", OccurredAt: timestamppb.New(now.Add(-2 * time.Hour)), ExpiresAt: timestamppb.New(now.Add(-time.Hour)),
		Event: &corev1.NotificationEvent_Read{Read: &corev1.NotificationRead{NotificationId: "N1", ReadAt: timestamppb.New(now)}},
	}
	if _, err := publisher.AppendEventually(ctx, expired); !errors.Is(err, ErrExpiredEvent) {
		t.Fatalf("expired event error = %v, want ErrExpiredEvent", err)
	}
}

func TestNotificationSubjectsAreCompleteAndCallerOwned(t *testing.T) {
	want := []string{SignalledSubject, ReadSubject, DismissedSubject, AlertResolvedSubject}
	got := Subjects()
	if len(got) != len(want) {
		t.Fatalf("Subjects() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Subjects() = %v, want %v", got, want)
		}
	}
	got[0] = "mutated"
	if Subjects()[0] != SignalledSubject {
		t.Fatal("caller mutation changed the notification subject contract")
	}
}
