package dataloader_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/dataloader"
	"hmans.de/chatto/internal/testutil"
)

// setupTestCore creates a ChattoCore with an embedded NATS server for testing.
func setupTestCore(t *testing.T) *core.ChattoCore {
	t.Helper()

	_, nc := testutil.StartSharedNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	cfg := config.CoreConfig{
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}
	c, err := core.NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}

	runCtx, runCancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- c.Run(runCtx) }()
	t.Cleanup(func() {
		runCancel()
		select {
		case <-runDone:
		case <-time.After(5 * time.Second):
			t.Fatal("core.Run did not stop within timeout")
		}
	})

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bootCancel()
	if err := c.WaitForBoot(bootCtx); err != nil {
		t.Fatalf("WaitForBoot: %v", err)
	}

	return c
}

func TestUserLoader_LoadSingleUser(t *testing.T) {
	c := setupTestCore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a user
	user, err := c.CreateUser(ctx, "", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create loaders
	loaders := dataloader.NewLoaders(c)

	// Load the user
	loaded, err := loaders.GetUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to load user: %v", err)
	}

	if loaded.Id != user.Id {
		t.Errorf("Expected user ID %s, got %s", user.Id, loaded.Id)
	}

	if loaded.Login != "testuser" {
		t.Errorf("Expected login 'testuser', got %s", loaded.Login)
	}
}

func TestUserLoader_LoadNotFound(t *testing.T) {
	c := setupTestCore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loaders := dataloader.NewLoaders(c)

	_, err := loaders.GetUser(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("Expected error for nonexistent user")
	}

	if err != core.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestUserLoader_BatchesConcurrentLoads(t *testing.T) {
	c := setupTestCore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create multiple users
	users := make([]*struct{ id, login string }, 5)
	for i := 0; i < 5; i++ {
		login := "user" + string(rune('a'+i))
		user, err := c.CreateUser(ctx, "", login, "User "+login, "password123")
		if err != nil {
			t.Fatalf("Failed to create user %d: %v", i, err)
		}
		users[i] = &struct{ id, login string }{user.Id, login}
	}

	// Create loaders
	loaders := dataloader.NewLoaders(c)

	// Load all users concurrently
	var wg sync.WaitGroup
	results := make([]string, len(users))
	errors := make([]error, len(users))

	for i, u := range users {
		wg.Add(1)
		go func(idx int, userID string) {
			defer wg.Done()
			loaded, err := loaders.GetUser(ctx, userID)
			if err != nil {
				errors[idx] = err
				return
			}
			results[idx] = loaded.Id
		}(i, u.id)
	}
	wg.Wait()

	// Verify all users loaded correctly
	for i, u := range users {
		if errors[i] != nil {
			t.Errorf("Error loading user %d: %v", i, errors[i])
			continue
		}
		if results[i] != u.id {
			t.Errorf("Expected user ID %s, got %s", u.id, results[i])
		}
	}
}

func TestUserLoader_CachesSameRequest(t *testing.T) {
	c := setupTestCore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a user
	user, err := c.CreateUser(ctx, "", "cachetest", "Cache Test", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	loaders := dataloader.NewLoaders(c)

	// Load the same user twice
	loaded1, err := loaders.GetUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("First load failed: %v", err)
	}

	loaded2, err := loaders.GetUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("Second load failed: %v", err)
	}

	// Both should return the same user
	if loaded1.Id != loaded2.Id {
		t.Errorf("Expected same user, got different IDs: %s vs %s", loaded1.Id, loaded2.Id)
	}
}

func TestUserLoader_GetUsers(t *testing.T) {
	c := setupTestCore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create multiple users
	user1, err := c.CreateUser(ctx, "", "multi1", "Multi 1", "password123")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}

	user2, err := c.CreateUser(ctx, "", "multi2", "Multi 2", "password123")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	loaders := dataloader.NewLoaders(c)

	// Load multiple users at once
	users, err := loaders.GetUsers(ctx, []string{user1.Id, user2.Id})
	if err != nil {
		t.Fatalf("Failed to load users: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("Expected 2 users, got %d", len(users))
	}

	if users[0].Id != user1.Id {
		t.Errorf("Expected first user ID %s, got %s", user1.Id, users[0].Id)
	}

	if users[1].Id != user2.Id {
		t.Errorf("Expected second user ID %s, got %s", user2.Id, users[1].Id)
	}
}

func TestContext_ForContext_ReturnsNilWhenNotSet(t *testing.T) {
	ctx := context.Background()

	loaders := dataloader.ForContext(ctx)
	if loaders != nil {
		t.Error("Expected nil when loaders not set in context")
	}
}

func TestContext_WithLoaders_InjectsLoaders(t *testing.T) {
	c := setupTestCore(t)
	loaders := dataloader.NewLoaders(c)

	ctx := context.Background()
	ctx = dataloader.WithLoaders(ctx, loaders)

	retrieved := dataloader.ForContext(ctx)
	if retrieved == nil {
		t.Fatal("Expected loaders to be retrievable from context")
	}

	if retrieved != loaders {
		t.Error("Expected same loaders instance")
	}
}
