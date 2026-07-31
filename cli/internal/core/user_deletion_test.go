package core

import (
	"testing"
)

func TestChattoCore_DeleteUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "deletetest", "Delete Test", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Verify user exists
	_, err = core.GetUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get user after creation: %v", err)
	}

	// Delete the user (self-deletion)
	err = core.DeleteUser(ctx, user.Id, user.Id)
	if err != nil {
		t.Fatalf("Failed to delete user: %v", err)
	}

	// Verify user no longer exists
	_, err = core.GetUser(ctx, user.Id)
	if err == nil {
		t.Error("Expected error when getting deleted user")
	}

	// Verify login index is removed (can't retrieve by login)
	_, err = core.GetUserByLogin(ctx, "deletetest")
	if err == nil {
		t.Error("Expected error when getting deleted user by login")
	}

	// Verify password no longer works
	_, err = core.VerifyPassword(ctx, "deletetest", "password123")
	if err == nil {
		t.Error("Expected error when verifying password for deleted user")
	}
}

// TestChattoCore_CanDeleteUser tests the authorization check function.
// Note: Core.DeleteUser no longer checks authorization - that's the API layer's responsibility.
// This test verifies the CanDeleteUser helper that the API layer uses.
func TestChattoCore_CanDeleteUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two users
	user1, err := core.CreateUser(ctx, "system", "user1", "User One", "password123")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}

	user2, err := core.CreateUser(ctx, "system", "user2", "User Two", "password123")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	// user1 can delete themselves
	canDelete, err := core.CanDeleteUser(ctx, user1.Id, user1.Id)
	if err != nil {
		t.Fatalf("CanDeleteUser failed: %v", err)
	}
	if !canDelete {
		t.Error("user1 should be able to delete themselves")
	}

	// user1 cannot delete user2 (no admin permission)
	canDelete, err = core.CanDeleteUser(ctx, user1.Id, user2.Id)
	if err != nil {
		t.Fatalf("CanDeleteUser failed: %v", err)
	}
	if canDelete {
		t.Error("user1 should NOT be able to delete user2 without permission")
	}

	// user2 can still be retrieved (we only tested authorization, not deletion)
	_, err = core.GetUser(ctx, user2.Id)
	if err != nil {
		t.Fatalf("user2 should still exist: %v", err)
	}
}

func TestChattoCore_DeleteUser_PreservesSpaceAndPurgesUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "spacemember", "Space Member", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if err := core.DeleteUser(ctx, user.Id, user.Id); err != nil {
		t.Fatalf("Failed to delete user: %v", err)
	}

	if _, err := core.GetUser(ctx, user.Id); err == nil {
		t.Error("Expected user record to be gone after deletion")
	}
}

func TestChattoCore_DeleteUser_WithVerifiedEmail(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "emailtest", "Email Test", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Add a verified email directly
	err = core.AddVerifiedEmailDirect(ctx, user.Id, "test@example.com")
	if err != nil {
		t.Fatalf("Failed to add verified email: %v", err)
	}

	// Verify email is claimed
	isClaimed, err := core.IsEmailClaimed(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("Failed to check email claim: %v", err)
	}
	if !isClaimed {
		t.Error("Expected email to be claimed")
	}

	// Delete the user
	err = core.DeleteUser(ctx, user.Id, user.Id)
	if err != nil {
		t.Fatalf("Failed to delete user: %v", err)
	}

	// Verify email is no longer claimed (index entry deleted)
	isClaimed, err = core.IsEmailClaimed(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("Failed to check email claim after deletion: %v", err)
	}
	if isClaimed {
		t.Error("Expected email to no longer be claimed after user deletion")
	}
}

func TestChattoCore_DeleteUser_WithMessageBodies(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two users
	user1, err := core.CreateUser(ctx, "system", "msgauthor", "Msg Author", "password123")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}
	user2, err := core.CreateUser(ctx, "system", "otheruser", "Other User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	// Create a space with both users

	// User 2 joins the space

	// Create a room
	room, err := core.CreateRoom(ctx, user1.Id, KindChannel, "", "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Both users join the room
	_, err = core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room (user1): %v", err)
	}
	_, err = core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room (user2): %v", err)
	}

	// User 1 posts two messages
	event1, err := core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "Message 1 from user1", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message 1: %v", err)
	}
	msg1ID := event1.Id

	event2, err := core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "Message 2 from user1", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message 2: %v", err)
	}
	msg2ID := event2.Id

	// User 2 posts one message
	event3, err := core.PostMessage(ctx, KindChannel, room.Id, user2.Id, "Message from user2", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message 3: %v", err)
	}
	msg3ID := event3.Id

	// Verify all message bodies exist
	_, err = core.GetMessageBody(ctx, msg1ID)
	if err != nil {
		t.Fatalf("Expected message 1 to exist: %v", err)
	}
	_, err = core.GetMessageBody(ctx, msg2ID)
	if err != nil {
		t.Fatalf("Expected message 2 to exist: %v", err)
	}
	_, err = core.GetMessageBody(ctx, msg3ID)
	if err != nil {
		t.Fatalf("Expected message 3 to exist: %v", err)
	}

	// Delete user 1
	err = core.DeleteUser(ctx, user1.Id, user1.Id)
	if err != nil {
		t.Fatalf("Failed to delete user1: %v", err)
	}

	// Verify user 1's message bodies are deleted (GetMessageBody returns empty string for missing bodies)
	body1, err := core.GetMessageBody(ctx, msg1ID)
	if err != nil {
		t.Fatalf("Unexpected error getting message 1: %v", err)
	}
	if body1 != "" {
		t.Errorf("Expected message 1 body to be empty after user deletion, got: %s", body1)
	}

	body2, err := core.GetMessageBody(ctx, msg2ID)
	if err != nil {
		t.Fatalf("Unexpected error getting message 2: %v", err)
	}
	if body2 != "" {
		t.Errorf("Expected message 2 body to be empty after user deletion, got: %s", body2)
	}

	// Verify user 2's message body still exists
	body3, err := core.GetMessageBody(ctx, msg3ID)
	if err != nil {
		t.Fatalf("Failed to get message 3: %v", err)
	}
	if body3 == "" {
		t.Error("Expected message 3 body to still exist after user1 deletion")
	}
}

func TestChattoCore_DeleteUser_RoomMembershipIntegrity(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two users
	user1, err := core.CreateUser(ctx, "system", "deleteuser1", "Delete User 1", "password123")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}
	user2, err := core.CreateUser(ctx, "system", "remaininguser", "Remaining User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	// Create a space

	// User 2 joins the space

	// Create a room
	room, err := core.CreateRoom(ctx, user1.Id, KindChannel, "", "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Both users join the room
	_, err = core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room (user1): %v", err)
	}
	_, err = core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room (user2): %v", err)
	}

	// Verify both users are room members
	members, err := core.GetRoomMembersList(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room members before deletion: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("Expected 2 room members before deletion, got %d", len(members))
	}

	// Delete user 1
	err = core.DeleteUser(ctx, user1.Id, user1.Id)
	if err != nil {
		t.Fatalf("Failed to delete user1: %v", err)
	}

	// CRITICAL: Verify user 2 is still a room member
	members, err = core.GetRoomMembersList(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room members after deletion: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("Expected 1 room member after deletion, got %d", len(members))
	}

	// Verify the remaining member is user 2
	if len(members) > 0 && members[0].UserId != user2.Id {
		t.Errorf("Expected remaining member to be user2 (%s), got %s", user2.Id, members[0].UserId)
	}

	// Verify user 2 can still check their own membership
	isMember, err := core.RoomMembershipExists(ctx, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check room membership for user2: %v", err)
	}
	if !isMember {
		t.Error("Expected user2 to still be a room member")
	}

	// Verify a new user can join and be listed
	user3, err := core.CreateUser(ctx, "system", "newuser", "New User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user3: %v", err)
	}
	_, err = core.JoinRoom(ctx, user3.Id, KindChannel, user3.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room (user3): %v", err)
	}

	// Verify all expected members are listed
	members, err = core.GetRoomMembersList(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room members after new user joins: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("Expected 2 room members after new user joins, got %d", len(members))
	}
}
