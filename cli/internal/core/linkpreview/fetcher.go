package linkpreview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	// Register image decoders for image.Decode used by assets.ProcessLogoImageWithConfig
	_ "image/jpeg"
	_ "image/png"

	"github.com/charmbracelet/log"
	"github.com/otiai10/opengraph/v2"
	"golang.org/x/net/html"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/assets"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	// MaxImageSize is the maximum size of a preview image to download (5MB).
	MaxImageSize = 5 * 1024 * 1024

	// MaxPageSize is the maximum size of an HTML page to read for OG metadata (2MB).
	// OG meta tags are in the <head>, so even very large pages need minimal data.
	MaxPageSize = 2 * 1024 * 1024

	// ImageFetchTimeout is the timeout for downloading preview images.
	ImageFetchTimeout = 10 * time.Second

	// PageFetchTimeout is the timeout for fetching page metadata.
	PageFetchTimeout = 10 * time.Second

	// MaxOEmbedSize bounds metadata returned by recognized embed providers.
	MaxOEmbedSize = 256 * 1024

	// SocialPostFetchTimeout bounds provider metadata and media work as one unit.
	SocialPostFetchTimeout = 20 * time.Second

	// MaxSocialPostImageBytes bounds total source image bytes fetched for one post.
	MaxSocialPostImageBytes int64 = 10 * 1024 * 1024

	// MaxSocialPostImageFetches bounds image downloads and decode attempts per post.
	MaxSocialPostImageFetches = 5
)

// ErrUnavailable marks URLs that were fetched or inspected successfully enough
// to know that Chatto cannot produce a useful preview for them.
var ErrUnavailable = errors.New("link preview unavailable")

var errProviderModeration = errors.New("provider moderation prevents structured preview")

// StoreImageFunc persists a processed preview image under the supplied asset ID.
type StoreImageFunc func(ctx context.Context, assetID string, data []byte, contentType string) (*corev1.AssetRecord, error)

// Fetcher fetches link preview metadata using OpenGraph.
type Fetcher struct {
	httpClient   *http.Client
	imageClient  *http.Client
	assetsConfig *assets.Config
	newAssetID   func() string // Generates new asset IDs
	storeImage   StoreImageFunc
	logger       *log.Logger
}

// NewFetcher creates a new link preview fetcher.
// The newAssetID function is used to generate asset IDs for stored images.
func NewFetcher(assetsConfig *assets.Config, newAssetID func() string, storeImage StoreImageFunc) *Fetcher {
	return &Fetcher{
		httpClient:   NewSSRFSafeClient(PageFetchTimeout),
		imageClient:  NewSSRFSafeClient(ImageFetchTimeout),
		assetsConfig: assetsConfig,
		newAssetID:   newAssetID,
		storeImage:   storeImage,
		logger:       log.WithPrefix("linkpreview"),
	}
}

// FetchResult contains the fetched link preview metadata.
type FetchResult struct {
	Title       string
	Description string
	SiteName    string
	ImageAsset  *corev1.AssetRecord // Image asset if image was downloaded, nil otherwise
	EmbedType   string              // "generic", "youtube", "bluesky"
	EmbedID     string              // Provider-specific canonical ID
	SocialPost  *corev1.SocialPostPreview
}

// Fetch fetches link preview metadata for a URL.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*FetchResult, error) {
	f.logger.Debug("Fetching link preview", "url", rawURL)

	// Check for YouTube first - we can extract the video ID without fetching
	if videoID := ParseYouTubeVideoID(rawURL); videoID != "" {
		f.logger.Debug("Detected YouTube URL", "video_id", videoID)
		return &FetchResult{
			Title:     "YouTube Video",
			EmbedType: "youtube",
			EmbedID:   videoID,
		}, nil
	}

	// Bluesky's oEmbed response supplies the canonical AT URI used to fetch a
	// bounded post snapshot. Fall back to OpenGraph if discovery is unavailable.
	if IsBlueskyPostURL(rawURL) {
		result, err := f.fetchBluesky(ctx, rawURL)
		if err == nil {
			return result, nil
		}
		if errors.Is(err, errProviderModeration) {
			return nil, fmt.Errorf("%w: provider moderation prevents preview", ErrUnavailable)
		}
		f.logger.Warn("Failed to fetch Bluesky oEmbed metadata", "url", rawURL, "error", err)
	}

	// Mastodon status URLs are instance-local, so discovery and status data are
	// fetched from the permalink's origin through the same SSRF-safe client.
	if _, _, ok := ParseMastodonStatusURL(rawURL); ok {
		result, err := f.fetchMastodon(ctx, rawURL)
		if err == nil {
			return result, nil
		}
		if errors.Is(err, errProviderModeration) {
			return nil, fmt.Errorf("%w: provider visibility prevents preview", ErrUnavailable)
		}
		f.logger.Warn("Failed to fetch Mastodon status metadata", "url", rawURL, "error", err)
	}

	// Fetch the page with a size limit to prevent memory exhaustion
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ChattoBot/1.0; Link Preview)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		f.logger.Warn("Failed to fetch page", "url", rawURL, "error", err)
		return nil, fmt.Errorf("%w: fetch page: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: page returned status %d", ErrUnavailable, resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.HasPrefix(contentType, "text/html") && !strings.HasPrefix(contentType, "application/xhtml") {
		return nil, fmt.Errorf("%w: not an HTML page: %s", ErrUnavailable, contentType)
	}

	// Parse OG metadata with a size-limited reader
	og := opengraph.New(rawURL)
	if err := og.Parse(io.LimitReader(resp.Body, MaxPageSize)); err != nil {
		f.logger.Warn("Failed to parse OG metadata", "url", rawURL, "error", err)
		return nil, fmt.Errorf("%w: parse metadata: %v", ErrUnavailable, err)
	}

	// Convert relative URLs to absolute
	og.ToAbs()

	var imageURL string
	if len(og.Image) > 0 {
		imageURL = og.Image[0].URL
	}
	f.logger.Debug("Fetched OG metadata",
		"url", rawURL,
		"title", og.Title,
		"description", truncate(og.Description, 50),
		"site_name", og.SiteName,
		"image_count", len(og.Image),
		"image_url", imageURL,
	)

	result := &FetchResult{
		Title:       og.Title,
		Description: og.Description,
		SiteName:    og.SiteName,
		EmbedType:   "generic",
	}

	// Check if OG detected a video type (YouTube, etc.)
	if strings.Contains(strings.ToLower(og.Type), "video") {
		// Try to extract YouTube video ID from the URL
		if videoID := ParseYouTubeVideoID(rawURL); videoID != "" {
			result.EmbedType = "youtube"
			result.EmbedID = videoID
		}
	}

	// Download and store the preview image if available
	if len(og.Image) > 0 && og.Image[0].URL != "" {
		imageURL := og.Image[0].URL
		f.logger.Debug("Attempting to download preview image", "image_url", imageURL)
		asset, err := f.downloadAndStoreImage(ctx, imageURL)
		if err != nil {
			f.logger.Warn("Failed to download preview image", "url", imageURL, "error", err)
			// Continue without image - don't fail the whole preview
		} else {
			f.logger.Debug("Successfully stored preview image", "asset_id", asset.GetId())
			result.ImageAsset = asset
		}
	} else {
		f.logger.Debug("No preview image found", "url", rawURL)
	}

	return result, nil
}

type blueskyOEmbed struct {
	HTML string `json:"html"`
}

type blueskyGetPosts struct {
	Posts []blueskyPost `json:"posts"`
}

type blueskyLabel struct {
	Value string `json:"val"`
}

type blueskyAuthor struct {
	Handle      string         `json:"handle"`
	DisplayName string         `json:"displayName"`
	Avatar      string         `json:"avatar"`
	Labels      []blueskyLabel `json:"labels"`
}

type blueskyRecord struct {
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
}

type blueskyExternal struct {
	URI         string `json:"uri"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Thumb       string `json:"thumb"`
}

type blueskyImage struct {
	Fullsize    string `json:"fullsize"`
	Alt         string `json:"alt"`
	AspectRatio *struct {
		Width  uint32 `json:"width"`
		Height uint32 `json:"height"`
	} `json:"aspectRatio"`
}

// blueskyEmbed covers the view unions returned for images, external cards,
// quoted records, and record-with-media combinations.
type blueskyEmbed struct {
	External *blueskyExternal   `json:"external"`
	Images   []blueskyImage     `json:"images"`
	Record   *blueskyRecordView `json:"record"`
	Media    *blueskyEmbed      `json:"media"`
}

type blueskyRecordView struct {
	URI    string             `json:"uri"`
	Author blueskyAuthor      `json:"author"`
	Value  blueskyRecord      `json:"value"`
	Labels []blueskyLabel     `json:"labels"`
	Embeds []blueskyEmbed     `json:"embeds"`
	Record *blueskyRecordView `json:"record"`
}

type blueskyPost struct {
	URI    string         `json:"uri"`
	Labels []blueskyLabel `json:"labels"`
	Author blueskyAuthor  `json:"author"`
	Record blueskyRecord  `json:"record"`
	Embed  blueskyEmbed   `json:"embed"`
}

func (f *Fetcher) fetchBluesky(ctx context.Context, rawURL string) (*FetchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, SocialPostFetchTimeout)
	defer cancel()

	endpoint, err := url.Parse("https://embed.bsky.app/oembed")
	if err != nil {
		return nil, fmt.Errorf("parse oEmbed endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("url", rawURL)
	query.Set("format", "json")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create oEmbed request: %w", err)
	}
	req.Header.Set("User-Agent", "ChattoBot/1.0 (Link Preview)")
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch oEmbed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oEmbed returned status %d", resp.StatusCode)
	}

	var metadata blueskyOEmbed
	decoder := json.NewDecoder(io.LimitReader(resp.Body, MaxOEmbedSize))
	if err := decoder.Decode(&metadata); err != nil {
		return nil, fmt.Errorf("decode oEmbed: %w", err)
	}

	embedID, _, err := parseBlueskyOEmbedHTML(metadata.HTML)
	if err != nil {
		return nil, err
	}
	post, err := f.fetchBlueskyPost(ctx, embedID)
	if err != nil {
		return nil, err
	}

	snapshot := &corev1.SocialPostPreview{
		Provider: "bluesky",
		Url:      rawURL,
		Author: &corev1.SocialPostAuthor{
			DisplayName: post.Author.DisplayName,
			Handle:      post.Author.Handle,
		},
		Text: post.Record.Text,
	}
	if snapshot.Author.DisplayName == "" {
		snapshot.Author.DisplayName = post.Author.Handle
	}
	if publishedAt, err := time.Parse(time.RFC3339Nano, post.Record.CreatedAt); err == nil {
		snapshot.PublishedAt = timestamppb.New(publishedAt)
	}
	imageBudget := socialPostImageBudget{
		bytesRemaining:   MaxSocialPostImageBytes,
		fetchesRemaining: MaxSocialPostImageFetches,
	}
	f.applyBlueskyEmbed(ctx, snapshot, &post.Embed, &imageBudget, true)
	if post.Author.Avatar != "" {
		snapshot.Author.AvatarAsset = f.downloadSocialPostImage(ctx, post.Author.Avatar, &imageBudget)
	}

	title := snapshot.Author.DisplayName
	if snapshot.Author.Handle != "" {
		title += " (@" + snapshot.Author.Handle + ")"
	}
	title = truncateUTF8Bytes(title, 300)

	return &FetchResult{
		Title:       title,
		Description: post.Record.Text,
		SiteName:    "Bluesky",
		ImageAsset:  socialPostCompatibilityImage(snapshot),
		EmbedType:   "bluesky",
		EmbedID:     embedID,
		SocialPost:  snapshot,
	}, nil
}

func (f *Fetcher) applyBlueskyEmbed(ctx context.Context, snapshot *corev1.SocialPostPreview, embed *blueskyEmbed, budget *socialPostImageBudget, allowQuote bool) {
	if snapshot == nil || embed == nil {
		return
	}
	if embed.Media != nil {
		f.applyBlueskyEmbed(ctx, snapshot, embed.Media, budget, false)
	}
	remainingImages := max(0, 4-len(snapshot.Images))
	for _, image := range embed.Images[:min(len(embed.Images), remainingImages)] {
		asset := f.downloadSocialPostImage(ctx, image.Fullsize, budget)
		if asset == nil {
			continue
		}
		out := &corev1.SocialPostImage{Asset: asset, Alt: truncateUTF8Bytes(image.Alt, 1000)}
		if image.AspectRatio != nil {
			out.Width = image.AspectRatio.Width
			out.Height = image.AspectRatio.Height
		}
		snapshot.Images = append(snapshot.Images, out)
	}
	if external := embed.External; external != nil {
		externalURL := safeExternalURL(truncateUTF8Bytes(external.URI, 2048))
		if externalURL != "" {
			snapshot.ExternalLink = &corev1.SocialPostExternalLink{
				Url:         externalURL,
				Title:       truncateUTF8Bytes(external.Title, 300),
				Description: truncateUTF8Bytes(external.Description, 1000),
				ImageAsset:  f.downloadSocialPostImage(ctx, external.Thumb, budget),
			}
		}
	}
	if !allowQuote || embed.Record == nil {
		return
	}
	record := unwrapBlueskyRecordView(embed.Record)
	if record == nil || len(record.Labels) > 0 || len(record.Author.Labels) > 0 {
		return
	}
	quote := blueskyRecordSnapshot(record)
	if quote == nil {
		return
	}
	for i := range record.Embeds {
		f.applyBlueskyEmbed(ctx, quote, &record.Embeds[i], budget, false)
	}
	if record.Author.Avatar != "" {
		quote.Author.AvatarAsset = f.downloadSocialPostImage(ctx, record.Author.Avatar, budget)
	}
	snapshot.QuotedPost = quote
}

func unwrapBlueskyRecordView(record *blueskyRecordView) *blueskyRecordView {
	for record != nil && record.URI == "" {
		record = record.Record
	}
	return record
}

func blueskyRecordSnapshot(record *blueskyRecordView) *corev1.SocialPostPreview {
	if record == nil || record.URI == "" || (record.Author.DisplayName == "" && record.Author.Handle == "") {
		return nil
	}
	displayName := truncateUTF8Bytes(record.Author.DisplayName, 300)
	postURL := blueskyPostURL(record.URI, record.Author.Handle)
	handle := truncateUTF8Bytes(record.Author.Handle, 200)
	if displayName == "" {
		displayName = handle
	}
	out := &corev1.SocialPostPreview{
		Provider: "bluesky",
		Url:      postURL,
		Author:   &corev1.SocialPostAuthor{DisplayName: displayName, Handle: handle},
		Text:     truncateUTF8Bytes(record.Value.Text, 1000),
	}
	if out.Url == "" {
		return nil
	}
	if publishedAt, err := time.Parse(time.RFC3339Nano, record.Value.CreatedAt); err == nil {
		out.PublishedAt = timestamppb.New(publishedAt)
	}
	return out
}

func blueskyPostURL(atURI, handle string) string {
	parts := strings.Split(strings.TrimPrefix(atURI, "at://"), "/")
	if len(parts) != 3 || parts[1] != "app.bsky.feed.post" || !validBlueskyRecordKey.MatchString(parts[2]) {
		return ""
	}
	profile := handle
	if profile == "" {
		profile = parts[0]
	}
	return "https://bsky.app/profile/" + url.PathEscape(profile) + "/post/" + url.PathEscape(parts[2])
}

func (f *Fetcher) fetchBlueskyPost(ctx context.Context, atURI string) (*blueskyPost, error) {
	endpoint, _ := url.Parse("https://public.api.bsky.app/xrpc/app.bsky.feed.getPosts")
	query := endpoint.Query()
	query.Set("uris", atURI)
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create Bluesky post request: %w", err)
	}
	req.Header.Set("User-Agent", "ChattoBot/1.0 (Link Preview)")
	req.Header.Set("Accept", "application/json")
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Bluesky post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Bluesky post API returned status %d", resp.StatusCode)
	}
	var result blueskyGetPosts
	if err := json.NewDecoder(io.LimitReader(resp.Body, MaxOEmbedSize)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Bluesky post: %w", err)
	}
	if len(result.Posts) != 1 || result.Posts[0].URI != atURI {
		return nil, errors.New("Bluesky post API returned no matching post")
	}
	if len(result.Posts[0].Labels) > 0 || len(result.Posts[0].Author.Labels) > 0 {
		return nil, errProviderModeration
	}
	post := &result.Posts[0]
	post.Author.DisplayName = truncateUTF8Bytes(post.Author.DisplayName, 300)
	post.Author.Handle = truncateUTF8Bytes(post.Author.Handle, 200)
	post.Record.Text = truncateUTF8Bytes(post.Record.Text, 1000)
	if post.Embed.External != nil {
		post.Embed.External.URI = safeExternalURL(truncateUTF8Bytes(post.Embed.External.URI, 2048))
		if post.Embed.External.URI == "" {
			post.Embed.External = nil
		} else {
			post.Embed.External.Title = truncateUTF8Bytes(post.Embed.External.Title, 300)
			post.Embed.External.Description = truncateUTF8Bytes(post.Embed.External.Description, 1000)
		}
	}
	for i := range post.Embed.Images {
		post.Embed.Images[i].Alt = truncateUTF8Bytes(post.Embed.Images[i].Alt, 1000)
	}
	return post, nil
}

func safeExternalURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return parsed.String()
}

func truncateUTF8Bytes(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

type socialPostImageBudget struct {
	bytesRemaining   int64
	fetchesRemaining int
}

func (f *Fetcher) downloadSocialPostImage(ctx context.Context, imageURL string, budget *socialPostImageBudget) *corev1.AssetRecord {
	if imageURL == "" || f.imageClient == nil || budget == nil || budget.bytesRemaining <= 0 || budget.fetchesRemaining <= 0 {
		return nil
	}
	budget.fetchesRemaining--
	maxBytes := min(int64(MaxImageSize), budget.bytesRemaining)
	asset, consumed, err := f.downloadAndStoreImageWithLimit(ctx, imageURL, maxBytes)
	budget.bytesRemaining -= min(consumed, budget.bytesRemaining)
	if err != nil {
		f.logger.Warn("Failed to persist social-post image", "url", imageURL, "error", err)
		return nil
	}
	return asset
}

func parseBlueskyOEmbedHTML(source string) (embedID string, description string, err error) {
	root, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", "", fmt.Errorf("parse oEmbed HTML: %w", err)
	}

	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "blockquote" {
			for _, attr := range node.Attr {
				if attr.Key == "data-bluesky-uri" && isValidBlueskyATURI(attr.Val) {
					embedID = attr.Val
				}
			}
		}
		if node.Type == html.ElementNode && node.Data == "p" && description == "" {
			description = strings.TrimSpace(nodeText(node))
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	if embedID == "" {
		return "", "", errors.New("oEmbed response has no valid Bluesky AT URI")
	}
	return embedID, description, nil
}

func nodeText(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}
	var text strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		text.WriteString(nodeText(child))
	}
	return text.String()
}

func isValidBlueskyATURI(value string) bool {
	const prefix = "at://did:"
	if !strings.HasPrefix(value, prefix) || len(value) > 256 {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(value, "at://"), "/")
	return len(parts) == 3 && validBlueskyDID.MatchString(parts[0]) &&
		parts[1] == "app.bsky.feed.post" && validBlueskyRecordKey.MatchString(parts[2])
}

var (
	validBlueskyDID       = regexp.MustCompile(`^did:[A-Za-z0-9._:%-]+$`)
	validBlueskyRecordKey = regexp.MustCompile(`^[A-Za-z0-9._:~-]+$`)
)

// truncate truncates a string to maxLen characters, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// downloadAndStoreImage downloads an image and stores it as an server asset.
func (f *Fetcher) downloadAndStoreImage(ctx context.Context, imageURL string) (*corev1.AssetRecord, error) {
	asset, _, err := f.downloadAndStoreImageWithLimit(ctx, imageURL, int64(MaxImageSize))
	return asset, err
}

func (f *Fetcher) downloadAndStoreImageWithLimit(ctx context.Context, imageURL string, maxBytes int64) (*corev1.AssetRecord, int64, error) {
	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "ChattoBot/1.0 (Link Preview)")

	// Fetch the image
	resp, err := f.imageClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()

	f.logger.Debug("Image fetch response",
		"url", imageURL,
		"status", resp.StatusCode,
		"content_type", resp.Header.Get("Content-Type"),
		"content_length", resp.Header.Get("Content-Length"),
	)

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("image returned status %d", resp.StatusCode)
	}

	// Check content type - be lenient since some servers don't set it properly
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.HasPrefix(contentType, "image/") && contentType != "application/octet-stream" {
		return nil, 0, fmt.Errorf("not an image: %s", contentType)
	}
	if resp.ContentLength > maxBytes {
		return nil, 0, fmt.Errorf("image too large (>%d bytes)", maxBytes)
	}

	// Read with size limit
	limitedReader := io.LimitReader(resp.Body, maxBytes+1)
	imageData, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, int64(len(imageData)), fmt.Errorf("read image: %w", err)
	}
	consumed := int64(len(imageData))
	if consumed > maxBytes {
		return nil, consumed, fmt.Errorf("image too large (>%d bytes)", maxBytes)
	}

	f.logger.Debug("Downloaded image data", "size", len(imageData))

	// Process the image (resize to fit OG dimensions, convert to WebP)
	processedReader, err := assets.ProcessLinkPreviewImageWithConfig(bytes.NewReader(imageData), *f.assetsConfig)
	if err != nil {
		return nil, consumed, fmt.Errorf("process image: %w", err)
	}

	processedData, err := io.ReadAll(processedReader)
	if err != nil {
		return nil, consumed, fmt.Errorf("read processed image: %w", err)
	}

	f.logger.Debug("Processed image", "original_size", len(imageData), "processed_size", len(processedData))

	// Generate asset ID and store
	assetID := f.newAssetID()

	if f.storeImage == nil {
		return nil, consumed, fmt.Errorf("store image: no image store configured")
	}
	asset, err := f.storeImage(ctx, assetID, processedData, "image/webp")
	if err != nil {
		return nil, consumed, fmt.Errorf("store image: %w", err)
	}

	f.logger.Debug("Stored image asset", "asset_id", assetID)

	return asset, consumed, nil
}

// ToProto converts a FetchResult to a protobuf LinkPreview.
func (r *FetchResult) ToProto(url string) *corev1.LinkPreview {
	lp := &corev1.LinkPreview{
		Url:         url,
		Title:       r.Title,
		Description: r.Description,
		SiteName:    r.SiteName,
		EmbedType:   r.EmbedType,
	}
	if r.ImageAsset != nil && r.ImageAsset.GetId() != "" {
		imageAssetID := r.ImageAsset.GetId()
		lp.ImageAssetId = &imageAssetID
		lp.ImageAsset = proto.Clone(r.ImageAsset).(*corev1.AssetRecord)
	}
	if r.EmbedID != "" {
		lp.EmbedId = &r.EmbedID
	}
	if r.SocialPost != nil {
		lp.SocialPost = proto.Clone(r.SocialPost).(*corev1.SocialPostPreview)
	}
	return lp
}
