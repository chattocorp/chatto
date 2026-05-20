package core

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
)

// ============================================================================
// ConfigManager Tests
// ============================================================================

func TestConfigManager_GetServerConfig(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("returns nil and false when not configured", func(t *testing.T) {
		cfg, isConfigured, err := core.configManager.GetServerConfig(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if isConfigured {
			t.Error("expected isConfigured to be false for fresh instance")
		}
		if cfg != nil {
			t.Error("expected nil config for fresh instance")
		}
	})

	t.Run("returns config after SetServerConfig", func(t *testing.T) {
		testCfg := &configv1.ServerConfig{
			ServerName: "Test Instance",
			WelcomeMessage: "Welcome!",
			Motd:           "Message of the day",
		}

		err := core.configManager.SetServerConfig(ctx, testCfg)
		if err != nil {
			t.Fatalf("failed to set config: %v", err)
		}

		cfg, isConfigured, err := core.configManager.GetServerConfig(ctx)
		if err != nil {
			t.Fatalf("failed to get config: %v", err)
		}
		if !isConfigured {
			t.Error("expected isConfigured to be true")
		}
		if cfg.ServerName != "Test Instance" {
			t.Errorf("expected instance name 'Test Instance', got '%s'", cfg.ServerName)
		}
		if cfg.WelcomeMessage != "Welcome!" {
			t.Errorf("expected welcome message 'Welcome!', got '%s'", cfg.WelcomeMessage)
		}
		if cfg.Motd != "Message of the day" {
			t.Errorf("expected MOTD 'Message of the day', got '%s'", cfg.Motd)
		}
	})
}

func TestConfigManager_UpdateServerConfigFunc(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("creates config when none exists", func(t *testing.T) {
		// Reset to ensure clean state
		core.configManager.ResetServerConfig(ctx)

		cfg, err := core.configManager.UpdateServerConfigFunc(ctx, func(current *configv1.ServerConfig) (*configv1.ServerConfig, error) {
			if current != nil {
				t.Error("expected nil current config for fresh instance")
			}
			return &configv1.ServerConfig{
				ServerName: "Created via UpdateFunc",
			}, nil
		})

		if err != nil {
			t.Fatalf("failed to update config: %v", err)
		}
		if cfg.ServerName != "Created via UpdateFunc" {
			t.Errorf("expected instance name 'Created via UpdateFunc', got '%s'", cfg.ServerName)
		}
	})

	t.Run("updates existing config", func(t *testing.T) {
		// Set initial config
		core.configManager.SetServerConfig(ctx, &configv1.ServerConfig{
			ServerName: "Original Name",
			Motd:         "Original MOTD",
		})

		cfg, err := core.configManager.UpdateServerConfigFunc(ctx, func(current *configv1.ServerConfig) (*configv1.ServerConfig, error) {
			if current == nil {
				t.Error("expected non-nil current config")
			}
			if current.ServerName != "Original Name" {
				t.Errorf("expected current instance name 'Original Name', got '%s'", current.ServerName)
			}
			current.ServerName = "Updated Name"
			return current, nil
		})

		if err != nil {
			t.Fatalf("failed to update config: %v", err)
		}
		if cfg.ServerName != "Updated Name" {
			t.Errorf("expected instance name 'Updated Name', got '%s'", cfg.ServerName)
		}
		// Verify MOTD was preserved
		if cfg.Motd != "Original MOTD" {
			t.Errorf("expected MOTD 'Original MOTD' to be preserved, got '%s'", cfg.Motd)
		}
	})

	t.Run("propagates update function errors", func(t *testing.T) {
		expectedErr := errors.New("update function error")

		_, err := core.configManager.UpdateServerConfigFunc(ctx, func(current *configv1.ServerConfig) (*configv1.ServerConfig, error) {
			return nil, expectedErr
		})

		if err != expectedErr {
			t.Errorf("expected error '%v', got '%v'", expectedErr, err)
		}
	})

	t.Run("handles concurrent updates with OCC", func(t *testing.T) {
		// Reset and set initial config
		core.configManager.ResetServerConfig(ctx)
		core.configManager.SetServerConfig(ctx, &configv1.ServerConfig{
			ServerName: "Concurrent Test",
		})

		const numGoroutines = 10
		var wg sync.WaitGroup
		var successCount atomic.Int32
		var conflictCount atomic.Int32

		// Launch concurrent updates
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				_, err := core.configManager.UpdateServerConfigFunc(ctx, func(current *configv1.ServerConfig) (*configv1.ServerConfig, error) {
					if current == nil {
						current = &configv1.ServerConfig{}
					}
					current.Motd = "Updated by goroutine"
					return current, nil
				})

				if err == nil {
					successCount.Add(1)
				} else if errors.Is(err, ErrConfigConflict) {
					conflictCount.Add(1)
				}
			}(i)
		}

		wg.Wait()

		// All updates should eventually succeed (retries handle conflicts)
		// OR fail with ErrConfigConflict after max retries
		total := successCount.Load() + conflictCount.Load()
		if total != numGoroutines {
			t.Errorf("expected %d total results, got %d (success: %d, conflict: %d)",
				numGoroutines, total, successCount.Load(), conflictCount.Load())
		}

		// At least some should succeed
		if successCount.Load() == 0 {
			t.Error("expected at least one successful update")
		}
	})
}

func TestConfigManager_ResetServerConfig(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("resets config to unconfigured state", func(t *testing.T) {
		// Set config first
		core.configManager.SetServerConfig(ctx, &configv1.ServerConfig{
			ServerName: "To Be Reset",
		})

		// Verify it's set
		_, isConfigured, _ := core.configManager.GetServerConfig(ctx)
		if !isConfigured {
			t.Fatal("config should be set before reset")
		}

		// Reset
		err := core.configManager.ResetServerConfig(ctx)
		if err != nil {
			t.Fatalf("failed to reset config: %v", err)
		}

		// Verify it's gone
		_, isConfigured, _ = core.configManager.GetServerConfig(ctx)
		if isConfigured {
			t.Error("config should be unconfigured after reset")
		}
	})

	t.Run("no error when resetting unconfigured instance", func(t *testing.T) {
		// Reset twice - should not error
		core.configManager.ResetServerConfig(ctx)
		err := core.configManager.ResetServerConfig(ctx)
		if err != nil {
			t.Errorf("reset should not error for unconfigured instance: %v", err)
		}
	})
}

func TestConfigManager_GetEffectiveWelcomeMessage(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("returns empty string when not configured", func(t *testing.T) {
		core.configManager.ResetServerConfig(ctx)

		msg, err := core.configManager.GetEffectiveWelcomeMessage(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msg != "" {
			t.Errorf("expected empty string, got '%s'", msg)
		}
	})

	t.Run("returns configured welcome message", func(t *testing.T) {
		core.configManager.SetServerConfig(ctx, &configv1.ServerConfig{
			WelcomeMessage: "Hello, world!",
		})

		msg, err := core.configManager.GetEffectiveWelcomeMessage(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msg != "Hello, world!" {
			t.Errorf("expected 'Hello, world!', got '%s'", msg)
		}
	})
}

func TestConfigManager_GetEffectiveServerName(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("returns 'Chatto' when not configured", func(t *testing.T) {
		core.configManager.ResetServerConfig(ctx)

		name, err := core.configManager.GetEffectiveServerName(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "Chatto" {
			t.Errorf("expected 'Chatto', got '%s'", name)
		}
	})

	t.Run("returns 'Chatto' when configured with empty name", func(t *testing.T) {
		core.configManager.SetServerConfig(ctx, &configv1.ServerConfig{
			ServerName: "",
		})

		name, err := core.configManager.GetEffectiveServerName(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "Chatto" {
			t.Errorf("expected 'Chatto', got '%s'", name)
		}
	})

	t.Run("returns configured instance name", func(t *testing.T) {
		core.configManager.SetServerConfig(ctx, &configv1.ServerConfig{
			ServerName: "My Custom Instance",
		})

		name, err := core.configManager.GetEffectiveServerName(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "My Custom Instance" {
			t.Errorf("expected 'My Custom Instance', got '%s'", name)
		}
	})
}

func TestConfigManager_GetEffectiveMOTD(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("returns empty string when not configured", func(t *testing.T) {
		core.configManager.ResetServerConfig(ctx)

		motd, err := core.configManager.GetEffectiveMOTD(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if motd != "" {
			t.Errorf("expected empty string, got '%s'", motd)
		}
	})

	t.Run("returns configured MOTD", func(t *testing.T) {
		core.configManager.SetServerConfig(ctx, &configv1.ServerConfig{
			Motd: "Today's announcement",
		})

		motd, err := core.configManager.GetEffectiveMOTD(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if motd != "Today's announcement" {
			t.Errorf("expected 'Today's announcement', got '%s'", motd)
		}
	})
}

func TestConfigManager_BlockedUsernames(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("returns default blocked usernames when not configured", func(t *testing.T) {
		core.configManager.ResetServerConfig(ctx)

		blocked, err := core.configManager.GetEffectiveBlockedUsernames(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if blocked != DefaultBlockedUsernames {
			t.Errorf("expected default blocked usernames, got '%s'", blocked)
		}
	})

	t.Run("returns configured blocked usernames", func(t *testing.T) {
		core.configManager.SetServerConfig(ctx, &configv1.ServerConfig{
			BlockedUsernames: "blocked1\nblocked2",
		})

		blocked, err := core.configManager.GetEffectiveBlockedUsernames(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if blocked != "blocked1\nblocked2" {
			t.Errorf("expected 'blocked1\\nblocked2', got '%s'", blocked)
		}
	})

	t.Run("returns empty when admin explicitly clears", func(t *testing.T) {
		core.configManager.SetServerConfig(ctx, &configv1.ServerConfig{
			BlockedUsernames: "", // Admin explicitly cleared
		})

		blocked, err := core.configManager.GetEffectiveBlockedUsernames(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if blocked != "" {
			t.Errorf("expected empty string, got '%s'", blocked)
		}
	})
}

func TestConfigManager_GetBlockedUsernamesList(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("parses default blocked usernames into list", func(t *testing.T) {
		core.configManager.ResetServerConfig(ctx)

		list, err := core.configManager.GetBlockedUsernamesList(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Check that default blocked usernames are parsed
		expected := []string{"root", "admin", "superuser", "op", "operator", "support"}
		if len(list) != len(expected) {
			t.Errorf("expected %d blocked usernames, got %d", len(expected), len(list))
		}
		for i, name := range expected {
			if i < len(list) && list[i] != name {
				t.Errorf("expected blocked username %d to be '%s', got '%s'", i, name, list[i])
			}
		}
	})

	t.Run("handles empty lines and whitespace", func(t *testing.T) {
		core.configManager.SetServerConfig(ctx, &configv1.ServerConfig{
			BlockedUsernames: "  user1  \n\nuser2\n  \nUSER3  ",
		})

		list, err := core.configManager.GetBlockedUsernamesList(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(list) != 3 {
			t.Errorf("expected 3 blocked usernames, got %d: %v", len(list), list)
		}

		// All should be lowercase
		if list[0] != "user1" || list[1] != "user2" || list[2] != "user3" {
			t.Errorf("expected ['user1', 'user2', 'user3'], got %v", list)
		}
	})
}

func TestConfigManager_IsUsernameBlocked(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("blocks default usernames", func(t *testing.T) {
		core.configManager.ResetServerConfig(ctx)

		blocked, err := core.configManager.IsUsernameBlocked(ctx, "admin")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !blocked {
			t.Error("expected 'admin' to be blocked by default")
		}
	})

	t.Run("case-insensitive blocking", func(t *testing.T) {
		core.configManager.ResetServerConfig(ctx)

		blocked, err := core.configManager.IsUsernameBlocked(ctx, "ADMIN")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !blocked {
			t.Error("expected 'ADMIN' to be blocked (case-insensitive)")
		}

		blocked, err = core.configManager.IsUsernameBlocked(ctx, "Root")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !blocked {
			t.Error("expected 'Root' to be blocked (case-insensitive)")
		}
	})

	t.Run("allows non-blocked usernames", func(t *testing.T) {
		core.configManager.ResetServerConfig(ctx)

		blocked, err := core.configManager.IsUsernameBlocked(ctx, "regularuser")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if blocked {
			t.Error("expected 'regularuser' to not be blocked")
		}
	})

	t.Run("respects custom blocked list", func(t *testing.T) {
		core.configManager.SetServerConfig(ctx, &configv1.ServerConfig{
			BlockedUsernames: "customblocked",
		})

		// Custom blocked username should be blocked
		blocked, err := core.configManager.IsUsernameBlocked(ctx, "customblocked")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !blocked {
			t.Error("expected 'customblocked' to be blocked")
		}

		// Default blocked username should NOT be blocked anymore
		blocked, err = core.configManager.IsUsernameBlocked(ctx, "admin")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if blocked {
			t.Error("expected 'admin' to NOT be blocked with custom list")
		}
	})
}

func TestParseBlockedUsernames(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single username",
			input:    "admin",
			expected: []string{"admin"},
		},
		{
			name:     "multiple usernames",
			input:    "admin\nroot\noperator",
			expected: []string{"admin", "root", "operator"},
		},
		{
			name:     "with whitespace",
			input:    "  admin  \n  root  ",
			expected: []string{"admin", "root"},
		},
		{
			name:     "with empty lines",
			input:    "admin\n\nroot\n\n\noperator",
			expected: []string{"admin", "root", "operator"},
		},
		{
			name:     "converts to lowercase",
			input:    "ADMIN\nRoot\nOPERATOR",
			expected: []string{"admin", "root", "operator"},
		},
		{
			name:     "only whitespace lines",
			input:    "  \n  \n  ",
			expected: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := parseBlockedUsernames(tc.input)

			if tc.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tc.expected) {
				t.Errorf("expected %d items, got %d: %v", len(tc.expected), len(result), result)
				return
			}

			for i, exp := range tc.expected {
				if result[i] != exp {
					t.Errorf("expected item %d to be '%s', got '%s'", i, exp, result[i])
				}
			}
		})
	}
}
