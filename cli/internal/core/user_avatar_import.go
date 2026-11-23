package core

import (
	"bytes"
	"context"
)

// ImportUserAvatarFromURL fetches a remote provider avatar and stores it as a
// Chatto-managed user avatar asset. Errors do not contain the URL, which can
// identify the account; callers log them.
func (c *ChattoCore) ImportUserAvatarFromURL(ctx context.Context, userID, avatarURL string) error {
	avatarData, err := c.FetchRemoteImage(ctx, avatarURL)
	if err != nil {
		return err
	}

	asset, err := c.UploadUserAvatar(ctx, userID, bytes.NewReader(avatarData))
	if err != nil {
		return err
	}
	if err := c.SetUserAvatar(ctx, userID, asset); err != nil {
		c.CleanupAsset(ctx, DeprecatedAssetFromAsset(asset))
		return err
	}
	return nil
}
