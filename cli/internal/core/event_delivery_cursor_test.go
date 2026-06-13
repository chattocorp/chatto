package core

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEventDeliveryCursor_RoundTripAndRejectsTampering(t *testing.T) {
	core, _ := setupTestCore(t)
	now := time.Unix(1_780_000_000, 0)

	cursor := core.FormatEventDeliveryCursor("U1", 42, now)
	if cursor == "" {
		t.Fatal("FormatEventDeliveryCursor returned empty cursor")
	}
	if strings.Contains(cursor, "42") || strings.Contains(cursor, "U1") {
		t.Fatalf("cursor should be opaque, got %q", cursor)
	}

	seq, err := core.ParseEventDeliveryCursor("U1", cursor, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ParseEventDeliveryCursor: %v", err)
	}
	if seq != 42 {
		t.Fatalf("seq = %d, want 42", seq)
	}

	if _, err := core.ParseEventDeliveryCursor("U2", cursor, now.Add(time.Minute)); !errors.Is(err, ErrEventDeliveryCursorInvalid) {
		t.Fatalf("wrong user parse error = %v, want ErrEventDeliveryCursorInvalid", err)
	}

	tampered := cursor[:len(cursor)-1] + "A"
	if _, err := core.ParseEventDeliveryCursor("U1", tampered, now.Add(time.Minute)); !errors.Is(err, ErrEventDeliveryCursorInvalid) {
		t.Fatalf("tampered parse error = %v, want ErrEventDeliveryCursorInvalid", err)
	}

	if _, err := core.ParseEventDeliveryCursor("U1", cursor, now.Add(eventDeliveryCursorMaxAge+time.Second)); !errors.Is(err, ErrEventDeliveryCursorExpired) {
		t.Fatalf("expired parse error = %v, want ErrEventDeliveryCursorExpired", err)
	}
}

func TestEventDeliveryCursor_RejectsRawSequenceCursor(t *testing.T) {
	core, _ := setupTestCore(t)

	if _, err := core.ParseEventDeliveryCursor("U1", "seq:1", time.Now()); !errors.Is(err, ErrEventDeliveryCursorInvalid) {
		t.Fatalf("raw sequence cursor parse error = %v, want ErrEventDeliveryCursorInvalid", err)
	}
}
