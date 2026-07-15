// SPDX-FileCopyrightText: 2026 Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package linkpreview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type mastodonOEmbed struct {
	Type        string `json:"type"`
	ProviderURL string `json:"provider_url"`
	HTML        string `json:"html"`
}

type mastodonInstance struct {
	Domain      string `json:"domain"`
	SourceURL   string `json:"source_url"`
	APIVersions struct {
		Mastodon int `json:"mastodon"`
	} `json:"api_versions"`
}

var errMastodonOEmbedNotFound = errors.New("Mastodon oEmbed status not found")

type mastodonAccount struct {
	DisplayName  string `json:"display_name"`
	Acct         string `json:"acct"`
	Avatar       string `json:"avatar"`
	AvatarStatic string `json:"avatar_static"`
}

type mastodonMediaAttachment struct {
	Type        string `json:"type"`
	URL         string `json:"url"`
	PreviewURL  string `json:"preview_url"`
	Description string `json:"description"`
	Meta        struct {
		Original struct {
			Width  uint32 `json:"width"`
			Height uint32 `json:"height"`
		} `json:"original"`
	} `json:"meta"`
}

type mastodonPreviewCard struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image"`
}

type mastodonQuote struct {
	State        string          `json:"state"`
	QuotedStatus *mastodonStatus `json:"quoted_status"`
}

type mastodonStatus struct {
	ID               string                    `json:"id"`
	URL              string                    `json:"url"`
	CreatedAt        string                    `json:"created_at"`
	Visibility       string                    `json:"visibility"`
	Content          string                    `json:"content"`
	SpoilerText      string                    `json:"spoiler_text"`
	Sensitive        bool                      `json:"sensitive"`
	Account          mastodonAccount           `json:"account"`
	MediaAttachments []mastodonMediaAttachment `json:"media_attachments"`
	Card             *mastodonPreviewCard      `json:"card"`
	Quote            *mastodonQuote            `json:"quote"`
	Reblog           *mastodonStatus           `json:"reblog"`
}

func (f *Fetcher) fetchMastodon(ctx context.Context, rawURL string) (*FetchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, SocialPostFetchTimeout)
	defer cancel()

	origin, statusID, ok := ParseMastodonStatusURL(rawURL)
	if !ok {
		return nil, errors.New("invalid Mastodon status URL")
	}
	if err := f.verifyMastodonOEmbed(ctx, origin, rawURL); err != nil {
		if !errors.Is(err, errMastodonOEmbedNotFound) {
			return nil, err
		}
		// Mastodon does not provide oEmbed metadata for every local proxy URL
		// that represents a federated status. In that case, verify the server's
		// public instance metadata before trusting its status API response.
		if err := f.verifyMastodonInstance(ctx, origin); err != nil {
			return nil, err
		}
	}
	status, err := f.fetchMastodonStatus(ctx, origin, statusID)
	if err != nil {
		return nil, err
	}

	// A boost wraps the original status. The current common snapshot can render
	// the original content but does not yet carry neutral boost attribution.
	effective := status
	if status.Reblog != nil {
		effective = status.Reblog
	}
	if effective.Visibility != "public" && effective.Visibility != "unlisted" {
		return nil, errProviderModeration
	}

	budget := socialPostImageBudget{
		bytesRemaining:   MaxSocialPostImageBytes,
		fetchesRemaining: MaxSocialPostImageFetches,
	}
	snapshot := f.mastodonStatusSnapshot(ctx, effective, rawURL, origin, &budget, 0)
	if snapshot == nil {
		return nil, errors.New("Mastodon status has no usable public snapshot")
	}

	title := snapshot.GetAuthor().GetDisplayName()
	if snapshot.GetAuthor().GetHandle() != "" {
		title += " (@" + snapshot.GetAuthor().GetHandle() + ")"
	}

	return &FetchResult{
		Title:       truncateUTF8Bytes(title, 300),
		Description: snapshot.Text,
		SiteName:    "Mastodon",
		ImageAsset:  socialPostCompatibilityImage(snapshot),
		EmbedType:   "mastodon",
		EmbedID:     statusID,
		SocialPost:  snapshot,
	}, nil
}

func (f *Fetcher) verifyMastodonOEmbed(ctx context.Context, origin, rawURL string) error {
	endpoint, err := url.Parse(origin + "/api/oembed")
	if err != nil {
		return fmt.Errorf("parse Mastodon oEmbed endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("url", rawURL)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("create Mastodon oEmbed request: %w", err)
	}
	req.Header.Set("User-Agent", "ChattoBot/1.0 (Link Preview)")
	req.Header.Set("Accept", "application/json")
	resp, err := f.mastodonHTTPClient(origin).Do(req)
	if err != nil {
		return fmt.Errorf("fetch Mastodon oEmbed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errMastodonOEmbedNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Mastodon oEmbed returned status %d", resp.StatusCode)
	}
	var metadata mastodonOEmbed
	if err := json.NewDecoder(io.LimitReader(resp.Body, MaxOEmbedSize)).Decode(&metadata); err != nil {
		return fmt.Errorf("decode Mastodon oEmbed: %w", err)
	}
	if metadata.Type != "rich" {
		return errors.New("Mastodon oEmbed response is not a rich status preview")
	}
	if !sameOrigin(metadata.ProviderURL, origin) || !isMastodonEmbedHTML(metadata.HTML, origin) {
		return errors.New("Mastodon oEmbed response is not bound to the status instance")
	}
	return nil
}

func (f *Fetcher) verifyMastodonInstance(ctx context.Context, origin string) error {
	endpoint := origin + "/api/v2/instance"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create Mastodon instance request: %w", err)
	}
	req.Header.Set("User-Agent", "ChattoBot/1.0 (Link Preview)")
	req.Header.Set("Accept", "application/json")
	resp, err := f.mastodonHTTPClient(origin).Do(req)
	if err != nil {
		return fmt.Errorf("fetch Mastodon instance: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Mastodon instance API returned status %d", resp.StatusCode)
	}

	var instance mastodonInstance
	if err := json.NewDecoder(io.LimitReader(resp.Body, MaxOEmbedSize)).Decode(&instance); err != nil {
		return fmt.Errorf("decode Mastodon instance: %w", err)
	}
	parsedOrigin, err := url.Parse(origin)
	if err != nil || !strings.EqualFold(instance.Domain, parsedOrigin.Hostname()) {
		return errors.New("Mastodon instance metadata is not bound to the status origin")
	}
	if instance.APIVersions.Mastodon <= 0 && !isMastodonSourceURL(instance.SourceURL) {
		return errors.New("instance metadata does not identify a Mastodon server")
	}
	return nil
}

func isMastodonSourceURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "github.com") {
		return false
	}
	return strings.EqualFold(strings.TrimSuffix(parsed.Path, "/"), "/mastodon/mastodon")
}

func (f *Fetcher) fetchMastodonStatus(ctx context.Context, origin, statusID string) (*mastodonStatus, error) {
	endpoint := origin + "/api/v1/statuses/" + url.PathEscape(statusID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create Mastodon status request: %w", err)
	}
	req.Header.Set("User-Agent", "ChattoBot/1.0 (Link Preview)")
	req.Header.Set("Accept", "application/json")
	resp, err := f.mastodonHTTPClient(origin).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Mastodon status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Mastodon status API returned status %d", resp.StatusCode)
	}
	var status mastodonStatus
	if err := json.NewDecoder(io.LimitReader(resp.Body, MaxOEmbedSize)).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode Mastodon status: %w", err)
	}
	if status.ID != statusID {
		return nil, errors.New("Mastodon status API returned no matching status")
	}
	canonicalURL := status.URL
	if status.Reblog != nil {
		canonicalURL = status.Reblog.URL
	}
	if safeExternalURL(canonicalURL) == "" {
		return nil, errors.New("Mastodon status API returned an invalid canonical URL")
	}
	return &status, nil
}

func (f *Fetcher) mastodonStatusSnapshot(ctx context.Context, status *mastodonStatus, fallbackURL, origin string, budget *socialPostImageBudget, quoteDepth int) *corev1.SocialPostPreview {
	if status == nil || (status.Account.DisplayName == "" && status.Account.Acct == "") {
		return nil
	}
	postURL := safeExternalURL(truncateUTF8Bytes(status.URL, 2048))
	if postURL == "" {
		postURL = safeExternalURL(truncateUTF8Bytes(fallbackURL, 2048))
	}
	if postURL == "" {
		return nil
	}

	handle := mastodonHandle(status.Account.Acct, postURL, origin)
	displayName := truncateUTF8Bytes(status.Account.DisplayName, 300)
	if displayName == "" {
		displayName = handle
	}
	snapshot := &corev1.SocialPostPreview{
		Provider: "mastodon",
		Url:      postURL,
		Author: &corev1.SocialPostAuthor{
			DisplayName: displayName,
			Handle:      truncateUTF8Bytes(handle, 200),
		},
		Text: truncateUTF8Bytes(mastodonHTMLText(status.Content), 1000),
	}
	if status.SpoilerText != "" || status.Sensitive {
		warning := truncateUTF8Bytes(strings.TrimSpace(status.SpoilerText), 300)
		if warning == "" {
			warning = "Sensitive content"
		}
		snapshot.ContentWarning = &warning
	}
	if publishedAt, err := time.Parse(time.RFC3339Nano, status.CreatedAt); err == nil {
		snapshot.PublishedAt = timestamppb.New(publishedAt)
	}

	for _, media := range status.MediaAttachments[:min(len(status.MediaAttachments), 4)] {
		if media.Type != "image" {
			continue
		}
		imageURL := safeExternalURL(media.URL)
		if imageURL == "" {
			imageURL = safeExternalURL(media.PreviewURL)
		}
		asset := f.downloadSocialPostImage(ctx, imageURL, budget)
		if asset == nil {
			continue
		}
		snapshot.Images = append(snapshot.Images, &corev1.SocialPostImage{
			Asset:  asset,
			Alt:    truncateUTF8Bytes(media.Description, 1000),
			Width:  media.Meta.Original.Width,
			Height: media.Meta.Original.Height,
		})
	}
	if card := status.Card; card != nil {
		cardURL := safeExternalURL(truncateUTF8Bytes(card.URL, 2048))
		if cardURL != "" {
			snapshot.ExternalLink = &corev1.SocialPostExternalLink{
				Url:         cardURL,
				Title:       truncateUTF8Bytes(card.Title, 300),
				Description: truncateUTF8Bytes(card.Description, 1000),
				ImageAsset:  f.downloadSocialPostImage(ctx, safeExternalURL(card.Image), budget),
			}
		}
	}

	avatarURL := status.Account.AvatarStatic
	if avatarURL == "" {
		avatarURL = status.Account.Avatar
	}
	snapshot.Author.AvatarAsset = f.downloadSocialPostImage(ctx, safeExternalURL(avatarURL), budget)

	if quoteDepth == 0 && status.Quote != nil && status.Quote.State == "accepted" {
		quote := status.Quote.QuotedStatus
		if quote != nil && (quote.Visibility == "public" || quote.Visibility == "unlisted") {
			snapshot.QuotedPost = f.mastodonStatusSnapshot(ctx, quote, quote.URL, origin, budget, quoteDepth+1)
		}
	}
	return snapshot
}

func mastodonHandle(acct, postURL, origin string) string {
	acct = strings.TrimPrefix(strings.TrimSpace(acct), "@")
	if acct == "" || strings.Contains(acct, "@") {
		return acct
	}
	instanceURL, err := url.Parse(postURL)
	if err != nil || instanceURL.Hostname() == "" {
		instanceURL, _ = url.Parse(origin)
	}
	if instanceURL != nil && instanceURL.Hostname() != "" {
		return acct + "@" + strings.ToLower(instanceURL.Hostname())
	}
	return acct
}

func mastodonHTMLText(source string) string {
	root, err := html.Parse(strings.NewReader("<body>" + source + "</body>"))
	if err != nil {
		return ""
	}
	var out strings.Builder
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && hasHTMLClass(node, "quote-inline") {
			return
		}
		if node.Type == html.TextNode {
			out.WriteString(node.Data)
			return
		}
		if node.Type == html.ElementNode && node.Data == "br" {
			out.WriteByte('\n')
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
		if node.Type == html.ElementNode && (node.Data == "p" || node.Data == "div" || node.Data == "li" || node.Data == "blockquote") {
			out.WriteByte('\n')
		}
	}
	visit(root)

	lines := strings.Split(out.String(), "\n")
	cleaned := lines[:0]
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

func hasHTMLClass(node *html.Node, className string) bool {
	for _, attr := range node.Attr {
		if attr.Key == "class" {
			for _, value := range strings.Fields(attr.Val) {
				if value == className {
					return true
				}
			}
		}
	}
	return false
}

func (f *Fetcher) mastodonHTTPClient(origin string) *http.Client {
	client := *f.httpClient
	parentRedirectPolicy := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !sameOrigin(req.URL.String(), origin) {
			return errors.New("Mastodon API redirect changed instance origin")
		}
		if parentRedirectPolicy != nil {
			return parentRedirectPolicy(req, via)
		}
		return nil
	}
	return &client
}

func sameOrigin(rawURL, rawOrigin string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	origin, err := url.Parse(rawOrigin)
	if err != nil || origin.Scheme == "" || origin.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Scheme, origin.Scheme) && strings.EqualFold(parsed.Host, origin.Host)
}

func isMastodonEmbedHTML(source, origin string) bool {
	root, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return false
	}
	var valid bool
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if valid {
			return
		}
		if node.Type == html.ElementNode && hasHTMLClass(node, "mastodon-embed") {
			for _, attr := range node.Attr {
				if (node.Data != "iframe" || attr.Key != "src") &&
					(node.Data != "blockquote" || attr.Key != "data-embed-url") {
					continue
				}
				statusURL := strings.TrimSuffix(attr.Val, "/embed")
				if statusURL != attr.Val && sameOrigin(attr.Val, origin) {
					_, _, isStatusURL := ParseMastodonStatusURL(statusURL)
					if !isStatusURL {
						continue
					}
					valid = true
					return
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	return valid
}

func socialPostCompatibilityImage(snapshot *corev1.SocialPostPreview) *corev1.AssetRecord {
	if snapshot == nil {
		return nil
	}
	if len(snapshot.Images) > 0 {
		return snapshot.Images[0].GetAsset()
	}
	if snapshot.ExternalLink != nil {
		return snapshot.ExternalLink.GetImageAsset()
	}
	return socialPostCompatibilityImage(snapshot.QuotedPost)
}
