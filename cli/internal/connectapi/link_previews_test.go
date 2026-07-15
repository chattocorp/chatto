package connectapi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestAPILinkPreviewMapsProviderNeutralSocialPost(t *testing.T) {
	publishedAt := timestamppb.New(time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC))
	preview := apiLinkPreview(&API{}, &corev1.LinkPreview{
		Url:         "https://bsky.app/profile/bsky.app/post/example",
		Title:       "Bluesky (@bsky.app)",
		Description: "A post rendered by Chatto.",
		EmbedType:   "bluesky",
		SocialPost: &corev1.SocialPostPreview{
			Provider: "bluesky",
			Url:      "https://bsky.app/profile/bsky.app/post/example",
			Author: &corev1.SocialPostAuthor{
				DisplayName: "Bluesky",
				Handle:      "bsky.app",
			},
			Text:        "A post rendered by Chatto.",
			PublishedAt: publishedAt,
			ExternalLink: &corev1.SocialPostExternalLink{
				Url:         "https://example.com/story",
				Title:       "Story",
				Description: "Description",
			},
			ContentWarning: stringPtr("Spoilers"),
			QuotedPost: &corev1.SocialPostPreview{
				Provider: "bluesky",
				Url:      "https://bsky.app/profile/quoted.example/post/quoted",
				Author:   &corev1.SocialPostAuthor{Handle: "quoted.example"},
				Text:     "Quoted words.",
			},
		},
	})

	require.NotNil(t, preview.GetSocialPost())
	assert.Equal(t, "bluesky", preview.GetSocialPost().GetProvider())
	assert.Equal(t, "A post rendered by Chatto.", preview.GetSocialPost().GetText())
	assert.Equal(t, "Bluesky", preview.GetSocialPost().GetAuthor().GetDisplayName())
	assert.Equal(t, "bsky.app", preview.GetSocialPost().GetAuthor().GetHandle())
	assert.Equal(t, publishedAt, preview.GetSocialPost().GetPublishedAt())
	assert.Equal(t, "https://example.com/story", preview.GetSocialPost().GetExternalLink().GetUrl())
	assert.Equal(t, "Spoilers", preview.GetSocialPost().GetContentWarning())
	assert.Equal(t, "https://bsky.app/profile/bsky.app/post/example", preview.GetSocialPost().GetUrl())
	require.NotNil(t, preview.GetSocialPost().GetQuotedPost())
	assert.Equal(t, "Quoted words.", preview.GetSocialPost().GetQuotedPost().GetText())
	assert.Equal(t, "https://bsky.app/profile/quoted.example/post/quoted", preview.GetSocialPost().GetQuotedPost().GetUrl())
}
