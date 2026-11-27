package email

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The fake queue must reject what the real queue rejects, so that handler
// tests fail when a caller sends a delivery that production refuses.
func TestMockSenderEnqueueMatchesQueueChecks(t *testing.T) {
	content := RegistrationCode{Code: "123456", TTL: time.Minute}
	if err := NewMockSender(false).Enqueue(context.Background(), Delivery{To: "a@example.test", Content: content, ExpiresAt: time.Now().Add(time.Minute)}); !errors.Is(err, ErrEmailDisabled) {
		t.Errorf("disabled Enqueue error = %v, want ErrEmailDisabled", err)
	}
	mock := NewMockSender(true)
	if err := mock.Enqueue(context.Background(), Delivery{To: "a@example.test", Content: content}); err == nil {
		t.Error("Enqueue without a deadline succeeded")
	}
	mock.Branding = func() Branding { return Branding{ServerName: "Acme"} }
	if err := mock.Enqueue(context.Background(), Delivery{To: "a@example.test", Content: content, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if msg := mock.LastMessage(); msg == nil || msg.Subject != "123456 is your code for Acme" {
		t.Errorf("captured message = %+v", msg)
	}
}
