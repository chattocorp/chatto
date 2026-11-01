package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func imageModel() *model {
	m := exampleModel()
	m.images.enabled = true
	m.images.room = m.room
	m.images.entries = map[string]*terminalImage{"room/a": {id: 0x123456, width: 320, height: 160, placements: map[int]bool{}}}
	m.page.Messages[0].Attachments = []chatto.Attachment{{ID: "a", Filename: "sample.png", ContentType: "image/png"}}
	m.renderMessages()
	return m
}

func TestPaneHighlightsFillGuttersWithoutCoveringSeparators(t *testing.T) {
	m := imageModel()
	m.root = "m"
	m.thread = m.page
	m.layout()
	left, center, right := m.columns()
	buf := uv.NewScreenBuffer(m.width, m.height)
	uv.NewStyledString(m.View().Content).Draw(buf, buf.Bounds())
	check := func(x, y, width int, red, green, blue uint32) {
		t.Helper()
		for col := x; col < x+width-1; col++ {
			cell := buf.CellAt(col, y)
			if cell == nil || cell.Style.Bg == nil {
				t.Fatalf("unfilled highlight at %d,%d", col, y)
			}
			r, g, b, _ := cell.Style.Bg.RGBA()
			if r>>8 != red || g>>8 != green || b>>8 != blue {
				t.Fatalf("wrong background at %d,%d", col, y)
			}
		}
		if cell := buf.CellAt(x+width-1, y); cell == nil || cell.Content != "│" || cell.Style.Bg != nil {
			t.Fatal("highlight covered separator")
		}
	}
	check(0, 2, left, 0x20, 0x33, 0x40)
	check(left, 2+m.mainRows[m.selected].Start, center, 0x19, 0x3d, 0x48)
	check(left+center, 2+m.sideRows[m.threadSelected].Start, right, 0x19, 0x3d, 0x48)
	// Image rows must keep both the full-width selection and encoded image colors.
	check(left, 2+m.mainRows[0].Images[0].y, center, 0x19, 0x3d, 0x48)
}

func TestViewerDownloadsOriginalAndKeepsThumbnailSeparate(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2048, 1024))); err != nil {
		t.Fatal(err)
	}
	gets := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/connect/chatto.api.v1.AssetService/GetAsset":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["roomId"] != "room" || in["assetId"] != "a" || in["thumbnail"] != nil {
				t.Error("original request used thumbnail transform")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"asset":{"id":"a","contentType":"image/png","assetUrl":{"url":"/original"},"thumbnailAssetUrl":{"url":"/thumbnail"}}}`))
		case "/original":
			gets++
			if r.Header.Get("Authorization") != "" {
				t.Error("bearer token sent with signed download")
			}
			_, _ = w.Write(data.Bytes())
		default:
			t.Error("viewer fetched thumbnail instead of original")
			w.WriteHeader(404)
		}
	}))
	defer s.Close()
	m := imageModel()
	m.e.state.Session = chatto.HumanSession{Origin: s.URL, AccessToken: "token", AccessExpiresAt: time.Now().Add(time.Hour)}
	m.openImage("room/a", "sample.png")
	cmd := m.syncImages()
	m.renderImageViewer()
	if cmd == nil || !strings.Contains(m.View().Content, "Loading original") {
		t.Fatal("viewer did not load original on demand")
	}
	result := cmd().(imageLoaded)
	if result.err != nil || result.width != 2048 || result.height != 1024 || gets != 1 {
		t.Fatal("original resolution lost", result.err)
	}
	m.Update(result)
	if m.images.original.width != 2048 || m.images.entries["room/a"].width != 320 {
		t.Fatal("original replaced timeline thumbnail")
	}
	if !strings.Contains(m.View().Content, "2048 × 1024") {
		t.Fatal("viewer did not upgrade to original")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.images.original != nil {
		t.Fatal("closed viewer retained original")
	}
	m.openImage("room/a", "sample.png")
	pending := m.syncImages()
	newID := m.images.original.id
	m.acceptImage(result)
	if !m.images.original.loading || m.images.original.id != newID {
		t.Fatal("old result replaced new request")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	cancelled := pending().(imageLoaded)
	if cancelled.err == nil || cancelled.packet != "" {
		t.Fatal("closed viewer continued original download")
	}
	m.Update(cancelled)
	if m.images.original != nil || m.images.expanded != "" {
		t.Fatal("late result reopened viewer")
	}
}

func TestOriginalFailureKeepsVisibleThumbnail(t *testing.T) {
	m := imageModel()
	m.openImage("room/a", "sample.png")
	m.syncImages()
	id := m.images.original.id
	m.acceptImage(imageLoaded{original: true, key: "room/a", epoch: m.images.epoch, id: id, err: http.ErrNotSupported})
	m.renderImageViewer()
	if !strings.Contains(m.View().Content, "Original unavailable") || !strings.Contains(m.View().Content, string(kitty.Placeholder)) {
		t.Fatal("failed original hid thumbnail or failed silently")
	}
	m.images.clear()
}

func TestImageCellsSurviveSelectionClippingAndResize(t *testing.T) {
	m := imageModel()
	for _, width := range []int{55, 120, 180} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 32})
		for _, expanded := range []bool{false, true} {
			if expanded {
				m.images.expanded = "room/a"
			} else {
				m.images.expanded = ""
			}
			m.renderMessages()
			m.renderImageViewer()
			m.main.SetYOffset(0)
			view := m.View().Content
			buf := uv.NewScreenBuffer(width, 32)
			uv.NewStyledString(view).Draw(buf, buf.Bounds())
			count := 0
			for y := 0; y < 32; y++ {
				for x := 0; x < width; x++ {
					cell := buf.CellAt(x, y)
					if cell == nil || !strings.HasPrefix(cell.Content, string(kitty.Placeholder)) {
						continue
					}
					count++
					if cell.Width != 1 || len([]rune(cell.Content)) != 3 {
						t.Fatal("placeholder lost its coordinates or cell width")
					}
					if cell.Style.Fg == nil || cell.Style.UnderlineColor == nil {
						t.Fatal("image or placement ID lost during rendering")
					}
					r, g, b, _ := cell.Style.Fg.RGBA()
					if r>>8 != 0x12 || g>>8 != 0x34 || b>>8 != 0x56 {
						t.Fatal("selection overwrote image ID")
					}
				}
			}
			if count == 0 {
				t.Fatal("no image survived layout")
			}
			if ansi.StringWidth(strings.Split(view, "\n")[0]) > width {
				t.Fatal("layout overflow")
			}
		}
	}
	if strings.Contains(clean(string(kitty.Placeholder)), string(kitty.Placeholder)) {
		t.Fatal("remote text can spoof image cells")
	}
}

func TestImageDecodeBoundsAndPackets(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1024, 512))); err != nil {
		t.Fatal(err)
	}
	packet, w, h, err := decodePreview(data.Bytes(), 42)
	if err != nil || w != 768 || h != 384 || !strings.Contains(packet, "i=42") || !strings.Contains(packet, "q=2") {
		t.Fatal("image was not bounded/transmitted", err)
	}
	if _, _, _, err := decodePreview([]byte("<svg>not an image</svg>"), 42); err == nil {
		t.Fatal("unsupported image accepted")
	}
}

func TestImageInvalidationDropsLateResultsAndDeletesOnlyOwnedIDs(t *testing.T) {
	m := imageModel()
	m.images.raw.Reset()
	m.images.clear()
	packet := m.images.raw.String()
	if !strings.Contains(packet, "d=I,i=1193046") || strings.Contains(packet, "d=A") {
		t.Fatal("image cleanup must be scoped")
	}
	m.acceptImage(imageLoaded{key: "room/a", id: 0x123456, epoch: 0, packet: "STALE"})
	if strings.Contains(m.images.raw.String(), "STALE") || len(m.images.entries) != 0 {
		t.Fatal("late download restored invalidated image")
	}
	m.renderMessages()
	if strings.Contains(m.View().Content, string(kitty.Placeholder)) {
		t.Fatal("invalidated image still visible")
	}
}

func TestImageFallbackAndExpandedEscape(t *testing.T) {
	m := imageModel()
	m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModAlt})
	if m.images.expanded != "room/a" {
		t.Fatal("Alt+V did not expand selected image")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.images.expanded != "" || m.focus != 1 {
		t.Fatal("Escape must close preview first")
	}
	m.images.enabled = false
	m.renderMessages()
	view := m.View().Content
	if !strings.Contains(view, "sample.png") || strings.Contains(view, string(kitty.Placeholder)) {
		t.Fatal("text fallback missing")
	}
}

func TestClickThumbnailOpensExactAttachmentInScrolledPanes(t *testing.T) {
	for _, thread := range []bool{false, true} {
		m := imageModel()
		m.page.Messages[0].Attachments = append(m.page.Messages[0].Attachments,
			chatto.Attachment{ID: "second", Filename: "portrait.png", ContentType: "image/png"})
		m.images.entries["room/second"] = &terminalImage{id: 0x123457, width: 100, height: 400, placements: map[int]bool{}}
		m.page.Messages = append(m.page.Messages, chatto.Message{ID: "later", Body: strings.Repeat("Later message\n", 30)})
		if thread {
			m.root = "m"
			m.thread = m.page
			m.focus = 2
		}
		m.layout()
		rows, viewport := m.mainRows, &m.main
		left, center, _ := m.columns()
		paneX := left
		if thread {
			rows, viewport = m.sideRows, &m.side
			paneX += center
		}
		hit := rows[0].Images[1]
		viewport.SetYOffset(hit.y)
		// Click a visible cell inside the second image, after scrolling.
		m.Update(tea.MouseClickMsg{X: paneX + 1 + hit.x, Y: 2 + hit.y - viewport.YOffset(), Button: tea.MouseLeft})
		if m.images.expanded != "room/second" || !strings.Contains(m.View().Content, "portrait.png") {
			t.Fatal("thumbnail click did not open the exact image")
		}
		if strings.Contains(m.View().Content, "CONVERSATIONS") {
			t.Fatal("viewer remained constrained to the timeline pane")
		}
		m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		m.Update(tea.PasteMsg{Content: "hidden paste"})
		if m.compose.Value() != "" {
			t.Fatal("viewer input changed the hidden draft")
		}
		m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
		if m.images.expanded != "" || viewport.YOffset() == 0 {
			t.Fatal("closing viewer lost scroll position")
		}
	}
}

func TestImageFitUsesTerminalCellPixels(t *testing.T) {
	c := imageCache{cellWidth: 10, cellHeight: 20}
	for _, test := range []struct{ width, height, cols, rows int }{{400, 100, 32, 4}, {100, 400, 4, 8}, {100, 100, 16, 8}} {
		cols, rows := c.imageDimensions(&terminalImage{width: test.width, height: test.height}, 32, 8)
		if cols != test.cols || rows != test.rows {
			t.Fatalf("incorrect aspect: got %dx%d, want %dx%d", cols, rows, test.cols, test.rows)
		}
	}
	m := imageModel()
	m.Update(uv.CellSizeEvent{Width: 12, Height: 24})
	if m.images.cellWidth != 12 || m.images.cellHeight != 24 {
		t.Fatal("cell size report ignored")
	}
	m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if m.images.expanded != "room/a" {
		t.Fatal("Ctrl+O did not open image")
	}
	before := strings.Count(m.View().Content, string(kitty.Placeholder))
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 48})
	if strings.Count(m.View().Content, string(kitty.Placeholder)) <= before {
		t.Fatal("viewer did not resize")
	}
}

func TestImageRequestsAreVisibleBoundedAndCancelledOnRoomChange(t *testing.T) {
	m := exampleModel()
	m.images.enabled = true
	for i := range 4 {
		m.page.Messages[0].Attachments = append(m.page.Messages[0].Attachments, chatto.Attachment{ID: string(rune('a' + i)), Filename: "sample.png", ContentType: "image/png"})
	}
	m.renderMessages()
	cmd := m.syncImages()
	if cmd == nil || len(m.images.entries) != 2 {
		t.Fatal("image requests not limited to two")
	}
	oldEpoch := m.images.epoch
	m.room = "other"
	m.page = chatto.TimelinePage{}
	m.renderMessages()
	m.syncImages()
	if len(m.images.entries) != 0 || m.images.epoch == oldEpoch {
		t.Fatal("room change retained pending images")
	}
}
