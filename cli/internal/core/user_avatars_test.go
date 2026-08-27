package core

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"
)

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
