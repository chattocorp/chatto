package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestChattoCore_CreateUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if user == nil {
		t.Fatal("Expected user to be returned")
	}

	if user.Id == "" {
		t.Error("Expected user ID to be set")
	}

	if user.Login != "testuser" {
		t.Errorf("Expected login 'testuser', got '%s'", user.Login)
	}

	// Verify we can retrieve the user
	retrieved, err := core.GetUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	if retrieved.Id != user.Id {
		t.Errorf("Expected user ID '%s', got '%s'", user.Id, retrieved.Id)
	}

	// Verify password was stored separately
	_, err = core.VerifyPassword(ctx, user.Login, "password123")
	if err != nil {
		t.Errorf("Expected password to be verifiable: %v", err)
	}
}

func TestChattoCore_CreateUserUsesProvidedActorID(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "actor-attribution", "Actor Attribution", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	accountEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(evtstream.EventUserAccountCreated))
	if err != nil {
		t.Fatalf("SubjectEvents account created: %v", err)
	}
	if len(accountEvents) != 1 {
		t.Fatalf("account created events = %d, want 1", len(accountEvents))
	}
	if got := accountEvents[0].GetActorId(); got != SystemActorID {
		t.Fatalf("account created actor = %q, want %q", got, SystemActorID)
	}

	dekEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(evtstream.EventUserDEKGenerated))
	if err != nil {
		t.Fatalf("SubjectEvents DEK generated: %v", err)
	}
	if len(dekEvents) != 2 {
		t.Fatalf("DEK generated events = %d, want 2", len(dekEvents))
	}
	for _, event := range dekEvents {
		if got := event.GetActorId(); got != SystemActorID {
			t.Fatalf("DEK generated actor = %q, want %q", got, SystemActorID)
		}
	}
}

func TestChattoCore_CreateUserLiveEventUsesProvidedActorID(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	sub, err := nc.SubscribeSync(subjects.LiveSyncAllEvents())
	if err != nil {
		t.Fatalf("SubscribeSync live events: %v", err)
	}
	defer sub.Unsubscribe()
	if err := nc.Flush(); err != nil {
		t.Fatalf("Flush subscription: %v", err)
	}

	user, err := core.CreateUser(ctx, SystemActorID, "actor-live-create", "Actor Live Create", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("waiting for user created live event: %v", err)
	}
	var live corev1.LiveEvent
	if err := proto.Unmarshal(msg.Data, &live); err != nil {
		t.Fatalf("unmarshal live event: %v", err)
	}
	created := live.GetUserCreated()
	if created == nil {
		t.Fatalf("expected UserCreatedSyncEvent, got %T", live.Event)
	}
	if created.GetUserId() != user.GetId() {
		t.Fatalf("created live user_id = %q, want %q", created.GetUserId(), user.GetId())
	}
	if got := live.GetActorId(); got != SystemActorID {
		t.Fatalf("created live actor = %q, want %q", got, SystemActorID)
	}
}

type cancelAfterWrapKeyWrapper struct {
	kms.KeyWrapper
	cancel    context.CancelFunc
	wrapped   bool
	wrappedBy string
}

func (w *cancelAfterWrapKeyWrapper) WrapContentKey(ctx context.Context, keyRef string, contentKey, aad []byte) (*kms.WrappedContentKey, error) {
	wrapped, err := w.KeyWrapper.WrapContentKey(ctx, keyRef, contentKey, aad)
	if err == nil && !w.wrapped {
		w.wrapped = true
		w.wrappedBy = keyRef
		w.cancel()
	}
	return wrapped, err
}

func TestChattoCore_CreateUser_AppendFailureCleansUpEncryptionKey(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx, cancel := context.WithCancel(testContext(t))
	wrapper := &cancelAfterWrapKeyWrapper{
		KeyWrapper: core.encryption.keyWrapper,
		cancel:     cancel,
	}
	core.encryption.keyWrapper = wrapper

	_, err := core.CreateUser(ctx, "system", "cancelled-signup", "Cancelled Signup", "password123")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateUser error = %v, want context.Canceled", err)
	}
	if !wrapper.wrapped {
		t.Fatal("test did not reach content-key wrapping")
	}

	exists, err := wrapper.KeyWrapper.KeyExists(context.Background(), wrapper.wrappedBy)
	if err != nil {
		t.Fatalf("KeyExists: %v", err)
	}
	if exists {
		t.Fatalf("encryption key for failed signup key ref %q still exists", wrapper.wrappedBy)
	}
}

func TestChattoCore_GetUser_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.GetUser(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting nonexistent user")
	}
}

func TestChattoCore_GetUserByLogin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	created, err := core.CreateUser(ctx, "system", "mylogin", "mylogin", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Retrieve by login
	retrieved, err := core.GetUserByLogin(ctx, "mylogin")
	if err != nil {
		t.Fatalf("Failed to get user by login: %v", err)
	}

	if retrieved.Id != created.Id {
		t.Errorf("Expected user ID '%s', got '%s'", created.Id, retrieved.Id)
	}

	if retrieved.Login != "mylogin" {
		t.Errorf("Expected login 'mylogin', got '%s'", retrieved.Login)
	}
}

func TestChattoCore_GetUserByLogin_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.GetUserByLogin(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting user by nonexistent login")
	}
}

func TestChattoCore_ConcurrentUserCreation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Try to create two users with the same login concurrently
	// One should succeed, one should fail (atomic login claim)
	login := "concurrent"
	errChan := make(chan error, 2)
	userChan := make(chan string, 2)

	createUser := func(displayName string) {
		user, err := core.CreateUser(ctx, "system", login, displayName, "password123")
		if err != nil {
			errChan <- err
			userChan <- ""
		} else {
			errChan <- nil
			userChan <- user.Id
		}
	}

	go createUser("User 1")
	go createUser("User 2")

	// Collect results
	err1 := <-errChan
	err2 := <-errChan
	user1 := <-userChan
	user2 := <-userChan

	// Exactly one should succeed and one should fail
	successCount := 0
	if err1 == nil {
		successCount++
	}
	if err2 == nil {
		successCount++
	}

	if successCount != 1 {
		t.Errorf("Expected exactly 1 success, got %d (err1=%v, err2=%v)", successCount, err1, err2)
	}

	// Verify only one user exists with this login
	retrieved, err := core.GetUserByLogin(ctx, login)
	if err != nil {
		t.Fatalf("Failed to get user by login: %v", err)
	}

	// The retrieved user should match one of the attempted creations
	if user1 != "" && retrieved.Id != user1 {
		t.Errorf("Expected user ID %s, got %s", user1, retrieved.Id)
	}
	if user2 != "" && retrieved.Id != user2 {
		t.Errorf("Expected user ID %s, got %s", user2, retrieved.Id)
	}
}

func TestChattoCore_CreateUser_BlockedUsername(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Default blocked usernames include: root, admin, superuser, op, operator, support
	blockedNames := []string{"admin", "root", "superuser", "op", "operator", "support"}

	for _, name := range blockedNames {
		t.Run(name, func(t *testing.T) {
			_, err := core.CreateUser(ctx, "system", name, name, "password123")
			if err == nil {
				t.Errorf("Expected error when creating user with blocked username '%s'", name)
			}
			if err != ErrUsernameBlocked {
				t.Errorf("Expected ErrUsernameBlocked, got: %v", err)
			}
		})
	}

	// Also test case-insensitivity
	t.Run("ADMIN (uppercase)", func(t *testing.T) {
		_, err := core.CreateUser(ctx, "system", "ADMIN", "ADMIN", "password123")
		if err == nil {
			t.Error("Expected error when creating user with blocked username 'ADMIN'")
		}
		if err != ErrUsernameBlocked {
			t.Errorf("Expected ErrUsernameBlocked, got: %v", err)
		}
	})

	t.Run("Admin (mixed case)", func(t *testing.T) {
		_, err := core.CreateUser(ctx, "system", "Admin", "Admin", "password123")
		if err == nil {
			t.Error("Expected error when creating user with blocked username 'Admin'")
		}
		if err != ErrUsernameBlocked {
			t.Errorf("Expected ErrUsernameBlocked, got: %v", err)
		}
	})
}

func TestChattoCore_CreateUser_MentionNamespaceReserved(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	if _, err := core.CreateServerRole(ctx, SystemActorID, "helpdesk", "Helpdesk", ""); err != nil {
		t.Fatalf("CreateServerRole helpdesk: %v", err)
	}

	for _, login := range []string{"all", "here", "helpdesk", "HELPDESK"} {
		t.Run(login, func(t *testing.T) {
			_, err := core.CreateUser(ctx, "system", login, login, "password123")
			if !errors.Is(err, ErrUsernameBlocked) {
				t.Fatalf("CreateUser(%q) error = %v, want ErrUsernameBlocked", login, err)
			}
		})
	}
}
