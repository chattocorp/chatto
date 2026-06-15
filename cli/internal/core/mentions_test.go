package core

import (
	"errors"
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
		{
			name:     "mention does not cross emphasis boundary",
			body:     "@al*ice*",
			expected: []string{"al"},
		},
		{
			name:     "mention does not start across emphasis boundary",
			body:     "@*alice*",
			expected: nil,
		},
		{
			name:     "underscore in mention handle survives adjacent text nodes",
			body:     "@user_name",
			expected: []string{"user_name"},
		},
		{
			name:     "inline code mention ignored",
			body:     "`@alice` @bob",
			expected: []string{"bob"},
		},
		{
			name:     "escaped backticks still form inline code",
			body:     "\\`@alice\\` @bob",
			expected: []string{"bob"},
		},
		{
			name:     "mention immediately after inline code at start",
			body:     "`cmd`@alice",
			expected: []string{"alice"},
		},
		{
			name:     "mention immediately after escaped-backtick inline code",
			body:     "\\`cmd\\`@alice",
			expected: []string{"alice"},
		},
		{
			name:     "mention immediately after inline code after prior text",
			body:     "see`cmd`@alice",
			expected: []string{"alice"},
		},
		{
			name:     "mention immediately after inline code after prior whitespace",
			body:     "see `cmd`@alice",
			expected: []string{"alice"},
		},
		{
			name:     "fenced code mention ignored",
			body:     "```\n@all\n```\n@bob",
			expected: []string{"bob"},
		},
		{
			name:     "indented code mention ignored",
			body:     "    @alice\n@bob",
			expected: []string{"bob"},
		},
		{
			name:     "blockquote mention ignored",
			body:     "> @alice said hi\n\n@bob replied",
			expected: []string{"bob"},
		},
		{
			name:     "outside mentions around excluded regions preserve order",
			body:     "@alice `@bob` @charlie\n> @dora\n```\n@erin\n```\n@frank",
			expected: []string{"alice", "charlie", "frank"},
		},
		{
			name:     "unmatched backtick does not suppress mention",
			body:     "` @alice",
			expected: []string{"alice"},
		},
		{
			name:     "literal html code tag is plain markdown text",
			body:     "<code>@alice</code>",
			expected: []string{"alice"},
		},
		{
			name:     "backslash before mention remains mention boundary",
			body:     "\\@alice",
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

func TestChattoCore_ResolveRoomMentions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	owner, err := core.CreateUser(ctx, "system", "owneruser", "Owner User", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	alice, err := core.CreateUser(ctx, "system", "alice-room", "Alice", "password123")
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	bob, err := core.CreateUser(ctx, "system", "bob-room", "Bob", "password123")
	if err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	carol, err := core.CreateUser(ctx, "system", "carol-room", "Carol", "password123")
	if err != nil {
		t.Fatalf("CreateUser carol: %v", err)
	}
	outsider, err := core.CreateUser(ctx, "system", "outsider-room", "Outsider", "password123")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	roleUser, err := core.CreateUser(ctx, "system", "role-user", "Role User", "password123")
	if err != nil {
		t.Fatalf("CreateUser role user: %v", err)
	}
	adminUser, err := core.CreateUser(ctx, "system", "admin-user", "Admin User", "password123")
	if err != nil {
		t.Fatalf("CreateUser admin user: %v", err)
	}
	moderatorUser, err := core.CreateUser(ctx, "system", "moderator-user", "Moderator User", "password123")
	if err != nil {
		t.Fatalf("CreateUser moderator user: %v", err)
	}

	room, err := core.CreateRoom(ctx, owner.Id, KindChannel, "", "mentions-room", "Mentions")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{owner.Id, alice.Id, bob.Id, carol.Id, roleUser.Id, adminUser.Id, moderatorUser.Id} {
		if _, err := core.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}

	if err := core.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole owner: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, adminUser.Id, RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole adminUser: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, moderatorUser.Id, RoleModerator); err != nil {
		t.Fatalf("AssignServerRole moderatorUser: %v", err)
	}
	if _, err := core.CreateServerRole(ctx, SystemActorID, "support", "Support", "Support team", true); err != nil {
		t.Fatalf("CreateServerRole: %v", err)
	}
	if _, err := core.CreateServerRole(ctx, SystemActorID, "quiet", "Quiet", "Quiet team"); err != nil {
		t.Fatalf("CreateServerRole quiet: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, roleUser.Id, "support"); err != nil {
		t.Fatalf("AssignServerRole roleUser: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, outsider.Id, "support"); err != nil {
		t.Fatalf("AssignServerRole outsider: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, roleUser.Id, "quiet"); err != nil {
		t.Fatalf("AssignServerRole quiet roleUser: %v", err)
	}
	if err := core.SetPresence(ctx, alice.Id, PresenceStatusOnline); err != nil {
		t.Fatalf("SetPresence alice: %v", err)
	}
	if err := core.SetPresence(ctx, bob.Id, PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence bob: %v", err)
	}

	t.Run("direct users must be room members", func(t *testing.T) {
		got, err := core.ResolveRoomMentions(ctx, KindChannel, room.Id, []string{"alice-room", "outsider-room"})
		if err != nil {
			t.Fatalf("ResolveRoomMentions: %v", err)
		}
		if len(got) != 1 || got[0] != alice.Id {
			t.Fatalf("ResolveRoomMentions direct users = %v, want [%s]", got, alice.Id)
		}
	})

	t.Run("role mentions intersect role users with room members", func(t *testing.T) {
		got, err := core.ResolveRoomMentions(ctx, KindChannel, room.Id, []string{"support"})
		if err != nil {
			t.Fatalf("ResolveRoomMentions role: %v", err)
		}
		requireUserIDs(t, got, roleUser.Id)
	})

	t.Run("non-pingable role mentions are ignored", func(t *testing.T) {
		got, err := core.ResolveRoomMentions(ctx, KindChannel, room.Id, []string{"quiet"})
		if err != nil {
			t.Fatalf("ResolveRoomMentions quiet role: %v", err)
		}
		requireUserIDs(t, got)
	})

	t.Run("system role mentions require explicit pingability and assignment", func(t *testing.T) {
		ownerMention, err := core.ResolveRoomMentions(ctx, KindChannel, room.Id, []string{RoleOwner})
		if err != nil {
			t.Fatalf("ResolveRoomMentions owner: %v", err)
		}
		requireUserIDs(t, ownerMention)

		adminMention, err := core.ResolveRoomMentions(ctx, KindChannel, room.Id, []string{RoleAdmin})
		if err != nil {
			t.Fatalf("ResolveRoomMentions admin: %v", err)
		}
		requireUserIDs(t, adminMention)

		moderatorMention, err := core.ResolveRoomMentions(ctx, KindChannel, room.Id, []string{RoleModerator})
		if err != nil {
			t.Fatalf("ResolveRoomMentions moderator: %v", err)
		}
		requireUserIDs(t, moderatorMention, moderatorUser.Id)
	})

	t.Run("all and here expand from room membership and presence", func(t *testing.T) {
		all, err := core.ResolveRoomMentions(ctx, KindChannel, room.Id, []string{"all"})
		if err != nil {
			t.Fatalf("ResolveRoomMentions all: %v", err)
		}
		requireUserIDs(t, all, owner.Id, alice.Id, bob.Id, carol.Id, roleUser.Id, adminUser.Id, moderatorUser.Id)

		here, err := core.ResolveRoomMentions(ctx, KindChannel, room.Id, []string{"here"})
		if err != nil {
			t.Fatalf("ResolveRoomMentions here: %v", err)
		}
		requireUserIDs(t, here, alice.Id, bob.Id)
	})
}

func requireUserIDs(t *testing.T, got []string, want ...string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got user IDs %v, want %v", got, want)
	}

	seen := make(map[string]struct{}, len(got))
	for _, userID := range got {
		seen[userID] = struct{}{}
	}
	for _, userID := range want {
		if _, ok := seen[userID]; !ok {
			t.Fatalf("got user IDs %v, want %v", got, want)
		}
	}
}

func TestChattoCore_LargeMentionConfirmation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	author, err := core.CreateUser(ctx, "system", "large-author", "Large Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	room, err := core.CreateRoom(ctx, author.Id, KindChannel, "", "large-mentions", "Large Mentions")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	for i := 0; i < LargeMentionNotificationThreshold+1; i++ {
		user, err := core.CreateUser(ctx, "system", "large-target-"+string(rune('a'+i)), "Target", "password123")
		if err != nil {
			t.Fatalf("CreateUser target %d: %v", i, err)
		}
		if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom target %d: %v", i, err)
		}
	}

	if _, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "@all important", nil, "", "", nil, false); err == nil {
		t.Fatal("PostMessage succeeded without confirmation, want confirmation error")
	} else {
		var confirmErr *MentionConfirmationRequiredError
		if !errors.As(err, &confirmErr) {
			t.Fatalf("PostMessage err = %v, want MentionConfirmationRequiredError", err)
		}
		if confirmErr.RecipientCount != LargeMentionNotificationThreshold+1 {
			t.Fatalf("RecipientCount = %d, want %d", confirmErr.RecipientCount, LargeMentionNotificationThreshold+1)
		}
	}

	if _, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "```\n@all\n```", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage with @all inside fenced code block: %v", err)
	}

	if _, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "@all confirmed", nil, "", "", nil, false, WithLargeMentionConfirmed()); err != nil {
		t.Fatalf("PostMessage with confirmation: %v", err)
	}
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
	if _, err := core.JoinRoom(ctx, mentioned.Id, KindChannel, mentioned.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom mentioned: %v", err)
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
}

func TestChattoCore_MentionInsideMarkdownCodeDoesNotNotify(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	mentioned, err := core.CreateUser(ctx, "system", "code-mentioned", "Code Mentioned", "password123")
	if err != nil {
		t.Fatalf("CreateUser mentioned: %v", err)
	}
	mentioner, err := core.CreateUser(ctx, "system", "code-mentionauthor", "Code Mention Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser mentioner: %v", err)
	}
	room, err := core.CreateRoom(ctx, mentioned.Id, KindChannel, "", "code-mentions", "Code Mentions")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, mentioner.Id, KindChannel, mentioner.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom mentioner: %v", err)
	}
	if _, err := core.JoinRoom(ctx, mentioned.Id, KindChannel, mentioned.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom mentioned: %v", err)
	}

	for _, body := range []string{"`@code-mentioned`", "\\`@code-mentioned\\`"} {
		t.Run(body, func(t *testing.T) {
			event, err := core.PostMessage(ctx, KindChannel, room.Id, mentioner.Id, body, nil, "", "", nil, false)
			if err != nil {
				t.Fatalf("PostMessage: %v", err)
			}
			if got := event.GetMessagePosted().GetMentionedUserIds(); len(got) != 0 {
				t.Fatalf("mentioned_user_ids = %v, want none", got)
			}

			notifications, err := core.GetNotifications(ctx, mentioned.Id)
			if err != nil {
				t.Fatalf("GetNotifications: %v", err)
			}
			if len(notifications) != 0 {
				t.Fatalf("expected no mention notification, got %#v", notifications)
			}
		})
	}
}

func TestChattoCore_MentionImmediatelyAfterMarkdownCodeNotifies(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	mentioned, err := core.CreateUser(ctx, "system", "post-code-mentioned", "Post Code Mentioned", "password123")
	if err != nil {
		t.Fatalf("CreateUser mentioned: %v", err)
	}
	mentioner, err := core.CreateUser(ctx, "system", "post-code-author", "Post Code Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser mentioner: %v", err)
	}
	room, err := core.CreateRoom(ctx, mentioned.Id, KindChannel, "", "post-code-mentions", "Post Code Mentions")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, mentioner.Id, KindChannel, mentioner.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom mentioner: %v", err)
	}
	if _, err := core.JoinRoom(ctx, mentioned.Id, KindChannel, mentioned.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom mentioned: %v", err)
	}

	event, err := core.PostMessage(ctx, KindChannel, room.Id, mentioner.Id, "`cmd`@post-code-mentioned", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	requireUserIDs(t, event.GetMessagePosted().GetMentionedUserIds(), mentioned.Id)

	notifications, err := core.GetNotifications(ctx, mentioned.Id)
	if err != nil {
		t.Fatalf("GetNotifications: %v", err)
	}
	if len(notifications) != 1 || notifications[0].GetMention() == nil {
		t.Fatalf("expected one mention notification, got %#v", notifications)
	}
}

func TestChattoCore_MentionSplitByMarkdownFormattingDoesNotNotify(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	alice, err := core.CreateUser(ctx, "system", "format-alice", "Format Alice", "password123")
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	author, err := core.CreateUser(ctx, "system", "format-author", "Format Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	room, err := core.CreateRoom(ctx, alice.Id, KindChannel, "", "format-mentions", "Format Mentions")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom author: %v", err)
	}
	if _, err := core.JoinRoom(ctx, alice.Id, KindChannel, alice.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom alice: %v", err)
	}

	for _, body := range []string{"@format-al*ice*", "@*format-alice*"} {
		t.Run(body, func(t *testing.T) {
			event, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, body, nil, "", "", nil, false)
			if err != nil {
				t.Fatalf("PostMessage: %v", err)
			}
			for _, mentionedUserID := range event.GetMessagePosted().GetMentionedUserIds() {
				if mentionedUserID == alice.Id {
					t.Fatalf("mentioned_user_ids includes formatted handle target %s", alice.Id)
				}
			}

			notifications, err := core.GetNotifications(ctx, alice.Id)
			if err != nil {
				t.Fatalf("GetNotifications: %v", err)
			}
			if len(notifications) != 0 {
				t.Fatalf("expected no mention notification, got %#v", notifications)
			}
		})
	}
}
