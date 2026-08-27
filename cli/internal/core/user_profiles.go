package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	MaxCustomStatusEmojiLength = 16
	MaxCustomStatusTextLength  = 100
)

// publishUserProfileUpdate publishes a UserProfileUpdatedEvent to the server stream.
// This allows other users to see profile changes (avatar, display name) in real-time.
func (c *ChattoCore) publishUserProfileUpdate(ctx context.Context, userID string) {
	// Get current user data
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		c.logger.Warn("failed to get user for profile update event", "error", err, "user_id", userID)
		return
	}

	// Get current avatar URL (full resolution for events)
	avatarURL, err := c.GetUserAvatarURL(ctx, userID, nil, nil, "")
	if err != nil {
		c.logger.Warn("failed to get avatar URL for profile update event", "error", err, "user_id", userID)
		avatarURL = ""
	}

	event := newLiveEvent(userID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_UserProfileUpdated{
			UserProfileUpdated: &corev1.UserProfileUpdatedEvent{
				UserId:      userID,
				DisplayName: user.DisplayName,
				AvatarUrl:   avatarURL,
				Login:       user.Login,
			},
		},
	})

	// Publish to live.sync.user.{userId}.profile_updated for real-time delivery.
	// Profile updates are transient (no need for JetStream storage/replay)
	subject := subjects.LiveSyncUserEvent(userID, "profile_updated")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("failed to publish user profile update event", "error", err, "user_id", userID)
	}
}

var ErrCustomStatusEmojiRequired = fmt.Errorf("custom status emoji is required")
var ErrCustomStatusEmojiInvalid = fmt.Errorf("custom status emoji must be a single supported emoji")
var ErrCustomStatusTextRequired = fmt.Errorf("custom status text is required")
var ErrCustomStatusEmojiTooLong = fmt.Errorf("custom status emoji is too long")
var ErrCustomStatusTextTooLong = fmt.Errorf("custom status text is too long")
var ErrCustomStatusExpiryInPast = fmt.Errorf("custom status expiry must be in the future")

// UpdateUserDisplayName updates a user's display name.
// Authorization: Caller should verify the actor is the user being updated.
func (c *ChattoCore) UpdateUserDisplayName(ctx context.Context, userID, displayName string) (*corev1.User, error) {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return nil, err
	}
	return c.updateUserDisplayNameAs(ctx, userID, userID, displayName)
}

func (c *ChattoCore) updateUserDisplayNameAs(ctx context.Context, actorID, userID, displayName string) (*corev1.User, error) {
	// Normalize and validate display name
	displayName = NormalizeDisplayName(displayName)
	if displayName == "" {
		return nil, fmt.Errorf("display name cannot be empty")
	}
	if utf8.RuneCountInString(displayName) > MaxDisplayNameLength {
		return nil, ErrDisplayNameTooLong
	}
	if err := ValidateDisplayName(displayName); err != nil {
		return nil, err
	}

	// Get current user
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserDisplayNameChanged{
		UserDisplayNameChanged: &corev1.UserDisplayNameChangedEvent{
			UserId: userID,
		},
	}})
	encryptedDisplayName, err := c.encryptUserPIIString(ctx, event.GetId(), userID, evtstream.EventUserDisplayNameChanged, "display_name", displayName)
	if err != nil {
		return nil, fmt.Errorf("encrypt display name: %w", err)
	}
	event.GetUserDisplayNameChanged().EncryptedDisplayName = encryptedDisplayName
	if _, err := c.appendUserEvent(ctx, userID, event, "", func() error {
		if _, err := c.GetUser(ctx, userID); err != nil {
			return fmt.Errorf("user not found: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to store user: %w", err)
	}
	user.DisplayName = displayName

	c.logger.Info("Updated user display name", "id", userID)

	// Publish profile update event
	c.publishUserProfileUpdate(ctx, userID)

	return user, nil
}

// AdminUpdateUserDisplayName updates a user's display name as an admin action.
// Behavior matches UpdateUserDisplayName; this exists as a distinct entry point
// for audit clarity in logs.
// Authorization: Caller must verify admin privileges.
func (c *ChattoCore) AdminUpdateUserDisplayName(ctx context.Context, userID, displayName string) (*corev1.User, error) {
	user, err := c.updateUserDisplayNameAs(ctx, SystemActorID, userID, displayName)
	if err != nil {
		return nil, err
	}
	c.logger.Info("Admin updated user display name", "id", userID)
	return user, nil
}

// AdminUpdateUserProfile updates a user's login and/or display name as a
// single admin-authored mutation. When both fields are changed, both durable
// events are appended atomically in one batch.
func (c *ChattoCore) AdminUpdateUserProfile(ctx context.Context, userID string, login, displayName *string) (*corev1.User, error) {
	return c.updateUserProfileAs(ctx, SystemActorID, userID, login, displayName, true)
}

func (c *ChattoCore) updateUserProfileAs(ctx context.Context, actorID, userID string, login, displayName *string, retryConflicts bool) (*corev1.User, error) {
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	var nextLogin string
	var loginChanged bool
	var loginNeedsMentionCheck bool
	if login != nil {
		nextLogin = strings.TrimSpace(*login)
		var validationErr error
		if user.GetIsBot() {
			validationErr = ValidateBotLogin(nextLogin)
		} else {
			validationErr = ValidateHumanLogin(nextLogin)
		}
		if validationErr != nil {
			return nil, validationErr
		}
		loginChanged = user.GetLogin() != nextLogin
		loginNeedsMentionCheck = loginChanged && !strings.EqualFold(user.GetLogin(), nextLogin)
		if loginNeedsMentionCheck {
			if c.configModel.IsUsernameBlocked(nextLogin) {
				return nil, ErrUsernameBlocked
			}
			if c.loginConflictsWithMentionHandle(nextLogin) {
				return nil, ErrUsernameBlocked
			}
		}
	}

	var nextDisplayName string
	var displayNameChanged bool
	if displayName != nil {
		nextDisplayName = NormalizeDisplayName(*displayName)
		if nextDisplayName == "" {
			return nil, ErrInvalidArgument
		}
		if utf8.RuneCountInString(nextDisplayName) > MaxDisplayNameLength {
			return nil, ErrDisplayNameTooLong
		}
		if err := ValidateDisplayName(nextDisplayName); err != nil {
			return nil, err
		}
		displayNameChanged = user.GetDisplayName() != nextDisplayName
	}

	agg := evtstream.UserAggregate(userID)
	entries := make([]evtstream.BatchEntry, 0, 2)
	if loginChanged {
		loginChangedEvent := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserLoginChanged{
			UserLoginChanged: &corev1.UserLoginChangedEvent{UserId: userID},
		}})
		encryptedLogin, err := c.encryptUserPIIString(ctx, loginChangedEvent.GetId(), userID, evtstream.EventUserLoginChanged, "login", nextLogin)
		if err != nil {
			return nil, fmt.Errorf("encrypt login: %w", err)
		}
		loginChangedEvent.GetUserLoginChanged().EncryptedLogin = encryptedLogin
		entries = append(entries, evtstream.BatchEntry{Subject: agg.SubjectFor(loginChangedEvent), Event: loginChangedEvent})
	}
	if displayNameChanged {
		displayNameChangedEvent := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserDisplayNameChanged{
			UserDisplayNameChanged: &corev1.UserDisplayNameChangedEvent{UserId: userID},
		}})
		encryptedDisplayName, err := c.encryptUserPIIString(ctx, displayNameChangedEvent.GetId(), userID, evtstream.EventUserDisplayNameChanged, "display_name", nextDisplayName)
		if err != nil {
			return nil, fmt.Errorf("encrypt display name: %w", err)
		}
		displayNameChangedEvent.GetUserDisplayNameChanged().EncryptedDisplayName = encryptedDisplayName
		entries = append(entries, evtstream.BatchEntry{Subject: agg.SubjectFor(displayNameChangedEvent), Event: displayNameChangedEvent})
	}

	checkUserExists := func() error {
		if _, err := c.GetUser(ctx, userID); err != nil {
			return fmt.Errorf("user not found: %w", err)
		}
		return nil
	}
	if len(entries) > 0 {
		if loginNeedsMentionCheck {
			appendBatch := c.appendUserBatchWithMentionableCheck
			if !retryConflicts {
				appendBatch = c.appendUserBatchWithMentionableCheckOnce
			}
			_, err = appendBatch(ctx, userID, entries, func() error {
				if err := checkUserExists(); err != nil {
					return err
				}
				return c.requireLoginMentionHandleAvailable(nextLogin)
			})
		} else {
			appendBatch := c.appendUserBatch
			if !retryConflicts {
				appendBatch = c.appendUserBatchOnce
			}
			_, err = appendBatch(ctx, userID, entries, "", checkUserExists)
		}
		if err != nil {
			if errors.Is(err, ErrLoginAlreadyTaken) {
				return nil, ErrLoginAlreadyTaken
			}
			return nil, fmt.Errorf("failed to store user: %w", err)
		}
	}

	if loginChanged {
		user.Login = nextLogin
	}
	if displayNameChanged {
		user.DisplayName = nextDisplayName
	}
	c.logger.Info("Admin updated user profile", "id", userID)
	c.publishUserProfileUpdate(ctx, userID)
	return user, nil
}

type AdminUpdateUserInput struct {
	Login       *string
	DisplayName *string
}

func (c *ChattoCore) AdminUpdateUser(ctx context.Context, actorID, targetUserID string, input AdminUpdateUserInput) (*corev1.User, error) {
	if err := c.requireCanAdminManageOtherUser(ctx, actorID, targetUserID); err != nil {
		return nil, err
	}
	if input.Login == nil && input.DisplayName == nil {
		return nil, fmt.Errorf("%w: at least one of login or display_name must be provided", ErrInvalidArgument)
	}
	if err := c.requireHumanUser(ctx, targetUserID); err != nil {
		return nil, err
	}
	return c.updateUserProfileAs(ctx, actorID, targetUserID, input.Login, input.DisplayName, true)
}

func (c *ChattoCore) AdminClearLoginChangeCooldown(ctx context.Context, actorID, targetUserID string) error {
	if err := c.requireCanAdminManageOtherUser(ctx, actorID, targetUserID); err != nil {
		return err
	}
	if err := c.requireHumanUser(ctx, targetUserID); err != nil {
		return err
	}
	return c.ClearLoginChangeCooldownAs(ctx, actorID, targetUserID)
}

func (c *ChattoCore) requireCanAdminManageOtherUser(ctx context.Context, actorID, targetUserID string) error {
	if actorID == "" {
		return ErrNotAuthenticated
	}
	if targetUserID == "" {
		return fmt.Errorf("%w: target user ID is required", ErrInvalidArgument)
	}
	if actorID == targetUserID {
		return ErrPermissionDenied
	}
	canManage, err := c.CanManageUserAccounts(ctx, actorID)
	if err != nil {
		return fmt.Errorf("check user.manage-accounts: %w", err)
	}
	if !canManage {
		return ErrPermissionDenied
	}
	return nil
}

// ============================================================================
// Login Change Operations
// ============================================================================

// userLoginChangedAtKey returns the KV key for tracking when a user last changed their login.
func userLoginChangedAtKey(userID string) string {
	return "user_login_changed_at." + userID
}

// UpdateUserLogin changes a user's login/username with 30-day cooldown enforcement.
// Authorization: Caller should verify the actor is the user being updated.
func (c *ChattoCore) UpdateUserLogin(ctx context.Context, userID, newLogin string) (*corev1.User, error) {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return nil, err
	}
	return c.applyLoginChange(ctx, userID, userID, newLogin, true)
}

// AdminUpdateUserLogin changes a user's login/username, bypassing the cooldown
// check and not advancing the cooldown timestamp. The user retains whatever
// rename allowance they had prior to the admin edit.
// Authorization: Caller must verify admin privileges.
func (c *ChattoCore) AdminUpdateUserLogin(ctx context.Context, userID, newLogin string) (*corev1.User, error) {
	user, err := c.applyLoginChange(ctx, SystemActorID, userID, newLogin, false)
	if err != nil {
		return nil, err
	}
	c.logger.Info("Admin updated user login", "id", userID)
	return user, nil
}

// applyLoginChange performs the actual login change. When enforceCooldown is
// true, the 30-day cooldown is checked before changing and a new timestamp is
// recorded after a successful change.
func (c *ChattoCore) applyLoginChange(ctx context.Context, actorID, userID, newLogin string, enforceCooldown bool) (*corev1.User, error) {
	// Trim (preserve original casing).
	newLogin = strings.TrimSpace(newLogin)

	// Get current user
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	if user.GetIsBot() {
		if err := ValidateBotLogin(newLogin); err != nil {
			return nil, err
		}
	} else if err := ValidateHumanLogin(newLogin); err != nil {
		return nil, err
	}

	// Check if unchanged (exact match — case-only changes are allowed)
	if user.Login == newLogin {
		return user, nil // No-op, return current user
	}

	caseOnly := strings.EqualFold(user.Login, newLogin)
	if !caseOnly {
		if c.configModel.IsUsernameBlocked(newLogin) {
			return nil, ErrUsernameBlocked
		}
		if c.loginConflictsWithMentionHandle(newLogin) {
			return nil, ErrUsernameBlocked
		}
	}

	// Check cooldown (skipped on admin path)
	if enforceCooldown && !caseOnly {
		lastChange, err := c.GetLastLoginChange(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to check login change cooldown: %w", err)
		}
		if !lastChange.IsZero() && time.Since(lastChange) < LoginChangeCooldown {
			return nil, ErrLoginChangeCooldown
		}
	}

	loginChanged := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserLoginChanged{
		UserLoginChanged: &corev1.UserLoginChangedEvent{
			UserId: userID,
		},
	}})
	encryptedLogin, err := c.encryptUserPIIString(ctx, loginChanged.GetId(), userID, evtstream.EventUserLoginChanged, "login", newLogin)
	if err != nil {
		return nil, fmt.Errorf("encrypt login: %w", err)
	}
	loginChanged.GetUserLoginChanged().EncryptedLogin = encryptedLogin
	agg := evtstream.UserAggregate(userID)
	entries := []evtstream.BatchEntry{{
		Subject: agg.SubjectFor(loginChanged),
		Event:   loginChanged,
	}}
	if enforceCooldown && !caseOnly {
		cooldownStarted := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserLoginCooldownStarted{
			UserLoginCooldownStarted: &corev1.UserLoginCooldownStartedEvent{UserId: userID},
		}})
		cooldownStarted.CreatedAt = loginChanged.GetCreatedAt()
		entries = append(entries, evtstream.BatchEntry{
			Subject: agg.SubjectFor(cooldownStarted),
			Event:   cooldownStarted,
		})
	}
	if !caseOnly {
		_, err = c.appendUserBatchWithMentionableCheck(ctx, userID, entries, func() error {
			if _, err := c.GetUser(ctx, userID); err != nil {
				return fmt.Errorf("user not found: %w", err)
			}
			return c.requireLoginMentionHandleAvailable(newLogin)
		})
	} else {
		_, err = c.appendUserBatch(ctx, userID, entries, evtstream.UserSubjectFilter(), func() error {
			if _, err := c.GetUser(ctx, userID); err != nil {
				return fmt.Errorf("user not found: %w", err)
			}
			return nil
		})
	}
	if err != nil {
		if errors.Is(err, ErrLoginAlreadyTaken) {
			return nil, ErrLoginAlreadyTaken
		}
		return nil, fmt.Errorf("failed to store user: %w", err)
	}
	user.Login = newLogin

	c.logger.Info("Updated user login", "id", userID)

	// Publish profile update event
	c.publishUserProfileUpdate(ctx, userID)

	return user, nil
}

// GetLastLoginChange returns when the user last changed their login.
// Returns zero time if the user has never changed their login.
func (c *ChattoCore) GetLastLoginChange(ctx context.Context, userID string) (time.Time, error) {
	return c.userModel.loginChangedAt(userID), nil
}

// ClearLoginChangeCooldown removes the cooldown timestamp for a user, allowing
// them to immediately change their login again. Idempotent — clearing an
// already-clear cooldown is a no-op.
// Authorization: Caller must verify admin privileges.
func (c *ChattoCore) ClearLoginChangeCooldown(ctx context.Context, userID string) error {
	return c.ClearLoginChangeCooldownAs(ctx, userID, userID)
}

// ClearLoginChangeCooldownAs removes the cooldown timestamp with explicit actor
// attribution. Authorization must be checked by the caller.
func (c *ChattoCore) ClearLoginChangeCooldownAs(ctx context.Context, actorID, userID string) error {
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserLoginCooldownCleared{
		UserLoginCooldownCleared: &corev1.UserLoginCooldownClearedEvent{UserId: userID},
	}})
	if _, err := c.appendUserEvent(ctx, userID, event, "", func() error {
		if _, err := c.GetUser(ctx, userID); err != nil {
			return fmt.Errorf("user not found: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to clear login change cooldown: %w", err)
	}
	c.logger.Info("Cleared user login change cooldown", "id", userID)
	c.publishUserProfileUpdate(ctx, userID)
	return nil
}

// SetUserCustomStatus stores or replaces a user's durable custom status.
// Expiry is modeled on the event itself; readers hide expired statuses without
// writing auxiliary runtime state.
func (c *ChattoCore) SetUserCustomStatus(ctx context.Context, userID, emoji, text string, expiresAt *time.Time) (*corev1.User, error) {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return nil, err
	}
	emoji = strings.TrimSpace(emoji)
	text = strings.TrimSpace(text)
	if emoji == "" {
		return nil, ErrCustomStatusEmojiRequired
	}
	if text == "" {
		return nil, ErrCustomStatusTextRequired
	}
	if utf8.RuneCountInString(emoji) > MaxCustomStatusEmojiLength {
		return nil, ErrCustomStatusEmojiTooLong
	}
	if !IsValidUnicodeEmoji(emoji) {
		return nil, ErrCustomStatusEmojiInvalid
	}
	if utf8.RuneCountInString(text) > MaxCustomStatusTextLength {
		return nil, ErrCustomStatusTextTooLong
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return nil, ErrCustomStatusExpiryInPast
	}
	if _, err := c.GetUser(ctx, userID); err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	status := &corev1.CustomUserStatus{
		Emoji: emoji,
		Text:  text,
	}
	if expiresAt != nil {
		status.ExpiresAt = timestamppb.New(*expiresAt)
	}

	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserCustomStatusSet{
		UserCustomStatusSet: &corev1.UserCustomStatusSetEvent{
			UserId: userID,
			Status: status,
		},
	}})
	if _, err := c.appendUserEvent(ctx, userID, event, "", nil); err != nil {
		return nil, fmt.Errorf("failed to store custom status: %w", err)
	}

	return c.GetUser(ctx, userID)
}

// ClearUserCustomStatus removes a user's durable custom status. It is
// idempotent and still records a clear event for explicit user action history.
func (c *ChattoCore) ClearUserCustomStatus(ctx context.Context, userID string) (*corev1.User, error) {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return nil, err
	}
	if _, err := c.GetUser(ctx, userID); err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserCustomStatusCleared{
		UserCustomStatusCleared: &corev1.UserCustomStatusClearedEvent{UserId: userID},
	}})
	if _, err := c.appendUserEvent(ctx, userID, event, "", nil); err != nil {
		return nil, fmt.Errorf("failed to clear custom status: %w", err)
	}

	return c.GetUser(ctx, userID)
}
