package core

import (
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
