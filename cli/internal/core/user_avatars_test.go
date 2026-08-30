package core

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"
	"time"

	"hmans.de/chatto/internal/evtstream"
)

func TestRequireCanManageUserAvatarAuthorizationMatrix(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "avatarowner", "Avatar Owner", "")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	otherHuman, err := c.CreateUser(ctx, SystemActorID, "avatarhuman", "Avatar Human", "")
	if err != nil {
		t.Fatalf("CreateUser other human: %v", err)
	}
	unrelated, err := c.CreateUser(ctx, SystemActorID, "avatarunrelated", "Avatar Unrelated", "")
	if err != nil {
		t.Fatalf("CreateUser unrelated: %v", err)
	}
	accountManager, err := c.CreateUser(ctx, SystemActorID, "avataraccountmanager", "Avatar Account Manager", "")
	if err != nil {
		t.Fatalf("CreateUser account manager: %v", err)
	}
	botManager, err := c.CreateUser(ctx, SystemActorID, "avatarbotmanager", "Avatar Bot Manager", "")
	if err != nil {
		t.Fatalf("CreateUser bot manager: %v", err)
	}
	bot, err := c.CreateBot(ctx, owner.GetId(), "avatar_helper_bot", "Avatar Helper Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, accountManager.GetId(), PermUserManageAccounts); err != nil {
		t.Fatalf("grant user.manage-accounts: %v", err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, botManager.GetId(), PermBotManage); err != nil {
		t.Fatalf("grant bot.manage: %v", err)
	}

	tests := []struct {
		name     string
		actorID  string
		targetID string
		wantErr  error
	}{
		{name: "human self", actorID: owner.GetId(), targetID: owner.GetId()},
		{name: "bot self", actorID: bot.User.GetId(), targetID: bot.User.GetId()},
		{name: "bot owner", actorID: owner.GetId(), targetID: bot.User.GetId()},
		{name: "account manager human", actorID: accountManager.GetId(), targetID: otherHuman.GetId()},
		{name: "account manager bot", actorID: accountManager.GetId(), targetID: bot.User.GetId()},
		{name: "bot manager bot", actorID: botManager.GetId(), targetID: bot.User.GetId()},
		{name: "unrelated human", actorID: unrelated.GetId(), targetID: otherHuman.GetId(), wantErr: ErrPermissionDenied},
		{name: "unrelated bot", actorID: unrelated.GetId(), targetID: bot.User.GetId(), wantErr: ErrPermissionDenied},
		{name: "bot targets human", actorID: bot.User.GetId(), targetID: otherHuman.GetId(), wantErr: ErrPermissionDenied},
		{name: "missing target", actorID: owner.GetId(), targetID: NewUserID(), wantErr: ErrNotFound},
		{name: "malformed target", actorID: owner.GetId(), targetID: "not-a-user-id", wantErr: ErrInvalidArgument},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := c.requireCanManageUserAvatar(ctx, test.actorID, test.targetID)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("requireCanManageUserAvatar() error = %v, want %v", err, test.wantErr)
			}
		})
	}

	if _, err := c.GetBot(ctx, accountManager.GetId(), bot.User.GetId()); err != nil {
		t.Fatalf("account manager GetBot: %v", err)
	}
	visibleBots, err := c.ListBots(ctx, accountManager.GetId())
	if err != nil || len(visibleBots) != 1 || visibleBots[0].User.GetId() != bot.User.GetId() {
		t.Fatalf("account manager ListBots = %+v, %v; want target bot", visibleBots, err)
	}
	if _, err := c.CreateBotAPIKey(ctx, accountManager.GetId(), bot.User.GetId(), "Not allowed"); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("account manager CreateBotAPIKey err = %v, want permission denied", err)
	}
}

func TestManagedBotAvatarUsesCanonicalProjectionAndIdempotentClear(t *testing.T) {
	c, nc := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "managedavatarowner", "Managed Avatar Owner", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bot, err := c.CreateBot(ctx, owner.GetId(), "managed_avatar_bot", "Managed Avatar Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	sub, err := nc.SubscribeSync("live.sync.user." + bot.User.GetId() + ".profile_updated")
	if err != nil {
		t.Fatalf("SubscribeSync: %v", err)
	}

	updated, err := c.UpdateUserAvatar(ctx, owner.GetId(), bot.User.GetId(), createTestImage(100, 100))
	if err != nil {
		t.Fatalf("UpdateUserAvatar: %v", err)
	}
	if updated.GetId() != bot.User.GetId() {
		t.Fatalf("updated user ID = %q, want %q", updated.GetId(), bot.User.GetId())
	}
	if avatar, _ := c.GetUserAvatar(ctx, bot.User.GetId()); avatar == nil {
		t.Fatal("bot avatar was not projected")
	}
	if _, err := sub.NextMsg(time.Second); err != nil {
		t.Fatalf("profile update after upload: %v", err)
	}
	uploadEvents, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(bot.User.GetId()).Subject(evtstream.EventAssetCreated))
	if err != nil || len(uploadEvents) != 1 || uploadEvents[0].GetActorId() != owner.GetId() {
		t.Fatalf("avatar upload events = %+v, %v; want one event by owner", uploadEvents, err)
	}

	if _, err := c.ClearUserAvatar(ctx, owner.GetId(), bot.User.GetId()); err != nil {
		t.Fatalf("ClearUserAvatar: %v", err)
	}
	if _, err := sub.NextMsg(time.Second); err != nil {
		t.Fatalf("profile update after clear: %v", err)
	}
	if _, err := c.ClearUserAvatar(ctx, owner.GetId(), bot.User.GetId()); err != nil {
		t.Fatalf("idempotent ClearUserAvatar: %v", err)
	}
	if avatar, _ := c.GetUserAvatar(ctx, bot.User.GetId()); avatar != nil {
		t.Fatalf("avatar after clear = %+v, want nil", avatar)
	}
	clearEvents, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(bot.User.GetId()).Subject(evtstream.EventUserAvatarCleared))
	if err != nil || len(clearEvents) != 1 || clearEvents[0].GetActorId() != owner.GetId() {
		t.Fatalf("avatar clear events = %+v, %v; want one event by owner", clearEvents, err)
	}
}

// createTestImage creates a test PNG image with the specified dimensions.
func createTestImage(width, height int) io.Reader {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return bytes.NewReader(buf.Bytes())
}

func TestChattoCore_UploadUserAvatar(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "avataruser", "Avatar User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Upload avatar
	testImage := createTestImage(100, 100)
	asset, err := core.UploadUserAvatar(ctx, user.Id, testImage)
	if err != nil {
		t.Fatalf("Failed to upload avatar: %v", err)
	}

	if asset == nil {
		t.Fatal("Expected asset to be returned")
	}

	// Verify it's a NATS asset
	natsAsset := asset.GetNats()
	if natsAsset == nil {
		t.Fatal("Expected NATS asset")
	}

	if natsAsset.Key == "" {
		t.Error("Expected asset key to be set")
	}
	if want := PublicServerAssetObjectKey(asset.GetId()); natsAsset.GetKey() != want {
		t.Errorf("avatar NATS key = %q, want %q", natsAsset.GetKey(), want)
	}
}

func TestChattoCore_SetUserAvatar(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "avataruser", "Avatar User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Initially no avatar
	avatar, err := core.GetUserAvatar(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get avatar: %v", err)
	}
	if avatar != nil {
		t.Error("Expected no avatar initially")
	}

	// Upload and set avatar
	testImage := createTestImage(100, 100)
	asset, err := core.UploadUserAvatar(ctx, user.Id, testImage)
	if err != nil {
		t.Fatalf("Failed to upload avatar: %v", err)
	}

	err = core.SetUserAvatar(ctx, user.Id, asset)
	if err != nil {
		t.Fatalf("Failed to set avatar: %v", err)
	}

	// Verify avatar is set (stored separately from user record)
	avatar, err = core.GetUserAvatar(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get avatar: %v", err)
	}
	if avatar == nil {
		t.Fatal("Expected avatar to be set")
	}

	if avatar.GetNats().Key != asset.GetNats().Key {
		t.Error("Avatar key mismatch")
	}
}

func TestChattoCore_SetUserAvatar_DoesNotModifyUserProfile(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "avataruser", "Avatar User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Upload and set avatar
	testImage := createTestImage(100, 100)
	asset, _ := core.UploadUserAvatar(ctx, user.Id, testImage)
	err = core.SetUserAvatar(ctx, user.Id, asset)
	if err != nil {
		t.Fatalf("Failed to set avatar: %v", err)
	}

	updated, err := core.GetUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}
	if updated.Login != user.Login || updated.DisplayName != user.DisplayName {
		t.Error("User profile fields were modified when avatar changed")
	}

	avatar, err := core.GetUserAvatar(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get avatar: %v", err)
	}
	if avatar == nil || avatar.GetNats().GetKey() != asset.GetNats().GetKey() {
		t.Error("Expected avatar projection to contain the uploaded avatar")
	}
}

func TestChattoCore_GetUserAvatarURL(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "avataruser", "Avatar User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// No avatar initially - should return empty string
	url, err := core.GetUserAvatarURL(ctx, user.Id, nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to get avatar URL: %v", err)
	}
	if url != "" {
		t.Errorf("Expected empty URL for user without avatar, got '%s'", url)
	}

	// Upload and set avatar
	testImage := createTestImage(100, 100)
	asset, _ := core.UploadUserAvatar(ctx, user.Id, testImage)
	core.SetUserAvatar(ctx, user.Id, asset)

	// Now should return URL
	url, err = core.GetUserAvatarURL(ctx, user.Id, nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to get avatar URL: %v", err)
	}
	if url == "" {
		t.Error("Expected non-empty URL after setting avatar")
	}

	// URL should contain the asset key
	if !bytes.Contains([]byte(url), []byte(asset.GetNats().Key)) {
		t.Errorf("URL should contain asset key, got '%s'", url)
	}
}

func TestChattoCore_GetUserAvatarURL_AbsoluteURL(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user with an avatar
	user, err := core.CreateUser(ctx, "system", "absurl-user", "Abs URL User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	testImage := createTestImage(100, 100)
	asset, _ := core.UploadUserAvatar(ctx, user.Id, testImage)
	core.SetUserAvatar(ctx, user.Id, asset)

	t.Run("returns relative URL when AssetBaseURL is empty", func(t *testing.T) {
		core.AssetBaseURL = ""
		url, err := core.GetUserAvatarURL(ctx, user.Id, nil, nil, "")
		if err != nil {
			t.Fatalf("Failed to get avatar URL: %v", err)
		}
		if !bytes.HasPrefix([]byte(url), []byte("/assets/server/")) {
			t.Errorf("Expected relative URL starting with /assets/server/, got '%s'", url)
		}
	})

	t.Run("returns absolute URL when AssetBaseURL is set", func(t *testing.T) {
		core.AssetBaseURL = "https://chat.example.com"
		defer func() { core.AssetBaseURL = "" }()

		url, err := core.GetUserAvatarURL(ctx, user.Id, nil, nil, "")
		if err != nil {
			t.Fatalf("Failed to get avatar URL: %v", err)
		}
		if !bytes.HasPrefix([]byte(url), []byte("https://chat.example.com/assets/server/")) {
			t.Errorf("Expected absolute URL, got '%s'", url)
		}
	})

	t.Run("returns absolute transformed URL when AssetBaseURL is set", func(t *testing.T) {
		core.AssetBaseURL = "https://chat.example.com"
		defer func() { core.AssetBaseURL = "" }()

		w, h := 64, 64
		url, err := core.GetUserAvatarURL(ctx, user.Id, &w, &h, "cover")
		if err != nil {
			t.Fatalf("Failed to get avatar URL: %v", err)
		}
		if !bytes.HasPrefix([]byte(url), []byte("https://chat.example.com/assets/server/")) {
			t.Errorf("Expected absolute transformed URL, got '%s'", url)
		}
	})
}

func TestChattoCore_UploadUserAvatar_ReplacesOld(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "replaceuser", "Replace User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Upload first avatar
	testImage1 := createTestImage(50, 50)
	asset1, _ := core.UploadUserAvatar(ctx, user.Id, testImage1)
	core.SetUserAvatar(ctx, user.Id, asset1)
	oldKey := asset1.GetNats().Key

	// Upload second avatar (should delete old one)
	testImage2 := createTestImage(75, 75)
	asset2, err := core.UploadUserAvatar(ctx, user.Id, testImage2)
	if err != nil {
		t.Fatalf("Failed to upload second avatar: %v", err)
	}

	// Keys should be different
	if asset2.GetNats().Key == oldKey {
		t.Error("Expected different asset keys for old and new avatars")
	}

	// Old asset should be deleted from object store
	_, err = core.ServerStore().Get(ctx, oldKey)
	if err == nil {
		t.Error("Expected old avatar to be deleted from object store")
	}
}

func TestChattoCore_UploadUserAvatar_InvalidUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	testImage := createTestImage(100, 100)
	_, err := core.UploadUserAvatar(ctx, "nonexistent", testImage)
	if err == nil {
		t.Error("Expected error when uploading avatar for non-existent user")
	}
}

func TestChattoCore_DeleteUserAvatar(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "deleteavataruser", "Delete Avatar User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Upload and set an avatar
	testImage := createTestImage(100, 100)
	asset, err := core.UploadUserAvatar(ctx, user.Id, testImage)
	if err != nil {
		t.Fatalf("Failed to upload avatar: %v", err)
	}
	err = core.SetUserAvatar(ctx, user.Id, asset)
	if err != nil {
		t.Fatalf("Failed to set avatar: %v", err)
	}

	// Verify avatar is set
	url, err := core.GetUserAvatarURL(ctx, user.Id, nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to get avatar URL: %v", err)
	}
	if url == "" {
		t.Fatal("Expected avatar URL to be set before deletion")
	}

	// Delete the avatar
	err = core.DeleteUserAvatar(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to delete avatar: %v", err)
	}

	// Verify avatar is gone
	url, err = core.GetUserAvatarURL(ctx, user.Id, nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to get avatar URL after deletion: %v", err)
	}
	if url != "" {
		t.Errorf("Expected empty avatar URL after deletion, got '%s'", url)
	}

	// Verify asset was removed from object store
	_, err = core.ServerStore().Get(ctx, asset.GetNats().Key)
	if err == nil {
		t.Error("Expected asset to be deleted from object store")
	}
}

func TestChattoCore_DeleteUser_CleansUpAvatarCache(t *testing.T) {
	core, _ := setupTestCoreWithCache(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "avatarcacheuser", "Avatar Cache User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	asset, err := core.UploadUserAvatar(ctx, user.Id, bytes.NewReader(createTestPNG(100, 100)))
	if err != nil {
		t.Fatalf("Failed to upload avatar: %v", err)
	}
	if err := core.SetUserAvatar(ctx, user.Id, asset); err != nil {
		t.Fatalf("Failed to set avatar: %v", err)
	}

	cacheKey := ImageCacheKey(ServerAssetSignResource, asset.Id, 64, 64, "cover")
	if err := core.StoreCachedResize(ctx, cacheKey, []byte("fake webp data")); err != nil {
		t.Fatalf("Failed to store avatar cached resize: %v", err)
	}

	if err := core.DeleteUser(ctx, user.Id, user.Id); err != nil {
		t.Fatalf("Failed to delete user: %v", err)
	}

	data, err := core.GetCachedResize(ctx, cacheKey)
	if err != nil {
		t.Fatalf("Unexpected error getting avatar cached resize: %v", err)
	}
	if data != nil {
		t.Fatal("Avatar cache entry should be deleted during account deletion")
	}
}

func TestChattoCore_DeleteUserAvatar_NoAvatar(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user without an avatar
	user, err := core.CreateUser(ctx, "system", "noavataruser", "No Avatar User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Delete should be a no-op (not an error)
	err = core.DeleteUserAvatar(ctx, user.Id)
	if err != nil {
		t.Errorf("DeleteUserAvatar on user without avatar should not error, got: %v", err)
	}

	// Verify still no avatar
	url, err := core.GetUserAvatarURL(ctx, user.Id, nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to get avatar URL: %v", err)
	}
	if url != "" {
		t.Errorf("Expected empty avatar URL, got '%s'", url)
	}
}
