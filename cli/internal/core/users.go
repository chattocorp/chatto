package core

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// User Operations
// ============================================================================

const DeletedUserDisplayName = "Deleted User"

func DeletedUserReference(userID string) *corev1.User {
	return &corev1.User{
		Id:          userID,
		DisplayName: DeletedUserDisplayName,
		Deleted:     true,
	}
}

// CreateUser creates a new user.
// Uses the mentionables projection plus stream-wide OCC to prevent user/role
// handle collisions across replicas.
// Password is optional - pass empty string for OAuth-only users.
func (c *ChattoCore) CreateUser(ctx context.Context, actorID string, login, displayName, password string) (*corev1.User, error) {
	return c.createUserWithOptions(ctx, actorID, login, displayName, password, userCreationOptions{})
}

type userCreationOptions struct {
	verifiedEmail string
	external      *PendingExternalIdentityFlow
	invitationID  string
	isBot         bool
	botOwnerID    string
	botAPIKeyOut  *string
	authorize     func() error
}

func (c *ChattoCore) createUserWithOptions(ctx context.Context, actorID string, login, displayName, password string, options userCreationOptions) (*corev1.User, error) {
	// Trim and validate login (preserve original casing)
	login = strings.TrimSpace(login)
	isBot := options.isBot
	if isBot {
		if err := ValidateBotLogin(login); err != nil {
			return nil, err
		}
		if strings.TrimSpace(options.botOwnerID) == "" || password != "" || options.verifiedEmail != "" || options.external != nil || options.invitationID != "" {
			return nil, ErrInvalidArgument
		}
	} else {
		if err := ValidateHumanLogin(login); err != nil {
			return nil, err
		}
	}

	// Normalize and validate display name
	displayName = NormalizeDisplayName(displayName)
	if utf8.RuneCountInString(displayName) > MaxDisplayNameLength {
		return nil, ErrDisplayNameTooLong
	}
	if err := ValidateDisplayName(displayName); err != nil {
		return nil, err
	}

	// Validate password strength if password is provided
	if password != "" {
		if err := ValidatePassword(password); err != nil {
			return nil, err
		}
	}

	// Check if login is blocked (defense in depth - HTTP layer should check first)
	if c.configModel.IsUsernameBlocked(login) {
		return nil, ErrUsernameBlocked
	}
	if c.loginConflictsWithMentionHandle(login) {
		return nil, ErrUsernameBlocked
	}

	// Enforce server-wide user limit at signup as a UX gate so people don't sign up
	// only to be blocked when adding their first verified sign-in factor. The
	// factor-add checks remain the race-safe hard gate.
	if max := c.config.Limits.MaxUsersOrDefault(); max >= 0 {
		count, err := c.CountUserLimitAccounts(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to count verified accounts: %w", err)
		}
		if count >= max {
			return nil, ErrLimitExceeded
		}
	}

	// Generate user ID upfront
	userID := NewUserID()
	eventActorID := strings.TrimSpace(actorID)
	if eventActorID == "" {
		eventActorID = userID
	}

	now := timestamppb.Now()
	user := &corev1.User{
		Id:             userID,
		Login:          login,
		DisplayName:    displayName,
		CreatedAt:      now,
		IsBot:          isBot,
		BotOwnerUserId: options.botOwnerID,
	}
	var botAPIKey string
	var botAPIKeyVerifier []byte
	if isBot {
		var err error
		botAPIKey, err = NewBotAPIKey(userID)
		if err != nil {
			return nil, err
		}
		botAPIKeyVerifier = c.botAPIKeyVerifier(botAPIKey)
	}

	// Create encryption key for this user. Keys are always created so they
	// exist if encryption is enabled later.
	keyRef, err := c.encryption.keyWrapper.CreateKey(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create encryption key: %w", err)
	}
	cleanupEncryptionKey := true
	var cleanupContentKeyRefs []string
	defer func() {
		if cleanupEncryptionKey {
			for _, contentKeyRef := range cleanupContentKeyRefs {
				if err := c.encryption.contentKeys.Shred(context.WithoutCancel(ctx), contentKeyRef); err != nil {
					c.logger.Warn("failed to clean up user content key after failed signup", "error", err, "content_key_ref", contentKeyRef)
				}
			}
			c.cleanupCreatedUserEncryptionKey(ctx, keyRef)
		}
	}()

	_, wrappedMessageDEK, err := c.newWrappedUserDEK(ctx, userID, keyRef, 1, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
	if err != nil {
		return nil, err
	}
	cleanupContentKeyRefs = append(cleanupContentKeyRefs, wrappedMessageDEK.GetContentKeyRef())

	piiDEKBytes, wrappedPIIDEK, err := c.newWrappedUserDEK(ctx, userID, keyRef, 1, corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII)
	if err != nil {
		return nil, err
	}
	cleanupContentKeyRefs = append(cleanupContentKeyRefs, wrappedPIIDEK.GetContentKeyRef())

	piiDEK := &userDEK{epoch: 1, purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, key: piiDEKBytes}
	agg := evtstream.UserAggregate(userID)
	messageDEKEvent := newEvent(eventActorID, &corev1.Event{Event: &corev1.Event_UserDekGenerated{
		UserDekGenerated: wrappedMessageDEK,
	}})
	messageDEKEvent.CreatedAt = now
	piiDEKEvent := newEvent(eventActorID, &corev1.Event{Event: &corev1.Event_UserDekGenerated{
		UserDekGenerated: wrappedPIIDEK,
	}})
	piiDEKEvent.CreatedAt = now
	accountCreated := newEvent(eventActorID, &corev1.Event{Event: &corev1.Event_UserAccountCreated{
		UserAccountCreated: &corev1.UserAccountCreatedEvent{
			UserId:         userID,
			IsBot:          isBot,
			BotOwnerUserId: options.botOwnerID,
		},
	}})
	accountCreated.CreatedAt = now
	account := accountCreated.GetUserAccountCreated()
	account.EncryptedLogin, err = encryptUserPIIStringWithDEK(piiDEK, accountCreated.GetId(), userID, evtstream.EventUserAccountCreated, "login", login)
	if err != nil {
		return nil, fmt.Errorf("encrypt login: %w", err)
	}
	account.EncryptedDisplayName, err = encryptUserPIIStringWithDEK(piiDEK, accountCreated.GetId(), userID, evtstream.EventUserAccountCreated, "display_name", displayName)
	if err != nil {
		return nil, fmt.Errorf("encrypt display name: %w", err)
	}

	entries := []evtstream.BatchEntry{{
		Subject: agg.Subject(evtstream.EventUserDEKGenerated),
		Event:   messageDEKEvent,
	}, {
		Subject: agg.Subject(evtstream.EventUserDEKGenerated),
		Event:   piiDEKEvent,
	}, {
		Subject: agg.Subject(evtstream.EventUserAccountCreated),
		Event:   accountCreated,
	}}
	if isBot {
		keyCreated := newEvent(eventActorID, &corev1.Event{Event: &corev1.Event_BotApiKeyCreated{
			BotApiKeyCreated: &corev1.BotApiKeyCreatedEvent{UserId: userID, Verifier: botAPIKeyVerifier},
		}})
		keyCreated.CreatedAt = now
		entries = append(entries, evtstream.BatchEntry{
			Subject: agg.Subject(evtstream.EventBotAPIKeyCreated),
			Event:   keyCreated,
		})
	}
	if password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), passwordHashCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		passwordChanged := newEvent(eventActorID, &corev1.Event{Event: &corev1.Event_UserPasswordHashChanged{
			UserPasswordHashChanged: &corev1.UserPasswordHashChangedEvent{
				UserId:       userID,
				PasswordHash: hashedPassword,
			},
		}})
		passwordChanged.CreatedAt = now
		entries = append(entries, evtstream.BatchEntry{
			Subject: agg.Subject(evtstream.EventUserPasswordHashChanged),
			Event:   passwordChanged,
		})
	}

	if options.invitationID != "" {
		invitationEvent := newEvent(eventActorID, &corev1.Event{Event: &corev1.Event_InvitationRedeemed{
			InvitationRedeemed: &corev1.InvitationRedeemedEvent{InvitationId: options.invitationID, UserId: userID},
		}})
		invitationEvent.CreatedAt = now
		entries = append(entries, evtstream.BatchEntry{
			Subject: evtstream.InvitationAggregate(options.invitationID).SubjectFor(invitationEvent),
			Event:   invitationEvent,
		})
	}

	if options.verifiedEmail != "" {
		email := strings.ToLower(strings.TrimSpace(options.verifiedEmail))
		verifiedEmailEvent := newEvent(eventActorID, &corev1.Event{Event: &corev1.Event_UserVerifiedEmailAdded{
			UserVerifiedEmailAdded: &corev1.UserVerifiedEmailAddedEvent{UserId: userID},
		}})
		verifiedEmailEvent.CreatedAt = now
		verifiedEmailEvent.GetUserVerifiedEmailAdded().EncryptedEmail, err = encryptUserPIIStringWithDEK(
			piiDEK,
			verifiedEmailEvent.GetId(),
			userID,
			evtstream.EventUserVerifiedEmailAdded,
			"email",
			email,
		)
		if err != nil {
			return nil, fmt.Errorf("encrypt verified email: %w", err)
		}
		entries = append(entries, evtstream.BatchEntry{Subject: agg.SubjectFor(verifiedEmailEvent), Event: verifiedEmailEvent})
	}

	if options.external != nil {
		flow := options.external
		externalEvent := newEvent(eventActorID, &corev1.Event{Event: &corev1.Event_UserExternalIdentityLinked{
			UserExternalIdentityLinked: &corev1.UserExternalIdentityLinkedEvent{
				UserId:       userID,
				Issuer:       flow.Issuer,
				Subject:      flow.Subject,
				SubjectHash:  externalIdentityHash(flow.Issuer, flow.Subject),
				ProviderId:   flow.ProviderID,
				ProviderType: flow.ProviderType,
			},
		}})
		externalEvent.CreatedAt = now
		entries = append(entries, evtstream.BatchEntry{Subject: agg.SubjectFor(externalEvent), Event: externalEvent})
	}

	_, err = c.appendUserBatchWithMentionableCheck(ctx, userID, entries, func() error {
		if options.authorize != nil {
			if err := options.authorize(); err != nil {
				return err
			}
		}
		if err := c.requireLoginMentionHandleAvailable(login); err != nil {
			return err
		}
		if isBot && c.config.Limits.MaxUsersOrDefault() >= 0 {
			count, err := c.CountUserLimitAccounts(ctx)
			if err != nil {
				return err
			}
			if count >= c.config.Limits.MaxUsersOrDefault() {
				return ErrLimitExceeded
			}
		}
		if options.verifiedEmail != "" {
			if _, claimed := c.userModel.emailOwnerID(options.verifiedEmail); claimed {
				return ErrEmailAlreadyVerified
			}
		}
		if options.external != nil {
			if _, claimed := c.userModel.externalIdentityOwnerID(options.external.Issuer, options.external.Subject); claimed {
				return ErrExternalIdentityAlreadyClaimed
			}
		}
		if (options.verifiedEmail != "" || options.external != nil) && c.config.Limits.MaxUsersOrDefault() >= 0 {
			if err := c.requireVerifiedAccountCapacity(ctx, ""); err != nil {
				return err
			}
		}
		if options.invitationID != "" {
			if _, err := c.invitationModel.validateIDAt(options.invitationID, time.Now()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	cleanupEncryptionKey = false
	if options.botAPIKeyOut != nil {
		*options.botAPIKeyOut = botAPIKey
	}
	if err := c.userModel.waitForContentKeysCurrent(ctx, userID); err != nil {
		return nil, err
	}
	if options.invitationID != "" {
		if err := c.invitationModel.projection.Projector().WaitForCurrent(ctx); err != nil {
			return nil, err
		}
	}
	if options.verifiedEmail != "" && c.config.Owners.IsServerOwnerEmail(options.verifiedEmail) {
		if err := c.AssignServerRoleToExistingUser(ctx, SystemActorID, userID, RoleOwner); err != nil {
			c.logger.Warn("Failed to auto-assign owner role on signup", "user_id", userID, "error", err)
		}
	}

	// Publish a best-effort transient signal for the new public user.
	event := newLiveEvent(eventActorID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_UserCreated{
			UserCreated: &corev1.UserCreatedSyncEvent{
				UserId:      userID,
				Login:       login,
				DisplayName: displayName,
			},
		},
	})
	subject := subjects.LiveSyncUserEvent(userID, "created")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish user created event", "error", err, "user_id", userID)
	}

	c.logger.Info("Created user", "id", userID)

	return user, nil
}

func (c *ChattoCore) cleanupCreatedUserEncryptionKey(ctx context.Context, keyRef string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := c.deleteEncryptionKeyOnly(cleanupCtx, keyRef); err != nil {
		c.logger.Warn("failed to clean up user encryption key after failed signup", "error", err, "key_ref", keyRef)
	}
}

// CreateVerifiedUser atomically creates a user with an already-verified email.
//
// Used by signup-completion (post email-link click) and trusted account-link
// flows where the email has already been proven.
func (c *ChattoCore) CreateVerifiedUser(ctx context.Context, actorID, login, displayName, password, email string) (*corev1.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, ErrInvalidArgument
	}
	return c.createUserWithOptions(ctx, actorID, login, displayName, password, userCreationOptions{verifiedEmail: email})
}

// CreateVerifiedUserWithInvitation atomically creates a verified account and
// records its invitation redemption.
func (c *ChattoCore) CreateVerifiedUserWithInvitation(ctx context.Context, actorID, login, displayName, password, email, invitationID string) (*corev1.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, ErrInvalidArgument
	}
	invitationID = strings.TrimSpace(invitationID)
	if invitationID == "" {
		return nil, ErrInvitationInvalid
	}
	return c.createUserWithOptions(ctx, actorID, login, displayName, password, userCreationOptions{
		verifiedEmail: email,
		invitationID:  invitationID,
	})
}

// rollbackUserCreation undoes the persisted writes performed by CreateUser. Best-effort —
// failures are logged but not returned, since the caller is already in an error path.
func (c *ChattoCore) rollbackUserCreation(ctx context.Context, user *corev1.User) {
	c.logger.Warn("rolling back user creation", "user_id", user.Id)
	_ = c.DeleteUser(ctx, "system:rollback", user.Id)
}

// GetUser retrieves a user from the user projection.
func (c *ChattoCore) GetUser(ctx context.Context, userID string) (*corev1.User, error) {
	user, ok, err := c.userModel.user(ctx, userID)
	if err != nil {
		return nil, err
	}
	if ok {
		return user, nil
	}
	return nil, ErrNotFound
}

// GetUserReference retrieves a public user reference. Deleted or crypto-shredded
// users are returned as tombstones; unknown users still return ErrNotFound.
func (c *ChattoCore) GetUserReference(ctx context.Context, userID string) (*corev1.User, error) {
	user, ok, err := c.userModel.userReference(ctx, userID)
	if err != nil {
		return nil, err
	}
	if ok {
		return user, nil
	}
	return nil, ErrNotFound
}

// GetUsers retrieves multiple users by ID from the user projection.
// Returns users in the same order as userIDs. nil entries indicate not-found users.
// More efficient than calling GetUser() in a loop for batched operations.
func (c *ChattoCore) GetUsers(ctx context.Context, userIDs []string) ([]*corev1.User, error) {
	if len(userIDs) == 0 {
		return []*corev1.User{}, nil
	}

	// Deduplicate IDs to avoid redundant fetches
	seen := make(map[string]bool, len(userIDs))
	uniqueIDs := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if !seen[id] {
			seen[id] = true
			uniqueIDs = append(uniqueIDs, id)
		}
	}

	userMap := make(map[string]*corev1.User, len(uniqueIDs))
	for _, id := range uniqueIDs {
		user, ok, err := c.userModel.user(ctx, id)
		if err != nil {
			return nil, err
		}
		if ok {
			userMap[id] = user
		}
	}

	// Return in original order (nil for not-found users)
	result := make([]*corev1.User, len(userIDs))
	for i, id := range userIDs {
		result[i] = userMap[id] // nil if not found
	}

	return result, nil
}

// GetUserByLogin retrieves a user by their login name using the login index.
func (c *ChattoCore) GetUserByLogin(ctx context.Context, login string) (*corev1.User, error) {
	user, ok, err := c.userModel.userByLogin(ctx, login)
	if err != nil {
		return nil, err
	}
	if ok {
		return user, nil
	}
	return nil, ErrNotFound
}

// ListUsers retrieves all users from the user projection.
// CountUsers returns the total number of users on the server.
func (c *ChattoCore) CountUsers(ctx context.Context) (int, error) {
	return c.userModel.userCount(), nil
}

func (c *ChattoCore) ListUsers(ctx context.Context) ([]*corev1.User, error) {
	return c.userModel.allUsers(ctx)
}

// ============================================================================
// Login Validation
// ============================================================================

// ErrLoginAlreadyTaken is returned when the login name is already taken.
var ErrLoginAlreadyTaken = fmt.Errorf("login name is already taken")

// ErrUsernameBlocked is returned when the login name is in the blocked list.
var ErrUsernameBlocked = fmt.Errorf("this username is not available")

// CheckLoginExists checks if a login name is already taken.
func (c *ChattoCore) CheckLoginExists(ctx context.Context, login string) (bool, error) {
	return c.userModel.loginExists(login), nil
}

// IsLoginAvailable reports whether a login currently passes validation and
// conflicts with neither reserved names nor existing mention handles. The
// result is advisory; account creation remains the race-safe authority.
func (c *ChattoCore) IsLoginAvailable(login string) bool {
	login = strings.TrimSpace(login)
	return ValidateLogin(login) == nil &&
		!c.configModel.IsUsernameBlocked(login) &&
		!c.loginConflictsWithMentionHandle(login) &&
		!c.userModel.loginExists(login)
}
