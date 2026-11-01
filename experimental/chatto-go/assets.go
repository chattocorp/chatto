package chatto

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// AssetURL is a server-issued, expiring capability. Do not log or persist its URL.
type AssetURL struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// Attachment describes an attached file. URL values are detached value types.
type Attachment struct {
	ID                string   `json:"id"`
	Filename          string   `json:"filename"`
	ContentType       string   `json:"contentType"`
	Width             int      `json:"width"`
	Height            int      `json:"height"`
	Description       string   `json:"description,omitempty"`
	AssetURL          AssetURL `json:"assetUrl"`
	ThumbnailAssetURL AssetURL `json:"thumbnailAssetUrl"`
}

// ImageTransform requests a server thumbnail. Fit is a protocol enum name,
// such as IMAGE_FIT_MODE_CONTAIN. The caller chooses dimensions and fit.
type ImageTransform struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Fit    string `json:"fit"`
}

// AssetDownloadOptions controls a bounded download. MaxBytes must be positive.
// A nil Thumbnail selects the original file. Deadlines belong to the context.
type AssetDownloadOptions struct {
	Thumbnail *ImageTransform
	MaxBytes  int64
}

// GetAsset checks current access and returns fresh asset metadata and signed URLs.
// It supports all file types. Do not log or persist the returned URLs.
func (c *Client) GetAsset(ctx context.Context, room, assetID string, thumbnail *ImageTransform) (Attachment, error) {
	var out struct {
		Asset Attachment `json:"asset"`
	}
	input := map[string]any{"roomId": room, "assetId": assetID}
	if thumbnail != nil {
		input["thumbnail"] = thumbnail
	}
	if err := c.call(ctx, "AssetService/GetAsset", input, &out); err != nil {
		return Attachment{}, err
	}
	if out.Asset.ID != assetID {
		return Attachment{}, errors.New("asset identity mismatch")
	}
	return out.Asset, nil
}

// DownloadAsset downloads freshly authorized bytes without forwarding bearer
// credentials or following redirects. URLs must use the server origin. Callers
// must also bound decoded sizes when processing compressed files or images.
func (c *Client) DownloadAsset(ctx context.Context, room, assetID string, options AssetDownloadOptions) ([]byte, error) {
	limit := options.MaxBytes
	if limit <= 0 || limit == 1<<63-1 {
		return nil, errors.New("asset download requires a positive bounded byte limit")
	}
	asset, err := c.GetAsset(ctx, room, assetID, options.Thumbnail)
	if err != nil {
		return nil, err
	}
	address := asset.AssetURL.URL
	if options.Thumbnail != nil && asset.ThumbnailAssetURL.URL != "" {
		address = asset.ThumbnailAssetURL.URL
	}
	return c.DownloadAssetURL(ctx, address, limit)
}

// DownloadAssetURL downloads a server-issued asset URL, such as a profile avatar.
// It sends no bearer credentials, rejects other origins and redirects, and bounds
// encoded bytes. Callers own deadlines, decoding limits, and URL refresh.
func (c *Client) DownloadAssetURL(ctx context.Context, address string, limit int64) ([]byte, error) {
	if limit <= 0 || limit == 1<<63-1 {
		return nil, errors.New("asset download requires a positive bounded byte limit")
	}
	base, _ := url.Parse(c.endpoint)
	u, err := url.Parse(address)
	if err != nil || address == "" {
		return nil, errors.New("invalid asset URL")
	}
	u = base.ResolveReference(u)
	if u.Scheme != base.Scheme || !strings.EqualFold(u.Host, base.Host) || u.User != nil || u.Fragment != "" {
		return nil, errors.New("asset URL must use the Chatto server origin")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("invalid asset request")
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("asset download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("asset download rejected")
	}
	if resp.ContentLength > limit {
		return nil, errors.New("asset exceeds download limit")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("asset download incomplete")
	}
	if int64(len(data)) > limit {
		return nil, errors.New("asset exceeds download limit")
	}
	return data, nil
}
