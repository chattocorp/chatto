package chatto

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAssetDownloadFreshAuthorizationAndDownloadBoundary(t *testing.T) {
	for _, mode := range []string{"relative", "absolute", "external", "redirect", "oversized", "denied", "broken"} {
		t.Run(mode, func(t *testing.T) {
			gets := 0
			var origin string
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					if r.URL.Path != "/api/connect/chatto.api.v1.AssetService/GetAsset" || r.Header.Get("Authorization") != "Bearer token" {
						t.Error("missing authorized asset lookup")
					}
					var in struct {
						Room, Asset string
						Thumbnail   struct {
							Width, Height int
							Fit           string
						}
					}
					var request map[string]json.RawMessage
					_ = json.NewDecoder(r.Body).Decode(&request)
					_ = json.Unmarshal(request["roomId"], &in.Room)
					_ = json.Unmarshal(request["assetId"], &in.Asset)
					_ = json.Unmarshal(request["thumbnail"], &in.Thumbnail)
					if in.Room != "room" || in.Asset != "image" || in.Thumbnail.Width != 320 || in.Thumbnail.Height != 180 || in.Thumbnail.Fit != "IMAGE_FIT_MODE_CONTAIN" {
						t.Error("incorrect asset scope")
					}
					address := "/assets/image?access=private-signature"
					if mode == "absolute" {
						address = origin + address
					}
					if mode == "external" {
						address = "https://external.invalid/image?access=private-signature"
					}
					if mode == "broken" {
						address = "http://%invalid?access=private-signature"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"asset": Attachment{ID: "image", ContentType: "image/png", ThumbnailAssetURL: AssetURL{URL: address}}})
					return
				}
				gets++
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("credential forwarded to signed asset")
				}
				if r.URL.Query().Get("access") != "private-signature" {
					t.Error("signed capability lost")
				}
				switch mode {
				case "redirect":
					http.Redirect(w, r, "/unexpected", http.StatusFound)
				case "oversized":
					w.Header().Set("Content-Length", "8388609")
				case "denied":
					w.WriteHeader(http.StatusForbidden)
				default:
					_, _ = fmt.Fprint(w, "synthetic-image")
				}
			}))
			defer s.Close()
			origin = s.URL
			c, _ := New(s.URL, "token")
			data, err := c.DownloadAsset(context.Background(), "room", "image", AssetDownloadOptions{MaxBytes: 8 << 20, Thumbnail: &ImageTransform{Width: 320, Height: 180, Fit: "IMAGE_FIT_MODE_CONTAIN"}})
			if mode == "relative" || mode == "absolute" {
				if err != nil || string(data) != "synthetic-image" {
					t.Fatal("preview failed", err)
				}
			} else if err == nil {
				t.Fatal("unsafe or failed download accepted")
			}
			if err != nil && strings.Contains(err.Error(), "private-signature") {
				t.Fatal("signed URL leaked")
			}
			if gets > 1 || ((mode == "external" || mode == "broken") && gets != 0) {
				t.Fatal("unexpected download or redirect")
			}
		})
	}
}

func TestAttachmentDecodeAndStoreIsolation(t *testing.T) {
	var msg Message
	if err := json.Unmarshal([]byte(`{"id":"m","attachments":[{"id":"a","filename":"test.png","width":32,"height":16,"assetUrl":{"url":"/signed"}}]}`), &msg); err != nil {
		t.Fatal(err)
	}
	page := TimelinePage{Messages: []Message{msg}}
	copy := clonePage(page)
	copy.Messages[0].Attachments[0].AssetURL.URL = "changed"
	copy.Messages[0].Attachments[0].Filename = "changed"
	if page.Messages[0].Attachments[0].AssetURL.URL != "/signed" || page.Messages[0].Attachments[0].Filename != "test.png" {
		t.Fatal("detached store shares attachment data")
	}
}
