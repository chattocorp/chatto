package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// Images remain in memory and in this terminal only. Virtual Kitty placements
// follow Unicode cells through Bubble Tea's normal clipping and scroll updates.
type imageCache struct {
	enabled               bool
	room                  string
	epoch                 int
	nextID                int
	entries               map[string]*terminalImage
	raw                   strings.Builder
	expanded              string // attachment cache key shown in the full-screen viewer
	preview               string
	previewTitle          string
	cellWidth, cellHeight int
	original              *terminalImage // one high-resolution image, retained only while its viewer is open
	originalKey           string
}

type imageHit struct {
	x, y, width, height int // coordinates in the timeline content, before viewport clipping
	key, title          string
}

type terminalImage struct {
	id, width, height int
	loading, failed   bool
	cancel            context.CancelFunc
	placements        map[int]bool
}

type imageLoaded struct {
	original                 bool
	key                      string
	epoch, id, width, height int
	packet                   string
	err                      error
}

func automaticImages() bool {
	// Multiplexers need explicit passthrough support; leave them in text mode.
	return os.Getenv("TMUX") == "" && os.Getenv("STY") == "" &&
		(os.Getenv("TERM_PROGRAM") == "ghostty" || os.Getenv("TERM") == "xterm-ghostty" || os.Getenv("KITTY_WINDOW_ID") != "")
}

func (c *imageCache) allocateID() int {
	if c.nextID == 0 {
		var b [4]byte
		_, _ = rand.Read(b[:])
		c.nextID = int(binary.BigEndian.Uint32(b[:])&0x7fffff) + 256
	}
	c.nextID = c.nextID%0xffffff + 1
	return c.nextID
}

func imageKey(room, id string) string { return room + "/" + id }

// Avatar identity includes the URL revision without retaining it in cache keys.
func avatarKey(id, address string) string {
	return fmt.Sprintf("avatar:%s:%x", id, sha256.Sum256([]byte(address)))
}

type requestedImage struct{ asset, avatar string }

func (c *imageCache) remove(key string) {
	entry := c.entries[key]
	if entry.cancel != nil {
		entry.cancel()
	}
	if !entry.loading && !entry.failed {
		c.raw.WriteString(ansi.KittyGraphics(nil, "a=d", "d=I", fmt.Sprintf("i=%d", entry.id), "q=2"))
	}
	delete(c.entries, key)
}

func (c *imageCache) clear() {
	c.clearOriginal()
	for key := range c.entries {
		c.remove(key)
	}
	c.epoch++
	c.expanded = ""
}

func (c *imageCache) clearOriginal() {
	if entry := c.original; entry != nil {
		if entry.cancel != nil {
			entry.cancel()
		}
		if !entry.loading && !entry.failed {
			c.raw.WriteString(ansi.KittyGraphics(nil, "a=d", "d=I", fmt.Sprintf("i=%d", entry.id), "q=2"))
		}
	}
	c.original, c.originalKey = nil, ""
}

func (e *engine) imagePreview(ctx context.Context, room, asset string) ([]byte, error) {
	return e.downloadImage(ctx, room, asset, false)
}

func (e *engine) downloadImage(ctx context.Context, room, asset string, original bool) ([]byte, error) {
	e.mu.Lock()
	c, err := e.client()
	e.mu.Unlock()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	options := chatto.AssetDownloadOptions{MaxBytes: 32 << 20}
	if !original {
		options.MaxBytes = 8 << 20
		options.Thumbnail = &chatto.ImageTransform{Width: 1024, Height: 1024, Fit: "IMAGE_FIT_MODE_CONTAIN"}
	}
	return c.DownloadAsset(ctx, room, asset, options)
}

// decodePreview bounds allocation before decoding and bounds terminal storage
// after decoding. Animation uses the first frame; SVG is not executed/rendered.
func decodePreview(data []byte, id int) (string, int, int, error) {
	return decodeImage(data, id, 768, 16_000_000)
}

func decodeImage(data []byte, id, maxSide int, maxPixels int64) (string, int, int, error) {
	return decodeTerminalImage(data, id, maxSide, maxPixels, false)
}

func decodeTerminalImage(data []byte, id, maxSide int, maxPixels int64, avatar bool) (string, int, int, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return "", 0, 0, fmt.Errorf("unsupported or oversized image")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", 0, 0, fmt.Errorf("invalid image")
	}
	w, h := cfg.Width, cfg.Height
	if avatar {
		img = circularAvatar(img, maxSide)
		w, h = img.Bounds().Dx(), img.Bounds().Dy()
	} else if max(w, h) > maxSide {
		w, h = max(1, w*maxSide/max(cfg.Width, cfg.Height)), max(1, h*maxSide/max(cfg.Width, cfg.Height))
		scaled := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), img, img.Bounds(), draw.Src, nil)
		img = scaled
	}
	var packet bytes.Buffer
	err = kitty.EncodeGraphics(&packet, img, &kitty.Options{ID: id, Action: kitty.Transmit, Transmission: kitty.Direct, Format: kitty.PNG, Quiet: 2, Chunk: true})
	return packet.String(), w, h, err
}

// circularAvatar center-crops before scaling, then applies a one-pixel soft
// circular edge. Multiply all premultiplied channels to preserve source alpha.
func circularAvatar(src image.Image, maxSide int) *image.RGBA {
	bounds := src.Bounds()
	side := min(bounds.Dx(), bounds.Dy())
	x, y := bounds.Min.X+(bounds.Dx()-side)/2, bounds.Min.Y+(bounds.Dy()-side)/2
	size := min(side, maxSide)
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, image.Rect(x, y, x+side, y+side), draw.Src, nil)
	radius := float64(size) / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			coverage := max(0, min(1, radius+0.5-math.Hypot(float64(x)+0.5-radius, float64(y)+0.5-radius)))
			i := dst.PixOffset(x, y)
			for c := 0; c < 4; c++ {
				dst.Pix[i+c] = uint8(math.Round(float64(dst.Pix[i+c]) * coverage))
			}
		}
	}
	return dst
}

// syncImages loads only visible messages, with two requests in flight and a
// bounded cache. It runs after input/state changes, never from View.
func (m *model) syncImages() tea.Cmd {
	c := &m.images
	if c.expanded != c.originalKey {
		c.clearOriginal()
	}
	if !c.enabled || !m.logged {
		return nil
	}
	if c.room != m.room {
		c.clear()
		c.room = m.room
		m.renderMessages()
	}
	if c.entries == nil {
		c.entries = map[string]*terminalImage{}
	}
	if c.expanded != "" {
		present := false
		for _, page := range []chatto.TimelinePage{m.page, m.thread} {
			for _, msg := range page.Messages {
				for _, a := range msg.Attachments {
					if imageKey(m.room, a.ID) == c.expanded {
						present = true
					}
				}
			}
		}
		if !present {
			c.expanded = ""
			c.clearOriginal()
		} else {
			// Upgrade the viewer without fetching or evicting background thumbnails.
			return m.loadOriginal()
		}
	}
	wanted := map[string]requestedImage{}
	_, center, right := m.columns()
	collect := func(p chatto.TimelinePage, rows []rowRange, offset, height int) {
		for i, row := range rows {
			if row.End <= offset || row.Start >= offset+height {
				continue
			}
			msg := p.Messages[i]
			for _, a := range msg.Attachments {
				if a.ID != "" && strings.HasPrefix(a.ContentType, "image/") {
					wanted[imageKey(m.room, a.ID)] = requestedImage{asset: a.ID}
				}
			}
			if msg.Thread != nil {
				for _, id := range msg.Thread.ParticipantPreviewUserIDs[:min(3, len(msg.Thread.ParticipantPreviewUserIDs))] {
					if u := m.users[id]; u.AvatarURL != "" {
						wanted[avatarKey(id, u.AvatarURL)] = requestedImage{avatar: u.AvatarURL}
					}
				}
			}
		}
	}
	if center > 0 {
		collect(m.page, m.mainRows, m.main.YOffset(), m.main.Height())
	}
	if right > 0 {
		collect(m.thread, m.sideRows, m.side.YOffset(), m.side.Height())
	}
	active := 0
	for key, entry := range c.entries {
		if _, visible := wanted[key]; !visible && entry.loading {
			c.remove(key)
			continue
		}
		if entry.loading {
			active++
		}
	}
	var cmds []tea.Cmd
	for key, a := range wanted {
		if c.entries[key] != nil || active >= 2 {
			continue
		}
		if len(c.entries) >= 16 {
			for old := range c.entries {
				if _, visible := wanted[old]; !visible && !c.entries[old].loading {
					c.remove(old)
					break
				}
			}
			if len(c.entries) >= 16 {
				break
			}
		}
		ctx, cancel := context.WithCancel(m.e.ctx)
		id, epoch, room, request := c.allocateID(), c.epoch, m.room, a
		c.entries[key] = &terminalImage{id: id, loading: true, cancel: cancel, placements: map[int]bool{}}
		active++
		cmds = append(cmds, func() tea.Msg {
			defer cancel()
			var data []byte
			var err error
			if request.avatar != "" {
				data, err = m.e.avatarPreview(ctx, request.avatar)
			} else {
				data, err = m.e.imagePreview(ctx, room, request.asset)
			}
			r := imageLoaded{key: key, epoch: epoch, id: id, err: err}
			if err == nil {
				if request.avatar != "" {
					r.packet, r.width, r.height, r.err = decodeTerminalImage(data, id, 64, 1_000_000, true)
				} else {
					r.packet, r.width, r.height, r.err = decodePreview(data, id)
				}
			}
			return r
		})
	}
	return tea.Batch(cmds...)
}

func (e *engine) avatarPreview(ctx context.Context, address string) ([]byte, error) {
	e.mu.Lock()
	c, err := e.client()
	e.mu.Unlock()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return c.DownloadAssetURL(ctx, address, 1<<20)
}

func (m *model) loadOriginal() tea.Cmd {
	c := &m.images
	if c.original != nil {
		return nil
	}
	key, room := c.expanded, m.room
	asset := strings.TrimPrefix(key, room+"/")
	id, epoch := c.allocateID(), c.epoch
	ctx, cancel := context.WithCancel(m.e.ctx)
	c.originalKey = key
	c.original = &terminalImage{id: id, loading: true, cancel: cancel, placements: map[int]bool{}}
	return func() tea.Msg {
		defer cancel()
		data, err := m.e.downloadImage(ctx, room, asset, true)
		r := imageLoaded{original: true, key: key, epoch: epoch, id: id, err: err}
		if err == nil && ctx.Err() == nil {
			r.packet, r.width, r.height, r.err = decodeImage(data, id, 4096, 32_000_000)
		}
		if ctx.Err() != nil {
			r.packet = ""
			r.err = ctx.Err()
		}
		return r
	}
}

func (m *model) acceptImage(v imageLoaded) {
	c := &m.images
	entry := c.entries[v.key]
	if v.original {
		if c.expanded != v.key || c.originalKey != v.key {
			return
		}
		entry = c.original
	}
	if v.epoch != c.epoch || entry == nil || entry.id != v.id {
		return
	}
	entry.loading, entry.failed = false, v.err != nil
	entry.width, entry.height = v.width, v.height
	if v.err == nil {
		c.raw.WriteString(v.packet)
	}
	if v.original {
		return
	}
	main := captureAnchor(m.page, m.mainRows, m.main.YOffset())
	side := captureAnchor(m.thread, m.sideRows, m.side.YOffset())
	followMain, followSide := m.main.AtBottom(), m.side.AtBottom()
	m.renderMessages()
	if followMain {
		m.main.GotoBottom()
	} else {
		restoreAnchor(&m.main, m.page, m.mainRows, main)
	}
	if followSide {
		m.side.GotoBottom()
	} else {
		restoreAnchor(&m.side, m.thread, m.sideRows, side)
	}
}

func (m *model) toggleImage() {
	if m.images.expanded != "" {
		m.images.expanded = ""
		return
	}
	p, index := m.page, m.selected
	if m.focus == 2 {
		p, index = m.thread, m.threadSelected
	}
	if index < 0 || index >= len(p.Messages) {
		return
	}
	for _, a := range p.Messages[index].Attachments {
		if m.openImage(imageKey(m.room, a.ID), a.Filename) {
			return
		}
	}
	m.status = "Select a loaded image to preview it"
}

func (m *model) openImage(key, title string) bool {
	entry := m.images.entries[key]
	if !m.images.enabled || entry == nil || entry.loading || entry.failed {
		return false
	}
	m.images.expanded, m.images.previewTitle = key, oneLine(title)
	m.dragSeparator = 0
	return true
}

// imageDimensions fits the source into terminal cells using their pixel size.
// Kitty preserves the exact pixel aspect within the final, rounded cell bounds.
func (c *imageCache) imageDimensions(entry *terminalImage, maxCols, maxRows int) (int, int) {
	cw, ch := c.cellWidth, c.cellHeight
	if cw <= 0 || ch <= 0 {
		cw, ch = 1, 2
	}
	maxCols, maxRows = max(1, min(256, maxCols)), max(1, min(128, maxRows))
	scale := math.Min(float64(maxCols*cw)/float64(entry.width), float64(maxRows*ch)/float64(entry.height))
	cols := min(maxCols, max(1, int(math.Ceil(float64(entry.width)*scale/float64(cw)))))
	rows := min(maxRows, max(1, int(math.Ceil(float64(entry.height)*scale/float64(ch)))))
	return cols, rows
}

func (c *imageCache) imageCells(entry *terminalImage, cols, rows int) []string {
	placement := cols*256 + rows
	if !entry.placements[placement] {
		entry.placements[placement] = true
		c.raw.WriteString(ansi.KittyGraphics(nil, "a=p", "U=1", "q=2", fmt.Sprintf("i=%d", entry.id), fmt.Sprintf("p=%d", placement), fmt.Sprintf("c=%d", cols), fmt.Sprintf("r=%d", rows)))
	}
	lines := make([]string, rows)
	for y := 0; y < rows; y++ {
		var line strings.Builder
		fmt.Fprintf(&line, "\x1b[38;2;%d;%d;%dm\x1b[58;2;%d;%d;%dm", entry.id>>16, entry.id>>8&255, entry.id&255, placement>>16, placement>>8&255, placement&255)
		for x := 0; x < cols; x++ {
			line.WriteRune(kitty.Placeholder)
			line.WriteRune(kitty.Diacritic(y))
			line.WriteRune(kitty.Diacritic(x))
		}
		line.WriteString("\x1b[39;59m")
		lines[y] = line.String()
	}
	return lines
}

func (m *model) renderImageViewer() {
	c := &m.images
	entry := c.entries[c.expanded]
	if c.expanded == "" {
		return
	}
	if entry == nil || entry.loading || entry.failed {
		c.expanded = ""
		c.clearOriginal()
		return
	}
	quality := "Loading original…"
	if original := c.original; original != nil {
		if original.failed {
			quality = "Original unavailable · showing thumbnail"
		} else if !original.loading {
			entry = original
			quality = fmt.Sprintf("%d × %d", entry.width, entry.height)
		}
	}
	cols, rows := c.imageDimensions(entry, m.width-4, m.height-4)
	body := strings.Join(c.imageCells(entry, cols, rows), "\n")
	c.preview = terminalBar("IMAGE / "+c.previewTitle, m.width, true) + "\n" +
		lipgloss.Place(m.width, max(1, m.height-2), lipgloss.Center, lipgloss.Center, body) + "\n" +
		terminalBar(quality+" · Esc / click to close", m.width, false)
}

// attachmentLines keeps image-ID foreground and placement-ID underline colors
// intact, including on selected rows. Never pass these cells through ansi.Strip.
func (m *model) attachmentLines(msg chatto.Message, width, gutter int) ([]string, []imageHit) {
	var lines []string
	var hits []imageHit
	indent := min(gutter, max(0, width-12))
	prefix := strings.Repeat(" ", indent)
	for _, a := range msg.Attachments {
		label := "▧ " + oneLine(a.Filename)
		if a.Filename == "" {
			label = "▧ Attachment"
		}
		entry := m.images.entries[imageKey(m.room, a.ID)]
		if entry != nil && entry.failed {
			label += " · preview unavailable"
		}
		lines = append(lines, prefix+ansi.Truncate(label, max(1, width-indent), "…"))
		if !m.images.enabled || entry == nil || entry.loading || entry.failed {
			continue
		}
		cols, rows := m.images.imageDimensions(entry, min(32, width-indent), 8)
		hits = append(hits, imageHit{x: indent, y: len(lines), width: cols, height: rows, key: imageKey(m.room, a.ID), title: a.Filename})
		for _, line := range m.images.imageCells(entry, cols, rows) {
			lines = append(lines, prefix+line)
		}
	}
	return lines, hits
}
