package graph

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ptr returns a pointer to the given string value
func ptr(s string) *string {
	return &s
}

func boolPtr(v bool) *bool {
	return &v
}

// ============================================================================
// CreateRoom Authorization Tests
// ============================================================================

func TestCreateRoom_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()
	groupID := env.testRoom.GroupId

	input := model.CreateRoomInput{
		Name:    "new-room",
		GroupID: groupID,
	}

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.CreateRoom(env.unauthContext(), input)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-member is rejected with permission denied", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-create", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.CreateRoom(env.authContextForUser(outsider), input)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("space admin can create room", func(t *testing.T) {
		// testUser is the space creator (admin)
		room, err := mutation.CreateRoom(env.authContext(), model.CreateRoomInput{
			Name:    "admin-created-room",
			GroupID: groupID,
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if room == nil {
			t.Fatal("expected room, got nil")
		}
		if room.Name != "admin-created-room" {
			t.Errorf("expected name 'admin-created-room', got %s", room.Name)
		}
	})

	t.Run("regular member cannot create room by default", func(t *testing.T) {
		member, err := env.core.CreateUser(env.ctx, "system", "member-create", "Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		// room.create is not granted to everyone role by default
		_, err = mutation.CreateRoom(env.authContextForUser(member), model.CreateRoomInput{
			Name:    "member-created-room",
			GroupID: groupID,
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("member with room.create permission can create room", func(t *testing.T) {
		member, err := env.core.CreateUser(env.ctx, "system", "member-create-granted", "Member Granted", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		// Grant room.create to the everyone role
		err = env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermRoomCreate)
		if err != nil {
			t.Fatalf("failed to grant permission: %v", err)
		}

		room, err := mutation.CreateRoom(env.authContextForUser(member), model.CreateRoomInput{
			Name:    "member-created-room-granted",
			GroupID: groupID,
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if room == nil {
			t.Fatal("expected room, got nil")
		}
	})
}

func TestPostMessage_LargeMentionConfirmationUsesScopedToken(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	for i := 0; i < core.LargeMentionNotificationThreshold+1; i++ {
		user, err := env.core.CreateUser(env.ctx, "system", "large-mention-target-"+string(rune('a'+i)), "Target", "password123")
		if err != nil {
			t.Fatalf("CreateUser target %d: %v", i, err)
		}
		if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, env.testRoom.Id); err != nil {
			t.Fatalf("JoinRoom target %d: %v", i, err)
		}
		if err := env.core.SetPresence(env.ctx, user.Id, core.PresenceStatusOnline); err != nil {
			t.Fatalf("SetPresence target %d: %v", i, err)
		}
	}

	_, err := mutation.PostMessage(env.authContext(), model.PostMessageInput{
		RoomID: env.testRoom.Id,
		Body:   ptr("@here token"),
	})
	if err == nil {
		t.Fatal("PostMessage without confirmation succeeded, want confirmation error")
	}
	var gqlErr *gqlerror.Error
	if !errors.As(err, &gqlErr) {
		t.Fatalf("PostMessage confirmation err = %T %v, want gqlerror.Error", err, err)
	}
	if gqlErr.Extensions["code"] != "MENTION_CONFIRMATION_REQUIRED" {
		t.Fatalf("confirmation error code = %v", gqlErr.Extensions["code"])
	}
	if gqlErr.Extensions["recipientCount"] != core.LargeMentionNotificationThreshold+1 {
		t.Fatalf("recipientCount = %v, want %d", gqlErr.Extensions["recipientCount"], core.LargeMentionNotificationThreshold+1)
	}
	token, ok := gqlErr.Extensions["mentionConfirmationToken"].(string)
	if !ok || token == "" {
		t.Fatalf("mentionConfirmationToken = %v, want non-empty string", gqlErr.Extensions["mentionConfirmationToken"])
	}

	_, err = mutation.PostMessage(env.authContext(), model.PostMessageInput{
		RoomID:                   env.testRoom.Id,
		Body:                     ptr("@here changed"),
		MentionConfirmationToken: &token,
	})
	if err == nil {
		t.Fatal("PostMessage with mismatched token scope succeeded, want confirmation error")
	}
	if !errors.As(err, &gqlErr) {
		t.Fatalf("PostMessage mismatched token err = %T %v, want gqlerror.Error", err, err)
	}
	if gqlErr.Extensions["code"] != "MENTION_CONFIRMATION_REQUIRED" {
		t.Fatalf("mismatched token error code = %v", gqlErr.Extensions["code"])
	}

	driftUser, err := env.core.CreateUser(env.ctx, "system", "large-mention-drift", "Drift", "password123")
	if err != nil {
		t.Fatalf("CreateUser drift: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, driftUser.Id, core.KindChannel, driftUser.Id, env.testRoom.Id); err != nil {
		t.Fatalf("JoinRoom drift: %v", err)
	}
	if err := env.core.SetPresence(env.ctx, driftUser.Id, core.PresenceStatusOnline); err != nil {
		t.Fatalf("SetPresence drift: %v", err)
	}
	if _, err := mutation.PostMessage(env.authContext(), model.PostMessageInput{
		RoomID:                   env.testRoom.Id,
		Body:                     ptr("@here token"),
		MentionConfirmationToken: &token,
	}); err != nil {
		t.Fatalf("PostMessage with valid token after recipient drift: %v", err)
	}
}

// ============================================================================
// PostMessage Authorization Tests
// ============================================================================

func TestPostMessage_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	input := model.PostMessageInput{
		RoomID: env.testRoom.Id,
		Body:   ptr("Hello, world!"),
	}

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.PostMessage(env.unauthContext(), input)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-space-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-post", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.PostMessage(env.authContextForUser(outsider), input)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("space member but not room member is rejected", func(t *testing.T) {
		spaceMember, err := env.core.CreateUser(env.ctx, "system", "spacemember-post", "Space Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		// Note: not joining the room

		_, err = mutation.PostMessage(env.authContextForUser(spaceMember), input)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("room member can post message", func(t *testing.T) {
		// testUser is a room member
		event, err := mutation.PostMessage(env.authContext(), input)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
	})
}

func TestPostMessage_VideoUploadsDisabled(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	before, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount before post: %v", err)
	}

	_, err = mutation.PostMessage(env.authContext(), model.PostMessageInput{
		RoomID: env.testRoom.Id,
		Attachments: []*graphql.Upload{{
			File:        bytes.NewReader([]byte("not a real video, but enough to reject by MIME type")),
			Filename:    "clip.mp4",
			Size:        48,
			ContentType: "video/mp4",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "video uploads are disabled") {
		t.Fatalf("PostMessage video while disabled error = %v, want disabled-video error", err)
	}

	after, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount after post: %v", err)
	}
	if after != before {
		t.Fatalf("asset count after rejected video = %d, want unchanged %d", after, before)
	}

	_, err = mutation.PostMessage(env.authContext(), model.PostMessageInput{
		RoomID: env.testRoom.Id,
		Attachments: []*graphql.Upload{
			{
				File:        bytes.NewReader([]byte("hello")),
				Filename:    "notes.txt",
				Size:        5,
				ContentType: "text/plain",
			},
			{
				File:        bytes.NewReader([]byte("video")),
				Filename:    "clip.mp4",
				Size:        5,
				ContentType: "video/mp4",
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "video uploads are disabled") {
		t.Fatalf("PostMessage mixed upload while disabled error = %v, want disabled-video error", err)
	}

	afterMixed, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount after mixed post: %v", err)
	}
	if afterMixed != before {
		t.Fatalf("asset count after rejected mixed upload = %d, want unchanged %d", afterMixed, before)
	}
}

func TestPostMessage_AllowsNonVideoUploadsWhenVideoProcessingDisabled(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	tests := []struct {
		name        string
		filename    string
		contentType string
		data        []byte
	}{
		{
			name:        "animated gif stays allowed as image upload",
			filename:    "tiny.gif",
			contentType: "image/gif",
			data: []byte{
				'G', 'I', 'F', '8', '9', 'a', 1, 0, 1, 0, 0x80, 0, 0,
				0, 0, 0, 0xff, 0xff, 0xff, 0x2c, 0, 0, 0, 0, 1, 0, 1, 0,
				0, 2, 2, 0x44, 1, 0, 0x3b,
			},
		},
		{
			name:        "audio",
			filename:    "tone.mp3",
			contentType: "audio/mpeg",
			data:        []byte("audio bytes"),
		},
		{
			name:        "generic file",
			filename:    "notes.txt",
			contentType: "text/plain",
			data:        []byte("hello"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := mutation.PostMessage(env.authContext(), model.PostMessageInput{
				RoomID: env.testRoom.Id,
				Attachments: []*graphql.Upload{{
					File:        bytes.NewReader(tt.data),
					Filename:    tt.filename,
					Size:        int64(len(tt.data)),
					ContentType: tt.contentType,
				}},
			})
			if err != nil {
				t.Fatalf("PostMessage returned error: %v", err)
			}
			if event == nil {
				t.Fatal("PostMessage returned nil event")
			}
		})
	}
}

// ============================================================================
// Permission-based moderation: edit-any / delete-any are gated by message.manage
// ============================================================================

func TestMessageModeration_PermissionOnly(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Build owner (testUser, default), admin, moderator, and regular members.
	// All are members of the seeded test room.
	admin := env.createVerifiedUser(t, "perm-mod-admin", "Admin", "password123")
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, admin.Id, core.RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole admin: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, admin.Id, core.KindChannel, admin.Id, env.testRoom.Id); err != nil {
		t.Fatalf("JoinRoom admin: %v", err)
	}

	moderator := env.createVerifiedUser(t, "perm-mod-moderator", "Moderator", "password123")
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, moderator.Id, core.RoleModerator); err != nil {
		t.Fatalf("AssignServerRole moderator: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, moderator.Id, core.KindChannel, moderator.Id, env.testRoom.Id); err != nil {
		t.Fatalf("JoinRoom moderator: %v", err)
	}

	regular := env.createVerifiedUser(t, "perm-mod-regular", "Regular", "password123")
	if _, err := env.core.JoinRoom(env.ctx, regular.Id, core.KindChannel, regular.Id, env.testRoom.Id); err != nil {
		t.Fatalf("JoinRoom regular: %v", err)
	}

	// Helper: post a message as `author` in the test room, return its event ID.
	post := func(author *corev1.User, body string) string {
		t.Helper()
		root, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, author.Id, body, nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage by %s: %v", author.Login, err)
		}
		return root.Id
	}

	t.Run("moderator can delete owner's message", func(t *testing.T) {
		eventID := post(env.testUser, "owner's message")
		if _, err := mutation.DeleteMessage(env.authContextForUser(moderator), model.DeleteMessageInput{
			RoomID:  env.testRoom.Id,
			EventID: eventID,
		}); err != nil {
			t.Errorf("expected moderator to delete owner's message, got %v", err)
		}
	})

	t.Run("moderator can edit owner's message", func(t *testing.T) {
		eventID := post(env.testUser, "another owner message")
		if _, err := mutation.UpdateMessage(env.authContextForUser(moderator), model.UpdateMessageInput{
			RoomID:  env.testRoom.Id,
			EventID: eventID,
			Body:    "tampered",
		}); err != nil {
			t.Errorf("expected moderator to edit owner's message, got %v", err)
		}
	})

	t.Run("moderator can delete admin's message", func(t *testing.T) {
		eventID := post(admin, "admin's message")
		if _, err := mutation.DeleteMessage(env.authContextForUser(moderator), model.DeleteMessageInput{
			RoomID:  env.testRoom.Id,
			EventID: eventID,
		}); err != nil {
			t.Errorf("expected moderator to delete admin's message, got %v", err)
		}
	})

	t.Run("moderator CAN delete a regular member's message", func(t *testing.T) {
		eventID := post(regular, "regular's message")
		if _, err := mutation.DeleteMessage(env.authContextForUser(moderator), model.DeleteMessageInput{
			RoomID:  env.testRoom.Id,
			EventID: eventID,
		}); err != nil {
			t.Errorf("expected moderator to be able to delete a regular user's message, got %v", err)
		}
	})

	t.Run("moderator CAN edit a regular member's message", func(t *testing.T) {
		eventID := post(regular, "regular's editable message")
		if _, err := mutation.UpdateMessage(env.authContextForUser(moderator), model.UpdateMessageInput{
			RoomID:  env.testRoom.Id,
			EventID: eventID,
			Body:    "edited by moderator",
		}); err != nil {
			t.Errorf("expected moderator to be able to edit a regular user's message, got %v", err)
		}
	})

	t.Run("admin can delete owner's message", func(t *testing.T) {
		eventID := post(env.testUser, "owner msg vs admin")
		if _, err := mutation.DeleteMessage(env.authContextForUser(admin), model.DeleteMessageInput{
			RoomID:  env.testRoom.Id,
			EventID: eventID,
		}); err != nil {
			t.Errorf("expected admin to delete owner's message, got %v", err)
		}
	})

	t.Run("admin CAN delete moderator's message", func(t *testing.T) {
		eventID := post(moderator, "moderator's message")
		if _, err := mutation.DeleteMessage(env.authContextForUser(admin), model.DeleteMessageInput{
			RoomID:  env.testRoom.Id,
			EventID: eventID,
		}); err != nil {
			t.Errorf("expected admin to be able to delete moderator's message, got %v", err)
		}
	})

	t.Run("owner can delete moderator's message", func(t *testing.T) {
		eventID := post(moderator, "another moderator message")
		if _, err := mutation.DeleteMessage(env.authContext(), model.DeleteMessageInput{
			RoomID:  env.testRoom.Id,
			EventID: eventID,
		}); err != nil {
			t.Errorf("expected owner to be able to delete moderator's message, got %v", err)
		}
	})

	t.Run("author can always delete own message", func(t *testing.T) {
		eventID := post(regular, "regular self-delete")
		if _, err := mutation.DeleteMessage(env.authContextForUser(regular), model.DeleteMessageInput{
			RoomID:  env.testRoom.Id,
			EventID: eventID,
		}); err != nil {
			t.Errorf("expected author to delete own message, got %v", err)
		}
	})
}

// ============================================================================
// PostMessage Thread Permission Tests
// ============================================================================

func TestPostMessage_ThreadPermissions(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Create a regular member (not admin/owner — affected by "everyone" role denials)
	member, err := env.core.CreateUser(env.ctx, "system", "thread-member", "Thread Member", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	_, err = env.core.JoinRoom(env.ctx, member.Id, core.KindChannel, member.Id, env.testRoom.Id)
	if err != nil {
		t.Fatalf("failed to join room: %v", err)
	}

	t.Run("member can post first thread reply with default permissions", func(t *testing.T) {
		root, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Root for thread test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post root: %v", err)
		}

		event, err := mutation.PostMessage(env.authContextForUser(member), model.PostMessageInput{
			RoomID:            env.testRoom.Id,
			Body:              ptr("Starting a thread"),
			ThreadRootEventID: ptr(root.Id),
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
	})

	t.Run("member with post-in-thread denied cannot post any thread reply", func(t *testing.T) {
		groupID := env.testRoom.GroupId
		if err := env.core.DenyGroupPermission(env.ctx, core.SystemActorID, groupID, core.RoleEveryone, core.PermMessagePostInThread); err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}
		defer env.core.GrantGroupPermission(env.ctx, core.SystemActorID, groupID, core.RoleEveryone, core.PermMessagePostInThread)

		root, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Root for deny test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post root: %v", err)
		}

		// First reply (no existing replies) — still requires post-in-thread
		_, err = mutation.PostMessage(env.authContextForUser(member), model.PostMessageInput{
			RoomID:            env.testRoom.Id,
			Body:              ptr("Trying to start thread"),
			ThreadRootEventID: ptr(root.Id),
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied for first reply, got %v", err)
		}

		// Subsequent reply (existing replies) — also denied
		_, err = env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "First reply by owner", nil, root.Id, "", nil, false)
		if err != nil {
			t.Fatalf("failed to create thread: %v", err)
		}

		_, err = mutation.PostMessage(env.authContextForUser(member), model.PostMessageInput{
			RoomID:            env.testRoom.Id,
			Body:              ptr("Trying to post in existing thread"),
			ThreadRootEventID: ptr(root.Id),
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied for subsequent reply, got %v", err)
		}
	})

	t.Run("denying message.post does not affect thread replies", func(t *testing.T) {
		if err := env.core.DenyServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermMessagePost); err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}
		defer env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermMessagePost)

		root, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Root for independence test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post root: %v", err)
		}

		event, err := mutation.PostMessage(env.authContextForUser(member), model.PostMessageInput{
			RoomID:            env.testRoom.Id,
			Body:              ptr("Thread reply still works"),
			ThreadRootEventID: ptr(root.Id),
		})
		if err != nil {
			t.Fatalf("expected success for thread reply, got error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
	})
}

// ============================================================================
// UpdateSpace Authorization Tests
// ============================================================================

func TestUpdateServerConfig_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	newName := "Updated Instance Name"

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.UpdateServerConfig(env.unauthContext(), model.UpdateServerConfigInput{ServerName: &newName})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-member is rejected with permission denied", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-update", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.UpdateServerConfig(env.authContextForUser(outsider), model.UpdateServerConfigInput{ServerName: &newName})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("regular member is rejected with permission denied", func(t *testing.T) {
		member, err := env.core.CreateUser(env.ctx, "system", "member-update", "Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.UpdateServerConfig(env.authContextForUser(member), model.UpdateServerConfigInput{ServerName: &newName})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("admin can update instance", func(t *testing.T) {
		// testUser is the space creator (admin)
		config, err := mutation.UpdateServerConfig(env.authContext(), model.UpdateServerConfigInput{ServerName: &newName})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if config == nil {
			t.Fatal("expected config, got nil")
		}
		if config.Name != newName {
			t.Fatalf("expected returned server name %q, got %q", newName, config.Name)
		}
		// Verify the canonical server name (stored in ServerConfig) was updated.
		gotName, err := env.core.ConfigManager().GetEffectiveServerName(env.ctx)
		if err != nil {
			t.Fatalf("failed to read server name from config: %v", err)
		}
		if gotName != newName {
			t.Errorf("expected server name %q, got %q", newName, gotName)
		}
	})
}

// ============================================================================
// JoinRoom Authorization Tests
// ============================================================================

func TestJoinRoom_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Create a new room for join tests
	newRoom, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "", "join-test-room", "Room for join tests")
	if err != nil {
		t.Fatalf("failed to create room: %v", err)
	}

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err = mutation.JoinRoom(env.unauthContext(), model.JoinRoomInput{RoomID: newRoom.Id})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("space member can join room (default member permission)", func(t *testing.T) {
		member, err := env.core.CreateUser(env.ctx, "system", "member-join", "Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		room, err := mutation.JoinRoom(env.authContextForUser(member), model.JoinRoomInput{RoomID: newRoom.Id})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if room == nil || room.Id != newRoom.Id {
			t.Errorf("expected joinRoom to return the joined room, got %+v", room)
		}

		// Verify membership
		exists, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, member.Id, newRoom.Id)
		if err != nil {
			t.Fatalf("failed to check membership: %v", err)
		}
		if !exists {
			t.Error("expected user to be room member")
		}
	})
}

// ============================================================================
// LeaveRoom Authorization Tests
// ============================================================================

func TestLeaveRoom_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.LeaveRoom(env.unauthContext(), model.LeaveRoomInput{RoomID: env.testRoom.Id})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("room member can leave room", func(t *testing.T) {
		member, err := env.core.CreateUser(env.ctx, "system", "room-leaver", "Room Leaver", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, member.Id, core.KindChannel, member.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		success, err := mutation.LeaveRoom(env.authContextForUser(member), model.LeaveRoomInput{RoomID: env.testRoom.Id})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected success=true")
		}

		// Verify no longer a member
		exists, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, member.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to check membership: %v", err)
		}
		if exists {
			t.Error("expected user to not be room member")
		}
	})
}

// ============================================================================
// BanRoomMember Authorization Tests
// ============================================================================

func TestBanRoomMember_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.BanRoomMember(env.unauthContext(), model.BanRoomMemberInput{
			RoomID: env.testRoom.Id,
			UserID: env.testUser.Id,
			Reason: "test",
		})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("member remover can remove member from channel room", func(t *testing.T) {
		admin, err := env.core.CreateUser(env.ctx, "system", "room-remover", "Room Remover", "password123")
		if err != nil {
			t.Fatalf("failed to create admin: %v", err)
		}
		target, err := env.core.CreateUser(env.ctx, "system", "remove-target", "Remove Target", "password123")
		if err != nil {
			t.Fatalf("failed to create target: %v", err)
		}
		if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, admin.Id, core.RoleAdmin); err != nil {
			t.Fatalf("AssignServerRole admin: %v", err)
		}
		if _, err := env.core.JoinRoom(env.ctx, admin.Id, core.KindChannel, admin.Id, env.testRoom.Id); err != nil {
			t.Fatalf("JoinRoom admin: %v", err)
		}
		if _, err := env.core.JoinRoom(env.ctx, target.Id, core.KindChannel, target.Id, env.testRoom.Id); err != nil {
			t.Fatalf("JoinRoom target: %v", err)
		}

		success, err := mutation.BanRoomMember(env.authContextForUser(admin), model.BanRoomMemberInput{
			RoomID: env.testRoom.Id,
			UserID: target.Id,
			Reason: "spam",
		})
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if !success {
			t.Fatal("expected success=true")
		}

		exists, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, target.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("RoomMembershipExists: %v", err)
		}
		if exists {
			t.Fatal("expected target membership to be removed")
		}

		_, err = mutation.PostMessage(env.authContextForUser(target), model.PostMessageInput{
			RoomID: env.testRoom.Id,
			Body:   ptr("I should not post"),
		})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Fatalf("expected removed target to lose post access, got %v", err)
		}

		result, err := env.resolver.Room().Events(env.authContextForUser(target), env.testRoom, nil, nil, nil)
		if result != nil {
			t.Fatalf("expected removed target to lose room read access, got %+v", result)
		}
		if !errors.Is(err, core.ErrNotRoomMember) {
			t.Fatalf("expected removed target to lose room read access, got %v", err)
		}

		ban, ok := env.core.RoomBans.ActiveBan(env.testRoom.Id, target.Id, time.Now())
		if !ok {
			t.Fatal("expected target to have an active room ban")
		}
		if ban.ModeratorID != admin.Id || ban.Reason != "spam" {
			t.Fatalf("unexpected ban projection: moderator=%q reason=%q", ban.ModeratorID, ban.Reason)
		}

		events, err := env.resolver.Room().Events(env.authContextForUser(admin), env.testRoom, nil, nil, nil)
		if err != nil {
			t.Fatalf("expected admin to read room events after ban, got %v", err)
		}
		if len(events.Events) == 0 {
			t.Fatal("expected room events after ban")
		}
		latest := events.Events[len(events.Events)-1]
		if latest.ActorID() != target.Id {
			t.Fatalf("expected public leave actor to be banned target, got %q", latest.ActorID())
		}
		if _, ok := latest.Payload().(*corev1.Event_UserLeftRoom); !ok {
			t.Fatalf("expected public ban history event to be UserLeftRoomEvent, got %T", latest.Payload())
		}

		_, err = mutation.JoinRoom(env.authContextForUser(target), model.JoinRoomInput{RoomID: env.testRoom.Id})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Fatalf("expected banned target to be blocked by generic join authorization, got %v", err)
		}

		_, err = mutation.UnbanRoomMember(env.authContextForUser(admin), model.UnbanRoomMemberInput{
			RoomID: env.testRoom.Id,
			UserID: target.Id,
			Reason: "",
		})
		if err == nil {
			t.Fatal("expected blank unban reason to be rejected")
		}

		success, err = mutation.UnbanRoomMember(env.authContextForUser(admin), model.UnbanRoomMemberInput{
			RoomID: env.testRoom.Id,
			UserID: target.Id,
			Reason: "appeal accepted",
		})
		if err != nil {
			t.Fatalf("expected unban success, got %v", err)
		}
		if !success {
			t.Fatal("expected unban success=true")
		}
		if _, ok := env.core.RoomBans.ActiveBan(env.testRoom.Id, target.Id, time.Now()); ok {
			t.Fatal("expected target room ban to be cleared")
		}

		if _, err = mutation.JoinRoom(env.authContextForUser(target), model.JoinRoomInput{RoomID: env.testRoom.Id}); err != nil {
			t.Fatalf("expected unbanned target to be able to join again, got %v", err)
		}
	})

	t.Run("non-member target is rejected without public leave event", func(t *testing.T) {
		admin, err := env.core.CreateUser(env.ctx, "system", "room-remover-nonmember", "Room Remover Nonmember", "password123")
		if err != nil {
			t.Fatalf("failed to create admin: %v", err)
		}
		target, err := env.core.CreateUser(env.ctx, "system", "remove-target-nonmember", "Remove Target Nonmember", "password123")
		if err != nil {
			t.Fatalf("failed to create target: %v", err)
		}
		if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, admin.Id, core.RoleAdmin); err != nil {
			t.Fatalf("AssignServerRole admin: %v", err)
		}
		if _, err := env.core.JoinRoom(env.ctx, admin.Id, core.KindChannel, admin.Id, env.testRoom.Id); err != nil {
			t.Fatalf("JoinRoom admin: %v", err)
		}

		before, err := env.resolver.Room().Events(env.authContextForUser(admin), env.testRoom, nil, nil, nil)
		if err != nil {
			t.Fatalf("expected admin to read room events before ban attempt, got %v", err)
		}

		_, err = mutation.BanRoomMember(env.authContextForUser(admin), model.BanRoomMemberInput{
			RoomID: env.testRoom.Id,
			UserID: target.Id,
			Reason: "spam",
		})
		if !errors.Is(err, core.ErrNotRoomMember) {
			t.Fatalf("expected ErrNotRoomMember for non-member target, got %v", err)
		}
		if _, ok := env.core.RoomBans.ActiveBan(env.testRoom.Id, target.Id, time.Now()); ok {
			t.Fatal("expected non-member target not to receive an active room ban")
		}

		after, err := env.resolver.Room().Events(env.authContextForUser(admin), env.testRoom, nil, nil, nil)
		if err != nil {
			t.Fatalf("expected admin to read room events after ban attempt, got %v", err)
		}
		if len(after.Events) != len(before.Events) {
			t.Fatalf("expected no public leave event for non-member target, before=%d after=%d", len(before.Events), len(after.Events))
		}
	})

	t.Run("caller without room.ban-member is rejected", func(t *testing.T) {
		caller, err := env.core.CreateUser(env.ctx, "system", "remove-no-perm", "No Perm", "password123")
		if err != nil {
			t.Fatalf("failed to create caller: %v", err)
		}
		target, err := env.core.CreateUser(env.ctx, "system", "remove-no-perm-target", "No Perm Target", "password123")
		if err != nil {
			t.Fatalf("failed to create target: %v", err)
		}
		if _, err := env.core.JoinRoom(env.ctx, caller.Id, core.KindChannel, caller.Id, env.testRoom.Id); err != nil {
			t.Fatalf("JoinRoom caller: %v", err)
		}
		if _, err := env.core.JoinRoom(env.ctx, target.Id, core.KindChannel, target.Id, env.testRoom.Id); err != nil {
			t.Fatalf("JoinRoom target: %v", err)
		}

		_, err = mutation.BanRoomMember(env.authContextForUser(caller), model.BanRoomMemberInput{
			RoomID: env.testRoom.Id,
			UserID: target.Id,
			Reason: "test",
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Fatalf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("caller with room.manage but without room.ban-member is rejected", func(t *testing.T) {
		caller, err := env.core.CreateUser(env.ctx, "system", "remove-manage-only", "Manage Only", "password123")
		if err != nil {
			t.Fatalf("failed to create caller: %v", err)
		}
		target, err := env.core.CreateUser(env.ctx, "system", "remove-manage-only-target", "Manage Only Target", "password123")
		if err != nil {
			t.Fatalf("failed to create target: %v", err)
		}
		if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, caller.Id, core.RoleAdmin); err != nil {
			t.Fatalf("AssignServerRole admin: %v", err)
		}
		if _, err := env.core.JoinRoom(env.ctx, caller.Id, core.KindChannel, caller.Id, env.testRoom.Id); err != nil {
			t.Fatalf("JoinRoom caller: %v", err)
		}
		if _, err := env.core.JoinRoom(env.ctx, target.Id, core.KindChannel, target.Id, env.testRoom.Id); err != nil {
			t.Fatalf("JoinRoom target: %v", err)
		}
		if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, env.testRoom.Id, core.RoleAdmin, core.PermRoomMemberBan); err != nil {
			t.Fatalf("DenyRoomPermission room.ban-member: %v", err)
		}
		if err := env.core.GrantRoomPermission(env.ctx, core.SystemActorID, env.testRoom.Id, core.RoleAdmin, core.PermRoomManage); err != nil {
			t.Fatalf("GrantRoomPermission room.manage: %v", err)
		}

		_, err = mutation.BanRoomMember(env.authContextForUser(caller), model.BanRoomMemberInput{
			RoomID: env.testRoom.Id,
			UserID: target.Id,
			Reason: "test",
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Fatalf("expected ErrPermissionDenied for manage-only caller, got %v", err)
		}
	})

	t.Run("caller with room.ban-member can ban peer", func(t *testing.T) {
		moderatorA, err := env.core.CreateUser(env.ctx, "system", "remove-mod-a", "Mod A", "password123")
		if err != nil {
			t.Fatalf("failed to create moderator A: %v", err)
		}
		moderatorB, err := env.core.CreateUser(env.ctx, "system", "remove-mod-b", "Mod B", "password123")
		if err != nil {
			t.Fatalf("failed to create moderator B: %v", err)
		}
		for _, user := range []*corev1.User{moderatorA, moderatorB} {
			if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, user.Id, core.RoleModerator); err != nil {
				t.Fatalf("AssignServerRole moderator: %v", err)
			}
			if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, env.testRoom.Id); err != nil {
				t.Fatalf("JoinRoom moderator: %v", err)
			}
		}
		if err := env.core.GrantRoomPermission(env.ctx, core.SystemActorID, env.testRoom.Id, core.RoleModerator, core.PermRoomMemberBan); err != nil {
			t.Fatalf("GrantRoomPermission room.ban-member: %v", err)
		}

		_, err = mutation.BanRoomMember(env.authContextForUser(moderatorA), model.BanRoomMemberInput{
			RoomID: env.testRoom.Id,
			UserID: moderatorB.Id,
			Reason: "test",
		})
		if err != nil {
			t.Fatalf("expected peer ban to succeed with room.ban-member, got %v", err)
		}
	})

	t.Run("DM room members cannot be banned", func(t *testing.T) {
		target, err := env.core.CreateUser(env.ctx, "system", "dm-remove-target", "DM Target", "password123")
		if err != nil {
			t.Fatalf("failed to create target: %v", err)
		}
		dmRoom, _, err := env.core.FindOrCreateDM(env.ctx, env.testUser.Id, []string{target.Id})
		if err != nil {
			t.Fatalf("FindOrCreateDM: %v", err)
		}

		_, err = mutation.BanRoomMember(env.authContext(), model.BanRoomMemberInput{
			RoomID: dmRoom.Id,
			UserID: target.Id,
			Reason: "test",
		})
		if !errors.Is(err, core.ErrCannotBanDMRoomMember) {
			t.Fatalf("expected ErrCannotBanDMRoomMember, got %v", err)
		}

		exists, err := env.core.RoomMembershipExists(env.ctx, core.KindDM, target.Id, dmRoom.Id)
		if err != nil {
			t.Fatalf("RoomMembershipExists: %v", err)
		}
		if !exists {
			t.Fatal("expected DM membership to remain")
		}
	})
}

// ============================================================================
// AddReaction Authorization Tests
// ============================================================================

func TestAddReaction_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Post a message to react to
	event, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Test message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post message: %v", err)
	}
	messageEventID := event.Id

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.AddReaction(env.unauthContext(), model.AddReactionInput{RoomID: env.testRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-room-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-react", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.AddReaction(env.authContextForUser(outsider), model.AddReactionInput{RoomID: env.testRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("space member but not room member is rejected", func(t *testing.T) {
		spaceMember, err := env.core.CreateUser(env.ctx, "system", "spacemember-react", "Space Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.AddReaction(env.authContextForUser(spaceMember), model.AddReactionInput{RoomID: env.testRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("room member can add reaction", func(t *testing.T) {
		success, err := mutation.AddReaction(env.authContext(), model.AddReactionInput{RoomID: env.testRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected success=true")
		}
	})
}

// ============================================================================
// RemoveReaction Authorization Tests
// ============================================================================

func TestRemoveReaction_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Post a message and add a reaction to remove
	event, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Test message for removal", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post message: %v", err)
	}
	messageEventID := event.Id

	// Add a reaction first
	_, err = env.core.AddReaction(env.ctx, core.KindChannel, env.testRoom.Id, messageEventID, "thumbsup", env.testUser.Id)
	if err != nil {
		t.Fatalf("failed to add reaction: %v", err)
	}

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.RemoveReaction(env.unauthContext(), model.RemoveReactionInput{RoomID: env.testRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-room-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-unreact", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.RemoveReaction(env.authContextForUser(outsider), model.RemoveReactionInput{RoomID: env.testRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("room member can remove reaction", func(t *testing.T) {
		success, err := mutation.RemoveReaction(env.authContext(), model.RemoveReactionInput{RoomID: env.testRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected success=true")
		}
	})
}

// ============================================================================
// MarkRoomAsRead Authorization Tests
// ============================================================================

func TestMarkRoomAsRead_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.MarkRoomAsRead(env.unauthContext(), model.MarkRoomAsReadInput{RoomID: env.testRoom.Id})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-room-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-read", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.MarkRoomAsRead(env.authContextForUser(outsider), model.MarkRoomAsReadInput{RoomID: env.testRoom.Id})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("room member can mark room as read", func(t *testing.T) {
		result, err := mutation.MarkRoomAsRead(env.authContext(), model.MarkRoomAsReadInput{RoomID: env.testRoom.Id})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result == nil {
			t.Error("expected result, got nil")
		}
	})
}

// ============================================================================
// MarkRoomAsRead UpToEventId Tests
// ============================================================================

func TestMarkRoomAsRead_UpToEventId(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Create a second user (the "reader") who joins the room as a member
	// but never posts — that way PostMessage's auto-advance of the poster's
	// read marker doesn't interfere with what this test wants to observe.
	reader, err := env.core.CreateUser(env.ctx, "system", "reader-uptoevent", "Reader", "password123")
	if err != nil {
		t.Fatalf("create reader: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, reader.Id, core.KindChannel, reader.Id, env.testRoom.Id); err != nil {
		t.Fatalf("add reader to room: %v", err)
	}

	// testUser posts three messages so we have explicit event IDs to anchor
	// against. The reader stays at the empty/initial marker.
	e1, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "first", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("post e1: %v", err)
	}
	// Seed the reader's marker at e1 so the test starts from a known baseline.
	if err := env.core.SetLastReadEventID(env.ctx, core.KindChannel, reader.Id, env.testRoom.Id, e1.Id); err != nil {
		t.Fatalf("seed marker at e1: %v", err)
	}
	e2, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "second", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("post e2: %v", err)
	}
	e3, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "third", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("post e3: %v", err)
	}

	readerCtx := env.authContextForUser(reader)

	t.Run("explicit upToEventId anchors marker at that event (not latest)", func(t *testing.T) {
		// Reader's marker is at e1 (seeded above). Advance to e2 — even
		// though e3 also exists. The marker should land on e2.
		result, err := mutation.MarkRoomAsRead(readerCtx, model.MarkRoomAsReadInput{
			RoomID:      env.testRoom.Id,
			UpToEventID: &e2.Id,
		})
		if err != nil {
			t.Fatalf("mark with upToEventId: %v", err)
		}
		if result.LastReadAt == nil {
			t.Fatal("expected LastReadAt to be set")
		}

		got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("get marker: %v", err)
		}
		if got != e2.Id {
			t.Errorf("expected marker = %s (e2), got %s", e2.Id, got)
		}
	})

	t.Run("advance-only: stale upToEventId does not regress marker", func(t *testing.T) {
		// Currently at e2. Try to regress to e1 — should be ignored.
		_, err := mutation.MarkRoomAsRead(readerCtx, model.MarkRoomAsReadInput{
			RoomID:      env.testRoom.Id,
			UpToEventID: &e1.Id,
		})
		if err != nil {
			t.Fatalf("mark with stale upToEventId: %v", err)
		}

		got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("get marker: %v", err)
		}
		if got != e2.Id {
			t.Errorf("expected marker to stay at e2 (%s), got %s", e2.Id, got)
		}
	})

	t.Run("advance with a newer upToEventId moves the marker forward", func(t *testing.T) {
		_, err := mutation.MarkRoomAsRead(readerCtx, model.MarkRoomAsReadInput{
			RoomID:      env.testRoom.Id,
			UpToEventID: &e3.Id,
		})
		if err != nil {
			t.Fatalf("mark advance to e3: %v", err)
		}

		got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("get marker: %v", err)
		}
		if got != e3.Id {
			t.Errorf("expected marker = e3 (%s), got %s", e3.Id, got)
		}
	})

	t.Run("unknown upToEventId falls back to room's latest event", func(t *testing.T) {
		bogus := "EDoesNotExist00"
		_, err := mutation.MarkRoomAsRead(readerCtx, model.MarkRoomAsReadInput{
			RoomID:      env.testRoom.Id,
			UpToEventID: &bogus,
		})
		if err != nil {
			t.Fatalf("mark with bogus id: %v", err)
		}
		// Marker should remain at e3 (it's the latest event; the bogus
		// upToEventId can't anchor anything newer).
		got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("get marker: %v", err)
		}
		if got != e3.Id {
			t.Errorf("expected marker = e3 (%s) after bogus upToEventId, got %s", e3.Id, got)
		}
	})
}

// ============================================================================
// MarkThreadAsRead Authorization Tests
// ============================================================================

func TestMarkThreadAsRead_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Post a message to use as thread root
	event, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Thread root message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post message: %v", err)
	}
	threadRootEventId := event.Id

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.MarkThreadAsRead(env.unauthContext(), model.MarkThreadAsReadInput{RoomID: env.testRoom.Id, ThreadRootEventID: threadRootEventId})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-room-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-thread", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.MarkThreadAsRead(env.authContextForUser(outsider), model.MarkThreadAsReadInput{RoomID: env.testRoom.Id, ThreadRootEventID: threadRootEventId})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("space member but not room member is rejected", func(t *testing.T) {
		spaceMember, err := env.core.CreateUser(env.ctx, "system", "spacemember-thread", "Space Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		// Note: not joining the room

		_, err = mutation.MarkThreadAsRead(env.authContextForUser(spaceMember), model.MarkThreadAsReadInput{RoomID: env.testRoom.Id, ThreadRootEventID: threadRootEventId})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("room member can mark thread as opened", func(t *testing.T) {
		// testUser is a room member
		result, err := mutation.MarkThreadAsRead(env.authContext(), model.MarkThreadAsReadInput{RoomID: env.testRoom.Id, ThreadRootEventID: threadRootEventId})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result == nil {
			t.Error("expected result, got nil")
		}
	})

	t.Run("first open returns nil previous time", func(t *testing.T) {
		// Create a new message for this test
		newEvent, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Another root message", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post message: %v", err)
		}

		result, err := mutation.MarkThreadAsRead(env.authContext(), model.MarkThreadAsReadInput{RoomID: env.testRoom.Id, ThreadRootEventID: newEvent.Id})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result.PreviousReadAt != nil {
			t.Errorf("expected nil previous time on first open, got %v", result.PreviousReadAt)
		}
	})

	t.Run("second open returns previous time", func(t *testing.T) {
		// Create a new message for this test
		newEvent, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Yet another root message", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post message: %v", err)
		}

		// First open
		_, err = mutation.MarkThreadAsRead(env.authContext(), model.MarkThreadAsReadInput{RoomID: env.testRoom.Id, ThreadRootEventID: newEvent.Id})
		if err != nil {
			t.Fatalf("first open failed: %v", err)
		}

		// Second open
		result, err := mutation.MarkThreadAsRead(env.authContext(), model.MarkThreadAsReadInput{RoomID: env.testRoom.Id, ThreadRootEventID: newEvent.Id})
		if err != nil {
			t.Fatalf("second open failed: %v", err)
		}
		if result.PreviousReadAt == nil {
			t.Error("expected non-nil previous time on second open")
		}
	})

	// A separate "reader" user who joins the room but never posts. This
	// avoids PostMessage's side-effect of auto-advancing the poster's thread
	// marker to time.Now(), which would otherwise mask the upToEventId
	// timestamp the test is trying to observe.
	threadReader, err := env.core.CreateUser(env.ctx, "system", "reader-thread-upto", "Thread Reader", "password123")
	if err != nil {
		t.Fatalf("create thread reader: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, threadReader.Id, core.KindChannel, threadReader.Id, env.testRoom.Id); err != nil {
		t.Fatalf("add reader to room: %v", err)
	}
	readerCtx := env.authContextForUser(threadReader)

	t.Run("upToEventId anchors thread marker at that event's timestamp", func(t *testing.T) {
		root, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "thread-root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("post thread root: %v", err)
		}
		reply1, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "reply-1", nil, root.Id, "", nil, false)
		if err != nil {
			t.Fatalf("post reply1: %v", err)
		}
		reply2, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "reply-2", nil, root.Id, "", nil, false)
		if err != nil {
			t.Fatalf("post reply2: %v", err)
		}

		// Anchor reader's marker at reply1 even though reply2 also exists.
		// Marker should land at reply1's CreatedAt, not "now".
		_, err = mutation.MarkThreadAsRead(readerCtx, model.MarkThreadAsReadInput{
			RoomID:            env.testRoom.Id,
			ThreadRootEventID: root.Id,
			UpToEventID:       &reply1.Id,
		})
		if err != nil {
			t.Fatalf("mark with upToEventId: %v", err)
		}

		marker, err := env.core.GetThreadLastOpened(env.ctx, core.KindChannel, threadReader.Id, env.testRoom.Id, root.Id)
		if err != nil {
			t.Fatalf("GetThreadLastOpened: %v", err)
		}
		reply2Event, err := env.core.GetRoomEventByEventID(env.ctx, core.KindChannel, env.testRoom.Id, reply2.Id)
		if err != nil || reply2Event == nil {
			t.Fatalf("lookup reply2: %v", err)
		}
		if !marker.Before(reply2Event.CreatedAt.AsTime()) {
			t.Errorf("expected marker (%v) to be before reply2's CreatedAt (%v)", marker, reply2Event.CreatedAt.AsTime())
		}
	})

	t.Run("thread upToEventId is advance-only", func(t *testing.T) {
		root, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "advance-root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("post thread root: %v", err)
		}
		reply1, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "reply-a", nil, root.Id, "", nil, false)
		if err != nil {
			t.Fatalf("post reply1: %v", err)
		}
		reply2, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "reply-b", nil, root.Id, "", nil, false)
		if err != nil {
			t.Fatalf("post reply2: %v", err)
		}

		// Set reader's marker to reply2's time.
		if _, err := mutation.MarkThreadAsRead(readerCtx, model.MarkThreadAsReadInput{
			RoomID:            env.testRoom.Id,
			ThreadRootEventID: root.Id,
			UpToEventID:       &reply2.Id,
		}); err != nil {
			t.Fatalf("mark up to reply2: %v", err)
		}
		marker2, _ := env.core.GetThreadLastOpened(env.ctx, core.KindChannel, threadReader.Id, env.testRoom.Id, root.Id)

		// Try to regress to reply1 — should be ignored.
		if _, err := mutation.MarkThreadAsRead(readerCtx, model.MarkThreadAsReadInput{
			RoomID:            env.testRoom.Id,
			ThreadRootEventID: root.Id,
			UpToEventID:       &reply1.Id,
		}); err != nil {
			t.Fatalf("mark up to reply1: %v", err)
		}
		markerAfter, _ := env.core.GetThreadLastOpened(env.ctx, core.KindChannel, threadReader.Id, env.testRoom.Id, root.Id)
		if !markerAfter.Equal(marker2) {
			t.Errorf("expected marker to stay at reply2's time (%v), got %v", marker2, markerAfter)
		}
	})
}

// ============================================================================
// DeleteServerLogo Authorization Tests
// ============================================================================

func TestDeleteServerLogo_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.DeleteServerLogo(env.unauthContext())
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-member is rejected with permission denied", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-logo", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.DeleteServerLogo(env.authContextForUser(outsider))
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("regular member is rejected with permission denied", func(t *testing.T) {
		member, err := env.core.CreateUser(env.ctx, "system", "member-logo", "Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.DeleteServerLogo(env.authContextForUser(member))
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("admin can delete logo (even if none exists)", func(t *testing.T) {
		// testUser is the server admin. Should succeed even if no logo exists - it's a no-op.
		instance, err := mutation.DeleteServerLogo(env.authContext())
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if instance == nil {
			t.Fatal("expected instance, got nil")
		}
	})
}

// ============================================================================
// DeleteMessage Authorization Tests
// ============================================================================

func TestDeleteMessage_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Post a message to delete
	event, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Message to delete", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post message: %v", err)
	}
	eventID := event.Id

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.DeleteMessage(env.unauthContext(), model.DeleteMessageInput{RoomID: env.testRoom.Id, EventID: eventID})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-delete", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.DeleteMessage(env.authContextForUser(outsider), model.DeleteMessageInput{RoomID: env.testRoom.Id, EventID: eventID})
		if !errors.Is(err, core.ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("member cannot delete another user's message", func(t *testing.T) {
		// Create another member
		otherMember, err := env.core.CreateUser(env.ctx, "system", "othermember-delete", "Other Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, otherMember.Id, core.KindChannel, otherMember.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		// Try to delete testUser's message as otherMember
		_, err = mutation.DeleteMessage(env.authContextForUser(otherMember), model.DeleteMessageInput{RoomID: env.testRoom.Id, EventID: eventID})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("author can delete own message", func(t *testing.T) {
		// Post a new message as testUser
		newEvent, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Another message to delete", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post message: %v", err)
		}

		// Delete own message
		success, err := mutation.DeleteMessage(env.authContext(), model.DeleteMessageInput{RoomID: env.testRoom.Id, EventID: newEvent.Id})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected success=true")
		}

		// Verify message body is deleted
		messageBodyKey := newEvent.Id
		body, err := env.core.GetMessageBody(env.ctx, core.KindChannel, messageBodyKey)
		if err != nil {
			t.Fatalf("failed to get message body: %v", err)
		}
		if body != "" {
			t.Error("expected empty body after deletion")
		}
	})

	// NOTE: Moderator delete (admin deleting any message) is no longer supported.
	// Only the author can delete their own messages.

	t.Run("deleting already deleted message succeeds (idempotent)", func(t *testing.T) {
		// Post and delete a message
		newEvent, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Message for idempotent test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post message: %v", err)
		}

		// Delete once
		_, err = mutation.DeleteMessage(env.authContext(), model.DeleteMessageInput{RoomID: env.testRoom.Id, EventID: newEvent.Id})
		if err != nil {
			t.Fatalf("first delete failed: %v", err)
		}

		// Delete again - should succeed
		success, err := mutation.DeleteMessage(env.authContext(), model.DeleteMessageInput{RoomID: env.testRoom.Id, EventID: newEvent.Id})
		if err != nil {
			t.Fatalf("second delete failed: %v", err)
		}
		if !success {
			t.Error("expected success=true for idempotent delete")
		}
	})
}

func TestDeleteMessage_EchoDeletesOnlyEchoArtifact(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()
	roomResolver := env.resolver.Room()

	rootEvent, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "GraphQL echo delete root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Post root: %v", err)
	}
	replyEvent, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "GraphQL echo delete reply", nil, rootEvent.Id, "", nil, true)
	if err != nil {
		t.Fatalf("Post reply with echo: %v", err)
	}

	events, err := roomResolver.Events(env.authContext(), env.testRoom, nil, nil, nil)
	if err != nil {
		t.Fatalf("Room.Events before delete: %v", err)
	}
	echoID := ""
	for _, event := range events.Events {
		if msg := core.EventMessagePosted(event); msg != nil && msg.GetEchoOfEventId() == replyEvent.Id {
			echoID = event.ID()
			break
		}
	}
	if echoID == "" {
		t.Fatal("expected echoed reply in room events")
	}

	ok, err := mutation.DeleteMessage(env.authContext(), model.DeleteMessageInput{
		RoomID:  env.testRoom.Id,
		EventID: echoID,
	})
	if err != nil {
		t.Fatalf("DeleteMessage echo: %v", err)
	}
	if !ok {
		t.Fatal("DeleteMessage echo returned false")
	}

	events, err = roomResolver.Events(env.authContext(), env.testRoom, nil, nil, nil)
	if err != nil {
		t.Fatalf("Room.Events after delete: %v", err)
	}
	for _, event := range events.Events {
		if event.ID() == echoID {
			t.Fatal("hidden echo should not appear in GraphQL room events")
		}
	}
	if event, err := roomResolver.Event(env.authContext(), env.testRoom, echoID); err != nil {
		t.Fatalf("Room.Event hidden echo: %v", err)
	} else if event != nil {
		t.Fatal("hidden echo should not be directly loadable through GraphQL room event")
	}

	body, err := env.core.GetMessageBody(env.ctx, core.KindChannel, replyEvent.Id)
	if err != nil {
		t.Fatalf("Get original reply body: %v", err)
	}
	if body != "GraphQL echo delete reply" {
		t.Fatalf("original reply body = %q, want %q", body, "GraphQL echo delete reply")
	}
}

// ============================================================================
// DM Reactions Tests
// ============================================================================

func TestAddReaction_DM(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Create a second user for DM
	user2, err := env.core.CreateUser(env.ctx, "system", "dm-react2", "DM React 2", "password123")
	if err != nil {
		t.Fatalf("failed to create user2: %v", err)
	}

	// Create a DM conversation
	dmRoom, _, err := env.core.FindOrCreateDM(env.ctx, env.testUser.Id, []string{user2.Id})
	if err != nil {
		t.Fatalf("failed to create DM: %v", err)
	}

	// Post a message in the DM
	event, err := env.core.PostMessage(env.ctx, core.KindDM, dmRoom.Id, env.testUser.Id, "DM message for reaction test", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post DM message: %v", err)
	}
	messageEventID := event.Id

	t.Run("DM participant can add reaction", func(t *testing.T) {
		success, err := mutation.AddReaction(env.authContext(), model.AddReactionInput{RoomID: dmRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected success=true")
		}
	})

	t.Run("other DM participant can add reaction", func(t *testing.T) {
		success, err := mutation.AddReaction(env.authContextForUser(user2), model.AddReactionInput{RoomID: dmRoom.Id, MessageEventID: messageEventID, Emoji: "heart"})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected success=true")
		}
	})

	t.Run("non-participant cannot add reaction to DM", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "dm-outsider", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create outsider: %v", err)
		}

		_, err = mutation.AddReaction(env.authContextForUser(outsider), model.AddReactionInput{RoomID: dmRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})
}

// ============================================================================
// PostMessage Echo Permission Tests
// ============================================================================

func TestPostMessage_EchoPermission(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Post a root message to reply to
	rootInput := model.PostMessageInput{
		RoomID: env.testRoom.Id,
		Body:   ptr("Thread root for echo test"),
	}
	rootEvent, err := mutation.PostMessage(env.authContext(), rootInput)
	if err != nil {
		t.Fatalf("failed to post root message: %v", err)
	}
	rootEventID := rootEvent.ID()

	t.Run("user with echo permission can post with alsoSendToChannel", func(t *testing.T) {
		alsoSend := true
		input := model.PostMessageInput{
			RoomID:            env.testRoom.Id,
			Body:              ptr("Reply echoed to channel"),
			ThreadRootEventID: &rootEventID,
			AlsoSendToChannel: &alsoSend,
		}

		event, err := mutation.PostMessage(env.authContext(), input)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
	})

	t.Run("alsoSendToChannel without inThread is rejected", func(t *testing.T) {
		alsoSend := true
		input := model.PostMessageInput{
			RoomID:            env.testRoom.Id,
			Body:              ptr("Not a reply but trying to echo"),
			AlsoSendToChannel: &alsoSend,
		}

		_, err = mutation.PostMessage(env.authContext(), input)
		if err == nil {
			t.Error("expected error when alsoSendToChannel is true without inThread")
		}
	})

	t.Run("user without echo permission is denied", func(t *testing.T) {
		// Create a user and deny echo permission on the everyone role
		member, err := env.core.CreateUser(env.ctx, "system", "no-echo-user", "No Echo", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, member.Id, core.KindChannel, member.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		// Deny echo at room level for everyone role (testUser is space owner, has roles.manage)
		err = env.core.DenyRoomPermission(env.ctx, core.SystemActorID, env.testRoom.Id, core.RoleEveryone, core.PermMessageEcho)
		if err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}

		alsoSend := true
		input := model.PostMessageInput{
			RoomID:            env.testRoom.Id,
			Body:              ptr("Reply trying to echo"),
			ThreadRootEventID: &rootEventID,
			AlsoSendToChannel: &alsoSend,
		}

		_, err = mutation.PostMessage(env.authContextForUser(member), input)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("user without message.post permission is denied echo", func(t *testing.T) {
		// Create a user and deny message.post on the everyone role (announcements pattern)
		member3, err := env.core.CreateUser(env.ctx, "system", "no-post-echo-user", "No Post Echo", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, member3.Id, core.KindChannel, member3.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		// Deny message.post at room level for everyone role
		err = env.core.DenyRoomPermission(env.ctx, core.SystemActorID, env.testRoom.Id, core.RoleEveryone, core.PermMessagePost)
		if err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}

		alsoSend := true
		input := model.PostMessageInput{
			RoomID:            env.testRoom.Id,
			Body:              ptr("Reply trying to echo without post permission"),
			ThreadRootEventID: &rootEventID,
			AlsoSendToChannel: &alsoSend,
		}

		_, err = mutation.PostMessage(env.authContextForUser(member3), input)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("user without echo permission can still post normal thread reply", func(t *testing.T) {
		// Create a fresh user — the previous test denied echo on everyone role for the room
		member2, err := env.core.CreateUser(env.ctx, "system", "normal-reply-user", "Normal Reply", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, member2.Id, core.KindChannel, member2.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		// Post normal thread reply without echo — should succeed even though echo is denied
		input := model.PostMessageInput{
			RoomID:            env.testRoom.Id,
			Body:              ptr("Normal thread reply"),
			ThreadRootEventID: &rootEventID,
		}

		event, err := mutation.PostMessage(env.authContextForUser(member2), input)
		if err != nil {
			t.Fatalf("expected success for normal reply, got error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
	})
}

func TestUpdateMessage_EchoState(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	rootEvent, err := mutation.PostMessage(env.authContext(), model.PostMessageInput{
		RoomID: env.testRoom.Id,
		Body:   ptr("Thread root for update echo"),
	})
	if err != nil {
		t.Fatalf("post root: %v", err)
	}
	rootEventID := rootEvent.ID()
	replyEvent, err := mutation.PostMessage(env.authContext(), model.PostMessageInput{
		RoomID:            env.testRoom.Id,
		Body:              ptr("Thread reply for update echo"),
		ThreadRootEventID: &rootEventID,
	})
	if err != nil {
		t.Fatalf("post reply: %v", err)
	}
	replyEventID := replyEvent.ID()

	t.Run("author can add preserve and remove echo while editing", func(t *testing.T) {
		ok, err := mutation.UpdateMessage(env.authContext(), model.UpdateMessageInput{
			RoomID:            env.testRoom.Id,
			EventID:           replyEventID,
			Body:              "edited with echo",
			AlsoSendToChannel: boolPtr(true),
		})
		if err != nil || !ok {
			t.Fatalf("update add echo = %v, %v", ok, err)
		}
		echoID, exists := env.core.RoomTimeline.ChannelEchoEventID(replyEventID)
		if !exists {
			t.Fatal("expected edit to create channel echo")
		}

		ok, err = mutation.UpdateMessage(env.authContext(), model.UpdateMessageInput{
			RoomID:  env.testRoom.Id,
			EventID: replyEventID,
			Body:    "edited preserving echo",
		})
		if err != nil || !ok {
			t.Fatalf("update preserve echo = %v, %v", ok, err)
		}
		if got, exists := env.core.RoomTimeline.ChannelEchoEventID(replyEventID); !exists || got != echoID {
			t.Fatalf("expected nil echo option to preserve echo %q, got %q exists=%v", echoID, got, exists)
		}

		ok, err = mutation.UpdateMessage(env.authContext(), model.UpdateMessageInput{
			RoomID:            env.testRoom.Id,
			EventID:           replyEventID,
			Body:              "edited without echo",
			AlsoSendToChannel: boolPtr(false),
		})
		if err != nil || !ok {
			t.Fatalf("update remove echo = %v, %v", ok, err)
		}
		if _, exists := env.core.RoomTimeline.ChannelEchoEventID(replyEventID); exists {
			t.Fatal("expected echo to be hidden")
		}
	})

	t.Run("moderator cannot change another author's echo state", func(t *testing.T) {
		regular := env.createVerifiedUser(t, "update-echo-regular", "Update Echo Regular", "password123")
		if _, err := env.core.JoinRoom(env.ctx, regular.Id, core.KindChannel, regular.Id, env.testRoom.Id); err != nil {
			t.Fatalf("JoinRoom regular: %v", err)
		}
		regularRoot, err := mutation.PostMessage(env.authContextForUser(regular), model.PostMessageInput{
			RoomID: env.testRoom.Id,
			Body:   ptr("regular root for moderator echo test"),
		})
		if err != nil {
			t.Fatalf("regular post root: %v", err)
		}
		regularRootID := regularRoot.ID()
		regularReply, err := mutation.PostMessage(env.authContextForUser(regular), model.PostMessageInput{
			RoomID:            env.testRoom.Id,
			Body:              ptr("regular reply for moderator echo test"),
			ThreadRootEventID: &regularRootID,
		})
		if err != nil {
			t.Fatalf("regular post reply: %v", err)
		}
		regularReplyID := regularReply.ID()

		moderator := env.createVerifiedUser(t, "update-echo-mod", "Update Echo Mod", "password123")
		if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, moderator.Id, core.RoleModerator); err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}
		if _, err := env.core.JoinRoom(env.ctx, moderator.Id, core.KindChannel, moderator.Id, env.testRoom.Id); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}
		_, err = mutation.UpdateMessage(env.authContextForUser(moderator), model.UpdateMessageInput{
			RoomID:            env.testRoom.Id,
			EventID:           regularReplyID,
			Body:              "moderated body",
			AlsoSendToChannel: boolPtr(true),
		})
		if !errors.Is(err, core.ErrNotMessageAuthor) {
			t.Fatalf("expected ErrNotMessageAuthor, got %v", err)
		}
	})
}
