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
// Key Helper Tests
// ============================================================================

func TestMentionStatusKey(t *testing.T) {
	tests := []struct {
		userID   string
		roomID   string
		expected string
	}{
		{"user123", "room456", "room_mention_status.user123.room456"},
		{"abc", "def", "room_mention_status.abc.def"},
	}

	for _, tt := range tests {
		t.Run(tt.userID+"_"+tt.roomID, func(t *testing.T) {
			result := mentionStatusKey(tt.userID, tt.roomID)
			if result != tt.expected {
				t.Errorf("mentionStatusKey(%q, %q) = %q, want %q", tt.userID, tt.roomID, result, tt.expected)
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
	space, err := core.CreateSpace(ctx, alice.Id, "Test Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	// bob joins the space

	t.Run("empty usernames returns nil", func(t *testing.T) {
		result, err := core.ResolveMentions(ctx, space.Id, nil)
		if err != nil {
			t.Fatalf("ResolveMentions failed: %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("resolves valid member usernames", func(t *testing.T) {
		result, err := core.ResolveMentions(ctx, space.Id, []string{"alice", "bob"})
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
		result, err := core.ResolveMentions(ctx, space.Id, []string{"alice", "nonexistent", "bob"})
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

		result, err := core.ResolveMentions(ctx, space.Id, []string{"alice", "charlie"})
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

func TestChattoCore_MentionStatus(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and space
	user, err := core.CreateUser(ctx, "system", "mentionuser", "Mention User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	space, err := core.CreateSpace(ctx, user.Id, "Mention Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	room, err := core.CreateRoom(ctx, user.Id, space.Id, "general", "General chat")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	t.Run("HasMention returns false when no mention exists", func(t *testing.T) {
		hasMention, err := core.HasMention(ctx, space.Id, room.Id, user.Id)
		if err != nil {
			t.Fatalf("HasMention failed: %v", err)
		}
		if hasMention {
			t.Error("Expected no mention, but HasMention returned true")
		}
	})

	t.Run("setMentionStatus creates mention indicator", func(t *testing.T) {
		err := core.setMentionStatus(ctx, space.Id, room.Id, user.Id)
		if err != nil {
			t.Fatalf("setMentionStatus failed: %v", err)
		}

		hasMention, err := core.HasMention(ctx, space.Id, room.Id, user.Id)
		if err != nil {
			t.Fatalf("HasMention failed: %v", err)
		}
		if !hasMention {
			t.Error("Expected mention after setMentionStatus, but HasMention returned false")
		}
	})

	t.Run("setMentionStatus is idempotent - preserves first mention", func(t *testing.T) {
		// Attempt to set another mention - should not fail
		err := core.setMentionStatus(ctx, space.Id, room.Id, user.Id)
		if err != nil {
			t.Fatalf("setMentionStatus (second call) failed: %v", err)
		}

		// Still has mention
		hasMention, err := core.HasMention(ctx, space.Id, room.Id, user.Id)
		if err != nil {
			t.Fatalf("HasMention failed: %v", err)
		}
		if !hasMention {
			t.Error("Expected mention to persist after second setMentionStatus")
		}
	})

	t.Run("ClearMentionStatus removes mention indicator", func(t *testing.T) {
		err := core.ClearMentionStatus(ctx, space.Id, room.Id, user.Id)
		if err != nil {
			t.Fatalf("ClearMentionStatus failed: %v", err)
		}

		hasMention, err := core.HasMention(ctx, space.Id, room.Id, user.Id)
		if err != nil {
			t.Fatalf("HasMention failed: %v", err)
		}
		if hasMention {
			t.Error("Expected no mention after ClearMentionStatus, but HasMention returned true")
		}
	})

	t.Run("ClearMentionStatus is idempotent", func(t *testing.T) {
		// Clear again - should not fail even though already cleared
		err := core.ClearMentionStatus(ctx, space.Id, room.Id, user.Id)
		if err != nil {
			t.Fatalf("ClearMentionStatus (second call) failed: %v", err)
		}
	})

	t.Run("mention status is room-specific", func(t *testing.T) {
		// Create another room
		room2, err := core.CreateRoom(ctx, user.Id, space.Id, "random", "Random chat")
		if err != nil {
			t.Fatalf("Failed to create room2: %v", err)
		}

		// Set mention in room2
		err = core.setMentionStatus(ctx, space.Id, room2.Id, user.Id)
		if err != nil {
			t.Fatalf("setMentionStatus failed: %v", err)
		}

		// room1 should still have no mention (we cleared it above)
		hasMention1, _ := core.HasMention(ctx, space.Id, room.Id, user.Id)
		if hasMention1 {
			t.Error("room1 should not have mention")
		}

		// room2 should have mention
		hasMention2, _ := core.HasMention(ctx, space.Id, room2.Id, user.Id)
		if !hasMention2 {
			t.Error("room2 should have mention")
		}
	})
}
