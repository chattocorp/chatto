package core

import (
	"testing"
)

func TestExtractMentionUsernames(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected []string
	}{
		{
			name:     "no mentions",
			body:     "Hello world!",
			expected: nil,
		},
		{
			name:     "single mention",
			body:     "Hey @alice, how are you?",
			expected: []string{"alice"},
		},
		{
			name:     "multiple mentions",
			body:     "@alice and @bob should check this out",
			expected: []string{"alice", "bob"},
		},
		{
			name:     "duplicate mentions deduplicated",
			body:     "@alice said hi to @bob, and @alice replied",
			expected: []string{"alice", "bob"},
		},
		{
			name:     "mention at start",
			body:     "@admin please help",
			expected: []string{"admin"},
		},
		{
			name:     "mention at end",
			body:     "Thanks @helper",
			expected: []string{"helper"},
		},
		{
			name:     "mention with underscore",
			body:     "Hey @user_name!",
			expected: []string{"user_name"},
		},
		{
			name:     "mention with hyphen",
			body:     "Check with @first-last",
			expected: []string{"first-last"},
		},
		{
			name:     "mention with dot in username",
			body:     "Hey @hendrik.mans check this",
			expected: []string{"hendrik.mans"},
		},
		{
			name:     "mention with dot followed by punctuation",
			body:     "Thanks @hendrik.mans.",
			expected: []string{"hendrik.mans"},
		},
		{
			name:     "multiple dots in username",
			body:     "@first.middle.last hello",
			expected: []string{"first.middle.last"},
		},
		{
			name:     "dot at end not captured",
			body:     "Thanks @alice.",
			expected: []string{"alice"},
		},
		{
			name:     "mention with numbers",
			body:     "Ask @user123 about it",
			expected: []string{"user123"},
		},
		{
			name:     "mixed case mentions",
			body:     "@Alice and @ALICE are different extractions",
			expected: []string{"Alice", "ALICE"},
		},
		{
			name:     "email address not a mention",
			body:     "Email me at user@example.com",
			expected: nil, // @ preceded by alphanumeric is not a mention
		},
		{
			name:     "mention followed by punctuation",
			body:     "@alice, @bob! @charlie?",
			expected: []string{"alice", "bob", "charlie"},
		},
		{
			name:     "empty body",
			body:     "",
			expected: nil,
		},
		{
			name:     "just at sign",
			body:     "@ nothing here",
			expected: nil,
		},
		// Additional edge cases for email false positive prevention
		{
			name:     "email with subdomain not a mention",
			body:     "Contact support@mail.company.org",
			expected: nil,
		},
		{
			name:     "mention after newline",
			body:     "Hello\n@alice check this",
			expected: []string{"alice"},
		},
		{
			name:     "mention in parentheses",
			body:     "Ask (@bob) about it",
			expected: []string{"bob"},
		},
		{
			name:     "mention after colon",
			body:     "CC: @charlie",
			expected: []string{"charlie"},
		},
		{
			name:     "multiple emails no mentions",
			body:     "Email john@example.com or jane@company.org",
			expected: nil,
		},
		{
			name:     "mix of email and mention",
			body:     "Email john@example.com or ping @alice",
			expected: []string{"alice"},
		},
		{
			name:     "mention at start of line after newline",
			body:     "Line one\n@alice line two",
			expected: []string{"alice"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractMentionUsernames(tt.body)
			if len(result) != len(tt.expected) {
				t.Errorf("ExtractMentionUsernames(%q) = %v, want %v", tt.body, result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("ExtractMentionUsernames(%q)[%d] = %q, want %q", tt.body, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestChattoCore_ResolveMentions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create users
	alice, err := core.CreateUser(ctx, "system", "alice", "Alice", "password123")
	if err != nil {
		t.Fatalf("Failed to create alice: %v", err)
	}

	bob, err := core.CreateUser(ctx, "system", "bob", "Bob", "password123")
	if err != nil {
		t.Fatalf("Failed to create bob: %v", err)
	}

	// Create a space with alice as owner

	t.Run("empty usernames returns nil", func(t *testing.T) {
		result, err := core.ResolveMentions(ctx, nil)
		if err != nil {
			t.Fatalf("ResolveMentions failed: %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("resolves valid member usernames", func(t *testing.T) {
		result, err := core.ResolveMentions(ctx, []string{"alice", "bob"})
		if err != nil {
			t.Fatalf("ResolveMentions failed: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("Expected 2 user IDs, got %d", len(result))
		}
		// Check both IDs are present (order may vary)
		hasAlice := result[0] == alice.Id || result[1] == alice.Id
		hasBob := result[0] == bob.Id || result[1] == bob.Id
		if !hasAlice || !hasBob {
			t.Errorf("Expected alice and bob IDs, got %v", result)
		}
	})

	t.Run("silently ignores invalid usernames", func(t *testing.T) {
		result, err := core.ResolveMentions(ctx, []string{"alice", "nonexistent", "bob"})
		if err != nil {
			t.Fatalf("ResolveMentions failed: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("Expected 2 user IDs (ignoring invalid), got %d: %v", len(result), result)
		}
	})

	t.Run("resolves every authenticated user", func(t *testing.T) {
		// Post-#330 there's no "non-member" — every authenticated user is part
		// of the server. ResolveMentions returns all valid login matches.
		charlie, err := core.CreateUser(ctx, "system", "charlie", "Charlie", "password123")
		if err != nil {
			t.Fatalf("Failed to create charlie: %v", err)
		}

		result, err := core.ResolveMentions(ctx, []string{"alice", "charlie"})
		if err != nil {
			t.Fatalf("ResolveMentions failed: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("Expected 2 user IDs, got %d: %v", len(result), result)
		}
		seen := map[string]bool{result[0]: true}
		if len(result) > 1 {
			seen[result[1]] = true
		}
		if !seen[alice.Id] || !seen[charlie.Id] {
			t.Errorf("Expected alice and charlie, got %v", result)
		}
	})
}

func TestChattoCore_MentionCreatesNotificationWithoutMentionStatus(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	mentioned, err := core.CreateUser(ctx, "system", "mentioneduser", "Mentioned User", "password123")
	if err != nil {
		t.Fatalf("CreateUser mentioned: %v", err)
	}
	mentioner, err := core.CreateUser(ctx, "system", "mentionauthor", "Mention Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser mentioner: %v", err)
	}
	room, err := core.CreateRoom(ctx, mentioned.Id, KindChannel, "", "mentions", "Mentions")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, mentioner.Id, KindChannel, mentioner.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom mentioner: %v", err)
	}

	if _, err := core.PostMessage(ctx, KindChannel, room.Id, mentioner.Id, "hello @mentioneduser", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	notifications, err := core.GetNotifications(ctx, mentioned.Id)
	if err != nil {
		t.Fatalf("GetNotifications: %v", err)
	}
	if len(notifications) != 1 || notifications[0].GetMention() == nil {
		t.Fatalf("expected one mention notification, got %#v", notifications)
	}

	legacyKey := "room_mention_status." + mentioned.Id + "." + room.Id
	assertLegacyKeyAbsent(t, core.storage.serverRuntimeKV, legacyKey, "legacy mention status key")
}
