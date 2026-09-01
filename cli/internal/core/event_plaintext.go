package core

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// PopulateEventPlaintext decrypts client-only companion fields on a delivery
// copy of an Event. Callers must authorize the event before they expose the
// populated copy. The method leaves fields absent after key shredding.
func (c *ChattoCore) PopulateEventPlaintext(ctx context.Context, event *evtv1.Event) error {
	if c == nil || c.userModel == nil || c.userModel.users.Projection() == nil || event == nil {
		return nil
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
			return fmt.Errorf("populate posted-message plaintext: %w", err)
		}
		if body != nil {
			payload.MessagePosted.BodyPlaintext = &body.Body
		}
	case *evtv1.Event_UserAccountCreated:
		userID := payload.UserAccountCreated.GetUserId()
		if err := set(userID, evtstream.EventUserAccountCreated, "login", payload.UserAccountCreated.GetEncryptedLogin(), &payload.UserAccountCreated.LoginPlaintext); err != nil {
			return fmt.Errorf("populate account login plaintext: %w", err)
		}
		if err := set(userID, evtstream.EventUserAccountCreated, "display_name", payload.UserAccountCreated.GetEncryptedDisplayName(), &payload.UserAccountCreated.DisplayNamePlaintext); err != nil {
			return fmt.Errorf("populate account display-name plaintext: %w", err)
		}
	case *evtv1.Event_UserLoginChanged:
		if err := set(payload.UserLoginChanged.GetUserId(), evtstream.EventUserLoginChanged, "login", payload.UserLoginChanged.GetEncryptedLogin(), &payload.UserLoginChanged.LoginPlaintext); err != nil {
			return fmt.Errorf("populate login plaintext: %w", err)
		}
	case *evtv1.Event_UserDisplayNameChanged:
		if err := set(payload.UserDisplayNameChanged.GetUserId(), evtstream.EventUserDisplayNameChanged, "display_name", payload.UserDisplayNameChanged.GetEncryptedDisplayName(), &payload.UserDisplayNameChanged.DisplayNamePlaintext); err != nil {
			return fmt.Errorf("populate display-name plaintext: %w", err)
		}
	case *evtv1.Event_UserBioChanged:
		if err := set(payload.UserBioChanged.GetUserId(), evtstream.EventUserBioChanged, "bio", payload.UserBioChanged.GetEncryptedBio(), &payload.UserBioChanged.BioPlaintext); err != nil {
			return fmt.Errorf("populate bio plaintext: %w", err)
		}
	case *evtv1.Event_UserVerifiedEmailAdded:
		if err := set(payload.UserVerifiedEmailAdded.GetUserId(), evtstream.EventUserVerifiedEmailAdded, "email", payload.UserVerifiedEmailAdded.GetEncryptedEmail(), &payload.UserVerifiedEmailAdded.EmailPlaintext); err != nil {
			return fmt.Errorf("populate verified-email plaintext: %w", err)
		}
	}
	return nil
}
