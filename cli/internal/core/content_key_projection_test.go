package core

import (
	"testing"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestContentKeyProjection_IndexesActiveEpoch(t *testing.T) {
	p := NewContentKeyProjection()

	events := []*corev1.Event{
		{
			Id: "E1",
			Event: &corev1.Event_UserContentKeyGenerated{
				UserContentKeyGenerated: &corev1.UserContentKeyGeneratedEvent{
					UserId:              "U1",
					Epoch:               1,
					EncryptedContentKey: []byte("wrapped-1"),
					ContentKeyNonce:     []byte("nonce-1"),
				},
			},
		},
		{
			Id: "E2",
			Event: &corev1.Event_UserContentKeyGenerated{
				UserContentKeyGenerated: &corev1.UserContentKeyGeneratedEvent{
					UserId:              "U1",
					Epoch:               2,
					EncryptedContentKey: []byte("wrapped-2"),
					ContentKeyNonce:     []byte("nonce-2"),
				},
			},
		},
	}
	for i, event := range events {
		if err := p.Apply(event, uint64(i+1)); err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}

	active, ok := p.Active("U1")
	if !ok {
		t.Fatal("expected active content key")
	}
	if active.GetEpoch() != 2 {
		t.Fatalf("active epoch = %d, want 2", active.GetEpoch())
	}

	epoch1, ok := p.Get("U1", 1)
	if !ok {
		t.Fatal("expected epoch 1")
	}
	if string(epoch1.GetEncryptedContentKey()) != "wrapped-1" {
		t.Fatalf("epoch 1 wrapped key = %q", epoch1.GetEncryptedContentKey())
	}
}

func TestContentKeyProjection_ShredClearsKeys(t *testing.T) {
	p := NewContentKeyProjection()

	if err := p.Apply(&corev1.Event{
		Id: "E1",
		Event: &corev1.Event_UserContentKeyGenerated{
			UserContentKeyGenerated: &corev1.UserContentKeyGeneratedEvent{
				UserId:              "U1",
				Epoch:               1,
				EncryptedContentKey: []byte("wrapped"),
				ContentKeyNonce:     []byte("nonce"),
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

	if _, ok := p.Active("U1"); ok {
		t.Fatal("active content key should be cleared after shred")
	}
	if _, ok := p.Get("U1", 1); ok {
		t.Fatal("epoch 1 content key should be cleared after shred")
	}
}
