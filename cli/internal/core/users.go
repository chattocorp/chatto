package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// ============================================================================
// User Operations
// ============================================================================

const DeletedUserDisplayName = "Deleted User"

func DeletedUserReference(userID string) *readmodel.User {
	return &readmodel.User{
		Id:          userID,
		DisplayName: DeletedUserDisplayName,
		Deleted:     true,
	}
}

// CreateUser creates a new user.
// Uses the mentionables projection plus stream-wide OCC to prevent user/role
// handle collisions across replicas.
// Password is optional - pass empty string for OAuth-only users.
func (c *ChattoCore) CreateUser(ctx context.Context, actorID string, login, displayName, password string) (*readmodel.User, error) {
	return c.createUserWithOptions(ctx, actorID, login, displayName, password, userCreationOptions{})
}

type userCreationOptions struct {
	setup         *ServerSetupInput // Only the first-run command may supply completion facts.
	verifiedEmail string
	external      *PendingExternalIdentityFlow
	invitationID  string
	selfService   bool // Admission is checked under account creation's EVT OCC.
	isBot         bool
	botOwnerID    string
	botAPIKeyOut  *string
	botAPIKeyName string
	// Historical bots have no initial API key. Normal bot creation still issues one.
	credentialFreeBot bool
	// initialDeactivated commits the initial status with account creation.
	initialDeactivated bool
	authorize          func() error
	// importReceipt commits identity mapping with account creation. Retain keys
	// after an ambiguous acknowledgement so a resumed import can hydrate it.
	importReceipt func(string) evtstream.BatchEntry
	importContext *evtv1.ImportContext
	// importEmails are operator-trusted email claims, committed with the new
	// account and its receipt. They use the normal encrypted email ownership index.
	importEmails []string
}

func (c *ChattoCore) createUserWithOptions(ctx context.Context, actorID string, login, displayName, password string, options userCreationOptions) (*readmodel.User, error) {
	if options.setup == nil && (options.verifiedEmail != "" || options.external != nil) {
		required, err := c.SetupRequired(ctx)
		if err != nil {
			return nil, err
		}
		if required {
			return nil, ErrSetupRequired
		}
	}
	// Trim and validate login (preserve original casing)
	login = strings.TrimSpace(login)
	isBot := options.isBot
	if err := ValidateLogin(login); err != nil {
		return nil, err
	}
	if isBot {
		if strings.TrimSpace(options.botOwnerID) == "" || password != "" || options.verifiedEmail != "" || len(options.importEmails) != 0 || options.external != nil || options.invitationID != "" {
			return nil, ErrInvalidArgument
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
	user := &readmodel.User{
		Id:             userID,
		Login:          login,
		DisplayName:    displayName,
		CreatedAt:      now,
		IsBot:          isBot,
		BotOwnerUserId: options.botOwnerID,
		Deactivated:    options.initialDeactivated,
	}
	var botAPIKey string
	var botAPIKeyVerifier []byte
	if isBot && !options.credentialFreeBot {
		options.botAPIKeyName = normalizedBotAPIKeyName(options.botAPIKeyName)
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

	_, wrappedMessageDEK, err := c.newWrappedUserDEK(ctx, userID, keyRef, 1, evtv1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
	if err != nil {
		return nil, err
	}
	cleanupContentKeyRefs = append(cleanupContentKeyRefs, wrappedMessageDEK.GetContentKeyRef())

	piiDEKBytes, wrappedPIIDEK, err := c.newWrappedUserDEK(ctx, userID, keyRef, 1, evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII)
	if err != nil {
		return nil, err
	}
	cleanupContentKeyRefs = append(cleanupContentKeyRefs, wrappedPIIDEK.GetContentKeyRef())

	piiDEK := &userDEK{epoch: 1, purpose: evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, key: piiDEKBytes}
	agg := evtstream.UserAggregate(userID)
	messageDEKEvent := newEvent(eventActorID, &evtv1.Event{Event: &evtv1.Event_UserDekGenerated{
		UserDekGenerated: wrappedMessageDEK,
	}})
	messageDEKEvent.CreatedAt = now
	piiDEKEvent := newEvent(eventActorID, &evtv1.Event{Event: &evtv1.Event_UserDekGenerated{
		UserDekGenerated: wrappedPIIDEK,
	}})
	piiDEKEvent.CreatedAt = now
	accountCreated := newEvent(eventActorID, &evtv1.Event{Event: &evtv1.Event_UserAccountCreated{
		UserAccountCreated: &evtv1.UserAccountCreatedEvent{
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
	if options.initialDeactivated {
		deactivation := newEvent(eventActorID, &evtv1.Event{Event: &evtv1.Event_UserAccountDeactivationChanged{
			UserAccountDeactivationChanged: &evtv1.UserAccountDeactivationChangedEvent{UserId: userID, Deactivated: true},
		}})
		deactivation.CreatedAt = now
		entries = append(entries, evtstream.BatchEntry{Subject: agg.Subject(evtstream.EventUserAccountDeactivationChanged), Event: deactivation})
	}
	if isBot && !options.credentialFreeBot {
		keyCreated := newEvent(eventActorID, &evtv1.Event{Event: &evtv1.Event_BotApiKeyCreated{
			BotApiKeyCreated: &evtv1.BotApiKeyCreatedEvent{
				UserId: userID, Verifier: botAPIKeyVerifier,
				KeyId: legacyBotAPIKeyID, Name: options.botAPIKeyName,
			},
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
		passwordChanged := newEvent(eventActorID, &evtv1.Event{Event: &evtv1.Event_UserPasswordHashChanged{
			UserPasswordHashChanged: &evtv1.UserPasswordHashChangedEvent{
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

	var invitationEntry *evtstream.BatchEntry
	if options.invitationID != "" {
		invitationEvent := newEvent(eventActorID, &evtv1.Event{Event: &evtv1.Event_InvitationRedeemed{
			InvitationRedeemed: &evtv1.InvitationRedeemedEvent{InvitationId: options.invitationID, UserId: userID},
		}})
		invitationEvent.CreatedAt = now
		invitationEntry = &evtstream.BatchEntry{
			Subject: evtstream.InvitationAggregate(options.invitationID).SubjectFor(invitationEvent),
			Event:   invitationEvent,
		}
	}

	creationEmails := options.importEmails
	if options.verifiedEmail != "" {
		creationEmails = []string{options.verifiedEmail}
	}
	for _, sourceEmail := range creationEmails {
		email := strings.ToLower(strings.TrimSpace(sourceEmail))
		verifiedEmailEvent := newEvent(eventActorID, &evtv1.Event{Event: &evtv1.Event_UserVerifiedEmailAdded{
			UserVerifiedEmailAdded: &evtv1.UserVerifiedEmailAddedEvent{UserId: userID},
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
		externalEvent := newEvent(eventActorID, &evtv1.Event{Event: &evtv1.Event_UserExternalIdentityLinked{
			UserExternalIdentityLinked: &evtv1.UserExternalIdentityLinkedEvent{
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

	if options.setup != nil {
		entries = append(entries, setupCompletionEntries(userID, options.setup)...)
		// An ambiguous publish acknowledgement can still mean the batch committed.
		// Retain these keys once setup attempts a publish; deleting them could
		// destroy the only owner's credentials and profile after successful setup.
		cleanupEncryptionKey = false
	}
	if options.importReceipt != nil {
		for _, entry := range entries {
			entry.Event.ImportContext = options.importContext
		}
		entries = append(entries, options.importReceipt(userID))
		cleanupEncryptionKey = false
	}
	check := func() error {
		if options.setup != nil {
			if err := c.requireSetupAvailable(ctx); err != nil {
				return err
			}
		} else if options.verifiedEmail != "" || options.external != nil {
			required, err := c.SetupRequired(ctx)
			if err != nil {
				return err
			}
			if required {
				return ErrSetupRequired
			}
		}
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
		for _, email := range creationEmails {
			if _, claimed := c.userModel.emailOwnerID(email); claimed {
				return ErrEmailAlreadyVerified
			}
		}
		if options.external != nil {
			if _, claimed := c.userModel.externalIdentityOwnerID(options.external.Issuer, options.external.Subject); claimed {
				return ErrExternalIdentityAlreadyClaimed
			}
		}
		if (len(creationEmails) > 0 || options.external != nil) && c.config.Limits.MaxUsersOrDefault() >= 0 {
			if err := c.requireVerifiedAccountCapacity(ctx, ""); err != nil {
				return err
			}
		}
		return nil
	}
	_, err = c.appendUserBatchWithMentionableCompose(ctx, userID, maxUserMutationRetries, func() ([]evtstream.BatchEntry, error) {
		if err := check(); err != nil {
			return nil, err
		}
		if options.selfService {
			if err := c.WaitForRegistrationPolicy(ctx); err != nil {
				return nil, err
			}
			switch c.ConfigModel().RegistrationAdmission() {
			case evtv1.RegistrationAdmission_REGISTRATION_ADMISSION_OPEN:
				return entries, nil
			case evtv1.RegistrationAdmission_REGISTRATION_ADMISSION_INVITE_ONLY:
				if invitationEntry == nil {
					return nil, ErrInvitationInvalid
				}
			default:
				return nil, ErrInvitationInvalid
			}
		}
		if invitationEntry != nil {
			if _, err := c.invitationModel.validateIDAt(options.invitationID, time.Now()); err != nil {
				return nil, err
			}
			return append(slices.Clone(entries), *invitationEntry), nil
		}
		return entries, nil
	})
	if err != nil {
		if options.setup != nil && errors.Is(err, ErrSetupUnavailable) {
			// A rejected OCC recheck is a definite non-commit for this user.
			cleanupEncryptionKey = true
		}
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
		if err := c.AssignConfiguredOwnerRole(ctx, userID); err != nil {
			c.logger.Warn("Failed to auto-assign owner role on signup", "user_id", userID, "error", err)
		}
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
// This trusted entry point bypasses registration admission. Self-service email
// registration must use CreateSelfServiceUser, even when registration is open.
func (c *ChattoCore) CreateVerifiedUser(ctx context.Context, actorID, login, displayName, password, email string) (*readmodel.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, ErrInvalidArgument
	}
	return c.createUserWithOptions(ctx, actorID, login, displayName, password, userCreationOptions{verifiedEmail: email})
}

// CreateSelfServiceUser completes self-service email registration with an
// already-verified address under the current admission policy.
// An empty invitation is allowed only when registration is open.
// Invitation redemption and account creation commit atomically when required.
func (c *ChattoCore) CreateSelfServiceUser(ctx context.Context, actorID, login, displayName, password, email, invitationID string) (*readmodel.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, ErrInvalidArgument
	}
	invitationID = strings.TrimSpace(invitationID)
	return c.createUserWithOptions(ctx, actorID, login, displayName, password, userCreationOptions{
		verifiedEmail: email,
		invitationID:  invitationID,
		selfService:   true,
	})
}

// rollbackUserCreation undoes the persisted writes performed by CreateUser. Best-effort —
// failures are logged but not returned, since the caller is already in an error path.
func (c *ChattoCore) rollbackUserCreation(ctx context.Context, user *readmodel.User) {
	c.logger.Warn("rolling back user creation", "user_id", user.Id)
	_ = c.DeleteUser(ctx, "system:rollback", user.Id)
}

// GetUser retrieves a user from the user projection.
func (c *ChattoCore) GetUser(ctx context.Context, userID string) (*readmodel.User, error) {
	return c.userModel.GetUser(ctx, userID)
}

// GetUserReference retrieves a public user reference. Deleted or crypto-shredded
// users are returned as tombstones; unknown users still return ErrNotFound.
func (c *ChattoCore) GetUserReference(ctx context.Context, userID string) (*readmodel.User, error) {
	user, ok, err := c.userModel.userReference(ctx, userID)
	if err != nil {
		return nil, err
	}
	if ok {
		return user, nil
	}
	return nil, ErrNotFound
}

// GetUserReferences returns public user references in request order. A deleted
// account has an explicit tombstone; an unknown or not-yet-projected ID is nil.
func (c *ChattoCore) GetUserReferences(ctx context.Context, userIDs []string) ([]*readmodel.User, error) {
	return c.userModel.userReferences(ctx, userIDs)
}

// GetUserByLogin retrieves a user by their login name using the login index.
func (c *ChattoCore) GetUserByLogin(ctx context.Context, login string) (*readmodel.User, error) {
	return c.userModel.GetUserByLogin(ctx, login)
}

// ListUsers retrieves all users from the user projection.
// CountUsers returns the total number of users on the server.
func (c *ChattoCore) CountUsers(ctx context.Context) (int, error) {
	return c.userModel.userCount(), nil
}

func (c *ChattoCore) ListUsers(ctx context.Context) ([]*readmodel.User, error) {
	return c.userModel.allUsers(ctx)
}

// ============================================================================
// Login Validation
// ============================================================================

// ErrLoginAlreadyTaken is returned when the login name is already taken.
var ErrLoginAlreadyTaken = fmt.Errorf("login name is already taken")

// ErrUsernameBlocked is returned when the login name is in the blocked list.
var ErrUsernameBlocked = fmt.Errorf("this username is not available")

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
