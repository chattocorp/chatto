package core

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// BotUpdateInput describes patchable bot profile fields.
type BotUpdateInput struct {
	Login       *string
	DisplayName *string
	Description *string
}

// ListBots returns all active bot accounts from the user projection.
func (c *ChattoCore) ListBots(ctx context.Context) ([]*corev1.User, error) {
	users, err := c.Users.UsersContext(ctx)
	if err != nil {
		return nil, err
	}
	bots := make([]*corev1.User, 0)
	for _, user := range users {
		if isBotAccount(user) {
			bots = append(bots, user)
		}
	}
	return bots, nil
}

// UpdateBot updates a bot profile while evaluating the owner/admin management
// gate inside the same authorization-fenced OCC retry as the durable facts.
func (c *ChattoCore) UpdateBot(ctx context.Context, actorID, botID string, input BotUpdateInput) (*corev1.User, error) {
	if input.Login == nil && input.DisplayName == nil && input.Description == nil {
		return nil, fmt.Errorf("%w: at least one bot field must be provided", ErrInvalidArgument)
	}
	bot, err := c.GetUser(ctx, botID)
	if err != nil {
		return nil, err
	}
	if !isBotAccount(bot) {
		return nil, ErrNotFound
	}

	nextLogin := bot.GetLogin()
	if input.Login != nil {
		nextLogin = strings.TrimSpace(*input.Login)
		if err := ValidateLogin(nextLogin); err != nil {
			return nil, err
		}
		if err := validateLoginForAccount(nextLogin, true); err != nil {
			return nil, err
		}
	}
	nextDisplayName := bot.GetDisplayName()
	if input.DisplayName != nil {
		nextDisplayName = NormalizeDisplayName(*input.DisplayName)
		if nextDisplayName == "" {
			return nil, fmt.Errorf("%w: display name cannot be empty", ErrInvalidArgument)
		}
		if utf8.RuneCountInString(nextDisplayName) > MaxDisplayNameLength {
			return nil, ErrDisplayNameTooLong
		}
		if err := ValidateDisplayName(nextDisplayName); err != nil {
			return nil, err
		}
	}
	nextDescription := bot.GetBot().GetDescription()
	if input.Description != nil {
		nextDescription = strings.TrimSpace(*input.Description)
		if nextDescription == "" {
			return nil, ErrBotDescriptionRequired
		}
		if len(nextDescription) > MaxBotDescriptionLength {
			return nil, ErrBotDescriptionTooLong
		}
	}

	loginChanged := nextLogin != bot.GetLogin()
	displayNameChanged := nextDisplayName != bot.GetDisplayName()
	descriptionChanged := nextDescription != bot.GetBot().GetDescription()
	if !loginChanged && !displayNameChanged && !descriptionChanged {
		allowed, err := c.CanManageBot(ctx, actorID, botID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, ErrPermissionDenied
		}
		return bot, nil
	}

	agg := events.UserAggregate(botID)
	entries := make([]events.BatchEntry, 0, 3)
	if loginChanged {
		event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserLoginChanged{
			UserLoginChanged: &corev1.UserLoginChangedEvent{UserId: botID},
		}})
		event.GetUserLoginChanged().EncryptedLogin, err = c.encryptUserPIIString(ctx, event.GetId(), botID, events.EventUserLoginChanged, "login", nextLogin)
		if err != nil {
			return nil, fmt.Errorf("encrypt bot login: %w", err)
		}
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}
	if displayNameChanged {
		event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserDisplayNameChanged{
			UserDisplayNameChanged: &corev1.UserDisplayNameChangedEvent{UserId: botID},
		}})
		event.GetUserDisplayNameChanged().EncryptedDisplayName, err = c.encryptUserPIIString(ctx, event.GetId(), botID, events.EventUserDisplayNameChanged, "display_name", nextDisplayName)
		if err != nil {
			return nil, fmt.Errorf("encrypt bot display name: %w", err)
		}
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}
	if descriptionChanged {
		event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_BotDescriptionChanged{
			BotDescriptionChanged: &corev1.BotDescriptionChangedEvent{UserId: botID},
		}})
		event.GetBotDescriptionChanged().EncryptedDescription, err = c.encryptUserPIIString(ctx, event.GetId(), botID, events.EventBotDescriptionChanged, "bot_description", nextDescription)
		if err != nil {
			return nil, fmt.Errorf("encrypt bot description: %w", err)
		}
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}

	check := func() error {
		allowed, err := c.CanManageBot(ctx, actorID, botID)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrPermissionDenied
		}
		if loginChanged && !strings.EqualFold(bot.GetLogin(), nextLogin) {
			if c.configModel.IsUsernameBlocked(nextLogin) || c.loginConflictsWithMentionHandle(nextLogin) {
				return ErrUsernameBlocked
			}
			return c.requireLoginMentionHandleAvailable(nextLogin)
		}
		return nil
	}
	if loginChanged && !strings.EqualFold(bot.GetLogin(), nextLogin) {
		_, err = c.appendUserBatchWithMentionableCheckAuthorized(ctx, botID, entries, true, check)
	} else {
		_, err = c.appendUserBatchAuthorized(ctx, botID, entries, events.UserSubjectFilter(), true, check)
	}
	if err != nil {
		return nil, err
	}
	c.publishUserProfileUpdate(ctx, botID)
	return c.GetUser(ctx, botID)
}

// DeleteBot permanently deletes a bot after fencing its owner/admin management
// authorization into the durable deletion-started fact.
func (c *ChattoCore) DeleteBot(ctx context.Context, actorID, botID string) error {
	bot, err := c.GetUser(ctx, botID)
	if err != nil {
		return err
	}
	if !isBotAccount(bot) {
		return ErrNotFound
	}
	if !c.Users.DeletionStarted(botID) {
		event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserAccountDeletionStarted{
			UserAccountDeletionStarted: &corev1.UserAccountDeletionStartedEvent{UserId: botID},
		}})
		if _, err := c.appendUserEvent(ctx, botID, event, "", func() error {
			allowed, err := c.CanManageBot(ctx, actorID, botID)
			if err != nil {
				return err
			}
			if !allowed {
				return ErrPermissionDenied
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return c.DeleteUser(ctx, actorID, botID)
}
