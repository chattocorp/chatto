package evtstream

import (
	"errors"
	"strings"
	"testing"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	livev1 "hmans.de/chatto/internal/pb/chatto/core/live/v1"
)

func TestValidateEventRejectsClientOnlyFields(t *testing.T) {
	plaintext := "public-login"
	event := &evtv1.Event{
		Id: "event-id",
		Event: &evtv1.Event_UserLoginChanged{UserLoginChanged: &evtv1.UserLoginChangedEvent{
			UserId:         "user-id",
			LoginPlaintext: &plaintext,
		}},
	}

	err := validateEvent(event)
	if !errors.Is(err, ErrInvalidEvent) || !strings.Contains(err.Error(), "login_plaintext") {
		t.Fatalf("validateEvent() error = %v, want client-only field rejection", err)
	}
}

func TestValidateEventRejectsTransientVariants(t *testing.T) {
	event := &evtv1.Event{
		Id: "event-id",
		Event: &evtv1.Event_UserTypingSignal{UserTypingSignal: &livev1.UserTypingEvent{
			RoomId: "room-id",
		}},
	}

	err := validateEvent(event)
	if !errors.Is(err, ErrInvalidEvent) || !strings.Contains(err.Error(), "not a durable EVT event type") {
		t.Fatalf("validateEvent() error = %v, want transient event rejection", err)
	}
}
