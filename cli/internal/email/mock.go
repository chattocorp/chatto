package email

import (
	"context"
	"sync"
)

// MockSender is a test implementation of Sender that captures sent messages.
// It is also a Queue that renders each delivery at once with Branding, so
// tests can check rendered email without running the queue worker.
type MockSender struct {
	// Branding supplies the branding for Enqueue. When it is nil, Enqueue uses
	// the server name "Chatto" and no server URL.
	Branding func() Branding

	enabled  bool
	messages []Message
	mu       sync.Mutex
	// SendError can be set to simulate send failures
	SendError error
}

// NewMockSender creates a new MockSender.
// If enabled is true, IsEnabled() returns true and Send() captures messages.
// If enabled is false, IsEnabled() returns false and Send() returns ErrEmailDisabled.
func NewMockSender(enabled bool) *MockSender {
	return &MockSender{enabled: enabled}
}

// Send captures the message if enabled, or returns ErrEmailDisabled.
func (m *MockSender) Send(msg Message) error {
	return m.SendContext(context.Background(), msg)
}

// SendContext captures the message if enabled, or returns ErrEmailDisabled.
func (m *MockSender) SendContext(_ context.Context, msg Message) error {
	if !m.enabled {
		return ErrEmailDisabled
	}
	if m.SendError != nil {
		return m.SendError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	return nil
}

// Enqueue renders the delivery and captures it as a sent message.
func (m *MockSender) Enqueue(ctx context.Context, delivery Delivery) error {
	branding := Branding{ServerName: "Chatto"}
	if m.Branding != nil {
		branding = m.Branding()
	}
	msg, err := Render(branding, delivery.To, delivery.Content)
	if err != nil {
		return err
	}
	return m.SendContext(ctx, msg)
}

// IsEnabled returns whether the mock is configured as enabled.
func (m *MockSender) IsEnabled() bool {
	return m.enabled
}

// Messages returns all captured messages.
func (m *MockSender) Messages() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return a copy to avoid race conditions
	result := make([]Message, len(m.messages))
	copy(result, m.messages)
	return result
}

// LastMessage returns the most recently sent message, or nil if none.
func (m *MockSender) LastMessage() *Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.messages) == 0 {
		return nil
	}
	msg := m.messages[len(m.messages)-1]
	return &msg
}

// Reset clears all captured messages.
func (m *MockSender) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = nil
}

// Verify MockSender implements Sender and Queue at compile time.
var (
	_ Sender = (*MockSender)(nil)
	_ Queue  = (*MockSender)(nil)
)
