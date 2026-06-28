package core

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"hmans.de/chatto/internal/core/linkpreview"
)

const (
	providerAvatarFetchTimeout = 10 * time.Second
	providerAvatarMaxBytes     = 5 * 1024 * 1024
)

var providerAvatarClient = linkpreview.NewSSRFSafeClient(providerAvatarFetchTimeout)

// ImportUserAvatarFromURL downloads a remote provider avatar and stores it as a
// Chatto-managed user avatar asset.
func (c *ChattoCore) ImportUserAvatarFromURL(ctx context.Context, userID, avatarURL string) error {
	u, err := url.Parse(avatarURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, avatarURL, nil)
	if err != nil {
		return err
	}

	resp, err := providerAvatarClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	avatarData, err := io.ReadAll(io.LimitReader(resp.Body, providerAvatarMaxBytes+1))
	if err != nil {
		return err
	}
	if len(avatarData) > providerAvatarMaxBytes {
		return fmt.Errorf("avatar exceeds maximum size of %d bytes", providerAvatarMaxBytes)
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
