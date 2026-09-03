package core

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// EventPlaintext contains decrypted values that can be added to an authorized
// public event. The values never become part of the canonical EVT message.
type EventPlaintext struct {
	MessageBody *string
	Login       *string
	DisplayName *string
	Bio         *string
}

// ResolveEventPlaintext decrypts values for an authorized public event.
// Callers must authorize the event before they expose the returned values. A
// field stays nil after key shredding or when the event does not carry it.
func (c *ChattoCore) ResolveEventPlaintext(ctx context.Context, event *evtv1.Event) (*EventPlaintext, error) {
	plaintext := &EventPlaintext{}
	if c == nil || c.userModel == nil || c.userModel.users.Projection() == nil || event == nil {
		return plaintext, nil
	}
	projection := c.userModel.users.Projection()
	set := func(userID, eventType, purpose string, encrypted *evtv1.EncryptedUserString, target **string) error {
		plaintext, ok, err := projection.decryptEventPII(ctx, event.GetId(), userID, eventType, purpose, encrypted)
		if err != nil {
			return err
		}
		if ok {
			*target = &plaintext
		}
		return nil
	}

	switch payload := event.GetEvent().(type) {
	case *evtv1.Event_MessagePosted:
		body, err := c.GetFullMessageBody(ctx, event.GetId())
		if err != nil {
			return nil, fmt.Errorf("resolve posted-message plaintext: %w", err)
		}
		if body != nil {
			plaintext.MessageBody = &body.Body
		}
	case *evtv1.Event_UserAccountCreated:
		userID := payload.UserAccountCreated.GetUserId()
		if err := set(userID, evtstream.EventUserAccountCreated, "login", payload.UserAccountCreated.GetEncryptedLogin(), &plaintext.Login); err != nil {
			return nil, fmt.Errorf("resolve account login plaintext: %w", err)
		}
		if err := set(userID, evtstream.EventUserAccountCreated, "display_name", payload.UserAccountCreated.GetEncryptedDisplayName(), &plaintext.DisplayName); err != nil {
			return nil, fmt.Errorf("resolve account display-name plaintext: %w", err)
		}
	case *evtv1.Event_UserLoginChanged:
		if err := set(payload.UserLoginChanged.GetUserId(), evtstream.EventUserLoginChanged, "login", payload.UserLoginChanged.GetEncryptedLogin(), &plaintext.Login); err != nil {
			return nil, fmt.Errorf("resolve login plaintext: %w", err)
		}
	case *evtv1.Event_UserDisplayNameChanged:
		if err := set(payload.UserDisplayNameChanged.GetUserId(), evtstream.EventUserDisplayNameChanged, "display_name", payload.UserDisplayNameChanged.GetEncryptedDisplayName(), &plaintext.DisplayName); err != nil {
			return nil, fmt.Errorf("resolve display-name plaintext: %w", err)
		}
	case *evtv1.Event_UserBioChanged:
		if err := set(payload.UserBioChanged.GetUserId(), evtstream.EventUserBioChanged, "bio", payload.UserBioChanged.GetEncryptedBio(), &plaintext.Bio); err != nil {
			return nil, fmt.Errorf("resolve bio plaintext: %w", err)
		}
	}
	return plaintext, nil
}
