package core

import (
	"testing"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestContentKeyProjection_IndexesActiveEpoch(t *testing.T) {
	p := NewContentKeyProjection()
	purpose := corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY

	events := []*corev1.Event{
		{
			Id: "E1",
			Event: &corev1.Event_UserDekGenerated{
				UserDekGenerated: &corev1.UserDEKGeneratedEvent{
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
			Event: &corev1.Event_UserDekGenerated{
				UserDekGenerated: &corev1.UserDEKGeneratedEvent{
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

	contentKeyRefs := p.ContentKeyRefs("U1")
	if len(contentKeyRefs) != 2 {
		t.Fatalf("content key refs = %v, want 2 refs", contentKeyRefs)
	}
	keyRefs := p.KeyRefs("U1")
	if len(keyRefs) != 1 || keyRefs[0] != "kek.1" {
		t.Fatalf("wrapping key refs = %v, want [kek.1]", keyRefs)
	}
}

func TestContentKeyProjection_ShredClearsKeys(t *testing.T) {
	p := NewContentKeyProjection()
	purpose := corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY

	if err := p.Apply(&corev1.Event{
		Id: "E1",
		Event: &corev1.Event_UserDekGenerated{
			UserDekGenerated: &corev1.UserDEKGeneratedEvent{
				UserId:        "U1",
				Epoch:         1,
				Purpose:       purpose,
				ContentKeyRef: "dek.1",
			},
		},
	}, 1); err != nil {
		t.Fatalf("Apply content key: %v", err)
	}
	if err := p.Apply(&corev1.Event{
		Id: "E2",
		Event: &corev1.Event_UserKeyShredded{
			UserKeyShredded: &corev1.UserKeyShreddedEvent{UserId: "U1"},
		},
	}, 2); err != nil {
		t.Fatalf("Apply shred: %v", err)
	}

	if _, ok := p.Active("U1", purpose); ok {
		t.Fatal("active content key should be cleared after shred")
	}
	if _, ok := p.Get("U1", purpose, 1); ok {
		t.Fatal("epoch 1 content key should be cleared after shred")
	}
	if refs := p.ContentKeyRefs("U1"); len(refs) != 0 {
		t.Fatalf("content key refs should be cleared after shred, got %v", refs)
	}
}
