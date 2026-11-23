package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/chattocorp/chatto/cli/internal/assets"
	"github.com/chattocorp/chatto/cli/internal/core/linkpreview"
)

const (
	providerAvatarFetchTimeout = 10 * time.Second
	providerAvatarMaxBytes     = 5 * 1024 * 1024
)

var providerAvatarClient = linkpreview.NewSSRFSafeClient(providerAvatarFetchTimeout)

// ImportUserAvatarFromURL downloads a remote provider avatar and stores it as a
// Chatto-managed user avatar asset.
func (c *ChattoCore) ImportUserAvatarFromURL(ctx context.Context, userID, avatarURL string) error {
	avatarData, err := fetchProviderAvatar(ctx, avatarURL)
	if err != nil || avatarData == nil {
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

// PendingExternalIdentityAvatar fetches the provider avatar of a pending flow
// and returns it as a processed WebP image, so the account creation page can
// show it without the browser contacting the provider. It returns
// ErrNotFound when the flow has no avatar or the provider does not supply one.
// It stores nothing; account creation imports the avatar again.
func (c *ChattoCore) PendingExternalIdentityAvatar(ctx context.Context, token string) ([]byte, error) {
	flow, err := c.GetPendingExternalIdentityFlow(ctx, token)
	if err != nil {
		return nil, err
	}
	avatarData, err := fetchProviderAvatar(ctx, flow.AvatarURL)
	if err != nil {
		return nil, err
	}
	if avatarData == nil {
		return nil, ErrNotFound
	}
	webp, err := assets.ProcessAvatarImageWithConfig(bytes.NewReader(avatarData), c.AssetsConfig())
	if err != nil {
		return nil, err
	}
	return io.ReadAll(webp)
}

// fetchProviderAvatar downloads a provider avatar through the SSRF-safe
// client. It returns nil data without an error when the URL is not HTTP(S) or
// the provider does not answer with 200 OK.
func fetchProviderAvatar(ctx context.Context, avatarURL string) ([]byte, error) {
	u, err := url.Parse(avatarURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, avatarURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := providerAvatarClient.Do(req)
	if err != nil {
		// A *url.Error names the full URL, which can identify the account, for
		// example an AT Protocol DID. Callers log this error.
		if urlErr, ok := errors.AsType[*url.Error](err); ok {
			return nil, fmt.Errorf("fetch provider avatar: %w", urlErr.Err)
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	avatarData, err := io.ReadAll(io.LimitReader(resp.Body, providerAvatarMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(avatarData) > providerAvatarMaxBytes {
		return nil, fmt.Errorf("avatar exceeds maximum size of %d bytes", providerAvatarMaxBytes)
	}
	return avatarData, nil
}
