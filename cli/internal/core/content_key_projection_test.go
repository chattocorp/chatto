package core

import (
	"bytes"
	"fmt"
	"testing"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestContentKeyProjection_IndexesActiveEpoch(t *testing.T) {
	p := NewContentKeyProjection()
	purpose := evtv1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY

	events := []*evtv1.Event{
		{
			Id: "E1",
			Event: &evtv1.Event_UserDekGenerated{
				UserDekGenerated: &evtv1.UserDEKGeneratedEvent{
					UserId:         "U1",
					Epoch:          1,
					Purpose:        purpose,
					ContentKeyRef:  "dek.1",
					WrappingKeyRef: "kek.1",
				},
			},
		},
		{
			Id: "E2",
			Event: &evtv1.Event_UserDekGenerated{
				UserDekGenerated: &evtv1.UserDEKGeneratedEvent{
					UserId:         "U1",
					Epoch:          2,
					Purpose:        purpose,
					ContentKeyRef:  "dek.2",
					WrappingKeyRef: "kek.1",
				},
			},
		},
	}
	for i, event := range events {
		if err := p.Apply(event, uint64(i+1)); err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}

	active, ok := p.Active("U1", purpose)
	if !ok {
		t.Fatal("expected active content key")
	}
	if active.GetEpoch() != 2 {
		t.Fatalf("active epoch = %d, want 2", active.GetEpoch())
	}

	epoch1, ok := p.Get("U1", purpose, 1)
	if !ok {
		t.Fatal("expected epoch 1")
	}
	if epoch1.GetContentKeyRef() != "dek.1" {
		t.Fatalf("epoch 1 content key ref = %q", epoch1.GetContentKeyRef())
	}
	if epoch1.GetWrappingKeyRef() != "kek.1" {
		t.Fatalf("epoch 1 wrapping key ref = %q", epoch1.GetWrappingKeyRef())
	}
}

func TestContentKeyProjection_ShredRequestPermanentlyClearsKeys(t *testing.T) {
	p := NewContentKeyProjection()
	purpose := evtv1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY

	if err := p.Apply(&evtv1.Event{
		Id: "E1",
		Event: &evtv1.Event_UserDekGenerated{
			UserDekGenerated: &evtv1.UserDEKGeneratedEvent{
				UserId:        "U1",
				Epoch:         1,
				Purpose:       purpose,
				ContentKeyRef: "dek.1",
			},
		},
	}, 1); err != nil {
		t.Fatalf("Apply content key: %v", err)
	}
	if err := p.Apply(&evtv1.Event{
		Id: "E2",
		Event: &evtv1.Event_UserKeyShreddingRequested{
			UserKeyShreddingRequested: &evtv1.UserKeyShreddingRequestedEvent{UserId: "U1"},
		},
	}, 2); err != nil {
		t.Fatalf("Apply shred request: %v", err)
	}
	if err := p.Apply(&evtv1.Event{
		Id: "E3",
		Event: &evtv1.Event_UserDekGenerated{UserDekGenerated: &evtv1.UserDEKGeneratedEvent{
			UserId: "U1", Epoch: 2, Purpose: purpose, ContentKeyRef: "dek.2",
		}},
	}, 3); err != nil {
		t.Fatalf("Apply late content key: %v", err)
	}

	if _, ok := p.Active("U1", purpose); ok {
		t.Fatal("active content key should be cleared after shred")
	}
	if _, ok := p.Get("U1", purpose, 1); ok {
		t.Fatal("epoch 1 content key should be cleared after shred")
	}
	payload, err := p.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	restored := NewContentKeyProjection()
	if err := restored.Restore(payload); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if err := restored.Apply(&evtv1.Event{
		Id: "E4",
		Event: &evtv1.Event_UserDekGenerated{UserDekGenerated: &evtv1.UserDEKGeneratedEvent{
			UserId: "U1", Epoch: 3, Purpose: purpose, ContentKeyRef: "dek.3",
		}},
	}, 4); err != nil {
		t.Fatalf("Apply late content key after restore: %v", err)
	}
	if _, ok := restored.Active("U1", purpose); ok {
		t.Fatal("snapshot restore must preserve the terminal shred boundary")
	}
}

func contentKeyTestEvent(id, userID string, purpose evtv1.UserDEKPurpose, epoch int32) *evtv1.Event {
	return &evtv1.Event{Id: id, Event: &evtv1.Event_UserDekGenerated{UserDekGenerated: &evtv1.UserDEKGeneratedEvent{
		UserId: userID, Purpose: purpose, Epoch: epoch, ContentKeyRef: fmt.Sprintf("dek.%s.%d", userID, epoch),
		WrappingKeyRef: "kek." + userID, WrappingAlgorithm: "aes-kw", WrappingMetadata: []byte{byte(epoch)},
	}}}
}

func TestContentKeyProjection_CompactStorageKeepsKeyFieldsAndIsolation(t *testing.T) {
	p := NewContentKeyProjection()
	body := evtv1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY
	legacy := evtv1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED
	for i, event := range []*evtv1.Event{
		contentKeyTestEvent("E1", "U1", legacy, 1),
		contentKeyTestEvent("E2", "U2", body, 1),
		contentKeyTestEvent("E3", "U2", body, 2),
		{Id: "E4", Event: &evtv1.Event_UserKeyShredded{UserKeyShredded: &evtv1.UserKeyShreddedEvent{UserId: "U1"}}},
		contentKeyTestEvent("E5", "U3", legacy, 1),
	} {
		if err := p.Apply(event, uint64(i+1)); err != nil {
			t.Fatal(err)
		}
	}

	// Shredding U1 leaves the keys of other users.
	if _, ok := p.Get("U1", legacy, 1); ok {
		t.Fatal("shredded user key is still readable")
	}
	active, ok := p.Active("U2", body)
	if !ok || active.GetEpoch() != 2 || active.GetContentKeyRef() != "dek.U2.2" || active.GetWrappingKeyRef() != "kek.U2" ||
		active.GetWrappingAlgorithm() != "aes-kw" || !bytes.Equal(active.GetWrappingMetadata(), []byte{2}) || active.GetUserId() != "U2" || active.GetPurpose() != body {
		t.Fatalf("Active(U2) = %v, %v", active, ok)
	}

	// A purpose without its own keys falls back to the legacy purpose.
	if fallback, ok := p.Active("U3", body); !ok || fallback.GetPurpose() != legacy || fallback.GetEpoch() != 1 {
		t.Fatalf("Active(U3, body) = %v, %v; want legacy epoch 1", fallback, ok)
	}
	if fallback, ok := p.Get("U3", body, 1); !ok || fallback.GetPurpose() != legacy {
		t.Fatalf("Get(U3, body, 1) = %v, %v; want legacy key", fallback, ok)
	}

	// Reads return detached copies.
	active.WrappingMetadata[0] = 99
	active.ContentKeyRef = "changed"
	if again, _ := p.Get("U2", body, 2); again.GetContentKeyRef() != "dek.U2.2" || again.GetWrappingMetadata()[0] != 2 {
		t.Fatalf("stored key changed through a returned copy: %v", again)
	}
}
