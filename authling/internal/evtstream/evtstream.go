// Package evtstream adapts Authling's protobuf event contract to the shared
// envelope-neutral event framework.
package evtstream

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	accountSubjectPrefix = "authling.evt.account."
	// AccountSubjectFilter contains every account aggregate.
	AccountSubjectFilter = accountSubjectPrefix + "*"
)

// Publisher validates and appends Authling events through the shared event log.
type Publisher struct {
	log *events.EncodedEventLog
}

// NewPublisher constructs an Authling protobuf publisher.
func NewPublisher(log *events.EncodedEventLog) *Publisher {
	return &Publisher{log: log}
}

// AppendAccountCreated creates a new account aggregate at expected sequence
// zero. A non-zero tail is returned as an events.ErrConflict.
func (p *Publisher) AppendAccountCreated(
	ctx context.Context,
	event *corev1.Event,
) (events.StreamPosition, error) {
	payload := event.GetAccountCreated()
	if payload == nil {
		return events.StreamPosition{}, fmt.Errorf("append account created: event payload is not account_created")
	}
	subject, err := AccountSubject(payload.GetAccountId())
	if err != nil {
		return events.StreamPosition{}, err
	}
	record, err := encode(event)
	if err != nil {
		return events.StreamPosition{}, err
	}
	sequence, err := p.log.AppendAt(ctx, subject, record, 0)
	if err != nil {
		return events.StreamPosition{}, err
	}
	return events.SubjectPosition(subject, sequence), nil
}

// Decode validates and decodes one persisted Authling event.
func Decode(data []byte) (events.DecodedEvent[*corev1.Event], error) {
	var event corev1.Event
	if err := proto.Unmarshal(data, &event); err != nil {
		return events.DecodedEvent[*corev1.Event]{}, fmt.Errorf("decode Authling event: %w", err)
	}
	if err := validate(&event); err != nil {
		return events.DecodedEvent[*corev1.Event]{}, err
	}
	return events.DecodedEvent[*corev1.Event]{Event: &event, ID: event.GetId()}, nil
}

func encode(event *corev1.Event) (events.EncodedRecord, error) {
	if err := validate(event); err != nil {
		return events.EncodedRecord{}, err
	}
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(event)
	if err != nil {
		return events.EncodedRecord{}, fmt.Errorf("encode Authling event: %w", err)
	}
	return events.EncodedRecord{ID: event.GetId(), Data: data}, nil
}

func validate(event *corev1.Event) error {
	if event == nil {
		return fmt.Errorf("Authling event is nil")
	}
	if strings.TrimSpace(event.GetId()) == "" {
		return fmt.Errorf("Authling event id is required")
	}
	if event.GetCreatedAt() == nil {
		return fmt.Errorf("Authling event created_at is required")
	}
	if err := event.GetCreatedAt().CheckValid(); err != nil {
		return fmt.Errorf("Authling event created_at: %w", err)
	}
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_AccountCreated:
		if _, err := AccountSubject(payload.AccountCreated.GetAccountId()); err != nil {
			return err
		}
	default:
		return fmt.Errorf("Authling event payload is required")
	}
	return nil
}

// AccountSubject returns the durable subject for one account aggregate.
func AccountSubject(accountID string) (string, error) {
	if !validSubjectToken(accountID) {
		return "", fmt.Errorf("invalid account id")
	}
	return accountSubjectPrefix + accountID, nil
}

func validSubjectToken(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' ||
			char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' ||
			char == '_' ||
			char == '-' {
			continue
		}
		return false
	}
	return true
}
