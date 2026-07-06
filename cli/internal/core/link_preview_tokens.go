package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/core/linkpreview"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	linkPreviewTokenKeyPrefix = "link_preview_token."
	// LinkPreviewTokenTTL keeps composer tokens useful across retries and
	// mention-confirmation flows without making cached preview selection durable.
	LinkPreviewTokenTTL = 30 * time.Minute
)

type linkPreviewTokenData struct {
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

func (c *ChattoCore) linkPreviewTokenKey(token string) string {
	return c.runtimeTokenKey(linkPreviewTokenKeyPrefix, token)
}

// CreateLinkPreviewToken stores a short-lived opaque reference to a cached
// server-fetched preview URL.
func (c *ChattoCore) CreateLinkPreviewToken(ctx context.Context, url string) (string, error) {
	if url == "" {
		return "", invalidArgument("link preview URL is required")
	}

	token := NewLinkPreviewToken()
	data, err := json.Marshal(linkPreviewTokenData{
		URL:       url,
		CreatedAt: time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("marshal link preview token: %w", err)
	}
	if _, err := c.storage.runtimeStateKV.Create(ctx, c.linkPreviewTokenKey(token), data, jetstream.KeyTTL(LinkPreviewTokenTTL)); err != nil {
		return "", fmt.Errorf("store link preview token: %w", err)
	}
	return token, nil
}

// ResolveLinkPreviewToken resolves a composer token to the canonical cached
// server-fetched preview. Client-provided metadata is intentionally not trusted.
func (c *ChattoCore) ResolveLinkPreviewToken(ctx context.Context, token string) (*corev1.LinkPreview, error) {
	if token == "" {
		return nil, nil
	}

	key := c.linkPreviewTokenKey(token)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, invalidArgument("link_preview_token is invalid or expired")
		}
		return nil, fmt.Errorf("get link preview token: %w", err)
	}

	var data linkPreviewTokenData
	if err := json.Unmarshal(entry.Value(), &data); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, invalidArgument("link_preview_token is invalid or expired")
	}
	if data.URL == "" || time.Since(data.CreatedAt) > LinkPreviewTokenTTL {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, invalidArgument("link_preview_token is invalid or expired")
	}

	preview, err := c.linkPreviewCache.Get(ctx, data.URL)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, linkpreview.ErrCachedFailure) {
			return nil, invalidArgument("link_preview_token is invalid or expired")
		}
		return nil, fmt.Errorf("get cached link preview: %w", err)
	}
	if preview == nil {
		return nil, invalidArgument("link_preview_token is invalid or expired")
	}

	return proto.Clone(preview).(*corev1.LinkPreview), nil
}
