package assets

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"io"

	"github.com/HugoSmits86/nativewebp"
	"github.com/disintegration/imageorient"
	xdraw "golang.org/x/image/draw"
)

// Default values for asset processing
const (
	// DefaultMaxUploadSize is the default maximum size for uploaded files (25 MB).
	DefaultMaxUploadSize int64 = 25 * 1024 * 1024
	// MaxAvatarDim is the maximum dimension for avatar images.
	MaxAvatarDim = 256
	// MaxLogoDim is the maximum dimension for space logo images.
	MaxLogoDim = 512
	// MaxBannerWidth is the maximum width for space banner images (4:3 aspect ratio).
	MaxBannerWidth = 768
	// MaxBannerHeight is the maximum height for space banner images (4:3 aspect ratio).
	MaxBannerHeight = 576
	// DefaultTransformJPEGQuality is the JPEG quality used by transformed images
	// unless the caller selects a surface-specific quality (1-100).
	// 80 provides a good balance between file size and visual quality.
	DefaultTransformJPEGQuality = 80
)

// Config holds configuration for asset processing.
type Config struct {
	// MaxUploadSize is the maximum size for uploaded files in bytes.
	MaxUploadSize int64
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() Config {
	return Config{
		MaxUploadSize: DefaultMaxUploadSize,
	}
}

// FitMode defines how an image should be fitted within the target dimensions.
type FitMode string

const (
	// FitContain fits the image within bounds while preserving aspect ratio.
	// The entire image will be visible, with possible letterboxing.
	FitContain FitMode = "contain"

	// FitCover fills the entire bounds while preserving aspect ratio.
	// The image is center-cropped if necessary.
	FitCover FitMode = "cover"

	// FitExact stretches the image to exactly match the target dimensions.
	// This may distort the image if the aspect ratio differs.
	FitExact FitMode = "exact"
)

// TransformResult holds the result of an image transformation.
type TransformResult struct {
	// Reader provides the transformed image data
	Reader io.Reader
	// ContentType is the MIME type of the output ("image/webp" or "image/jpeg")
	ContentType string
}

// TransformOptions controls image derivative encoding.
type TransformOptions struct {
	// JPEGQuality is used for opaque static images. It must be between 1 and 100.
	JPEGQuality int
}

// DetectImageContentType returns the MIME type of image data based on magic bytes.
// Returns "image/jpeg" for JPEG, "image/webp" for WebP, "image/gif" for GIF,
// "image/png" for PNG, or "application/octet-stream" if unrecognized.
func DetectImageContentType(data []byte) string {
	switch {
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg"
	case len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	case len(data) >= 3 && string(data[0:3]) == "GIF":
		return "image/gif"
	case len(data) >= 4 && data[0] == 0x89 && string(data[1:4]) == "PNG":
		return "image/png"
	default:
		return "application/octet-stream"
	}
}

// IsAnimatedGIF checks if the given bytes represent an animated GIF (more than 1 frame).
func IsAnimatedGIF(data []byte) bool {
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return false
	}
	return len(g.Image) > 1
}

// ProcessAvatarImage reads an image from the input reader, resizes it to fit
// within the configured max dimensions while maintaining aspect ratio, and
// encodes it as WebP. Uses default config values.
func ProcessAvatarImage(input io.Reader) (io.Reader, error) {
	return ProcessAvatarImageWithConfig(input, DefaultConfig())
}

// ProcessAvatarImageWithConfig reads an image from the input reader, resizes it to fit
// within MaxAvatarDim x MaxAvatarDim while maintaining aspect ratio, and
// encodes it as WebP. Returns an error if the input exceeds cfg.MaxUploadSize.
func ProcessAvatarImageWithConfig(input io.Reader, cfg Config) (io.Reader, error) {
	// Limit input size to prevent memory exhaustion
	limitedReader := io.LimitReader(input, cfg.MaxUploadSize+1)

	// Decode the input image (auto-detects format, applies EXIF orientation)
	img, _, err := imageorient.Decode(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Resize if necessary
	resized := resizeToFit(img, MaxAvatarDim, MaxAvatarDim)

	// Encode to WebP (lossless)
	var buf bytes.Buffer
	if err := nativewebp.Encode(&buf, resized, nil); err != nil {
		return nil, fmt.Errorf("failed to encode to webp: %w", err)
	}

	return bytes.NewReader(buf.Bytes()), nil
}

// ProcessLogoImage reads an image from the input reader, resizes it to fit
// within MaxLogoDim x MaxLogoDim while maintaining aspect ratio, and
// encodes it as WebP. Uses default config values.
func ProcessLogoImage(input io.Reader) (io.Reader, error) {
	return ProcessLogoImageWithConfig(input, DefaultConfig())
}

// ProcessLogoImageWithConfig reads an image from the input reader, resizes it to fit
// within MaxLogoDim x MaxLogoDim while maintaining aspect ratio, and
// encodes it as WebP. Returns an error if the input exceeds cfg.MaxUploadSize.
func ProcessLogoImageWithConfig(input io.Reader, cfg Config) (io.Reader, error) {
	// Limit input size to prevent memory exhaustion
	limitedReader := io.LimitReader(input, cfg.MaxUploadSize+1)

	// Decode the input image (auto-detects format, applies EXIF orientation)
	img, _, err := imageorient.Decode(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Resize if necessary
	resized := resizeToFit(img, MaxLogoDim, MaxLogoDim)

	// Encode to WebP (lossless)
	var buf bytes.Buffer
	if err := nativewebp.Encode(&buf, resized, nil); err != nil {
		return nil, fmt.Errorf("failed to encode to webp: %w", err)
	}

	return bytes.NewReader(buf.Bytes()), nil
}

// ProcessBannerImage reads an image from the input reader, resizes it to fit
// within MaxBannerWidth x MaxBannerHeight while maintaining aspect ratio, and
// encodes it as WebP. Uses default config values.
func ProcessBannerImage(input io.Reader) (io.Reader, error) {
	return ProcessBannerImageWithConfig(input, DefaultConfig())
}

// ProcessBannerImageWithConfig reads an image from the input reader, resizes it to fit
// within MaxBannerWidth x MaxBannerHeight while maintaining aspect ratio, and
// encodes it as WebP. Returns an error if the input exceeds cfg.MaxUploadSize.
func ProcessBannerImageWithConfig(input io.Reader, cfg Config) (io.Reader, error) {
	// Limit input size to prevent memory exhaustion
	limitedReader := io.LimitReader(input, cfg.MaxUploadSize+1)

	// Decode the input image (auto-detects format, applies EXIF orientation)
	img, _, err := imageorient.Decode(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Resize if necessary
	resized := resizeToFit(img, MaxBannerWidth, MaxBannerHeight)

	// Encode to WebP (lossless)
	var buf bytes.Buffer
	if err := nativewebp.Encode(&buf, resized, nil); err != nil {
		return nil, fmt.Errorf("failed to encode to webp: %w", err)
	}

	return bytes.NewReader(buf.Bytes()), nil
}

// MaxLinkPreviewWidth is the maximum width for link preview OG images.
// Standard OG images are 1200x630; we preserve that resolution for sharp display on 2x screens.
const MaxLinkPreviewWidth = 1200

// MaxLinkPreviewHeight is the maximum height for link preview OG images.
const MaxLinkPreviewHeight = 630

// ProcessLinkPreviewImageWithConfig reads an image from the input reader, resizes it to fit
// within MaxLinkPreviewWidth x MaxLinkPreviewHeight while maintaining aspect ratio, and
// encodes it as WebP. Returns an error if the input exceeds cfg.MaxUploadSize.
func ProcessLinkPreviewImageWithConfig(input io.Reader, cfg Config) (io.Reader, error) {
	// Limit input size to prevent memory exhaustion
	limitedReader := io.LimitReader(input, cfg.MaxUploadSize+1)

	// Decode the input image (auto-detects format, applies EXIF orientation)
	img, _, err := imageorient.Decode(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Resize if necessary
	resized := resizeToFit(img, MaxLinkPreviewWidth, MaxLinkPreviewHeight)

	// Encode to WebP (lossless)
	var buf bytes.Buffer
	if err := nativewebp.Encode(&buf, resized, nil); err != nil {
		return nil, fmt.Errorf("failed to encode to webp: %w", err)
	}

	return bytes.NewReader(buf.Bytes()), nil
}

// AttachmentImageResult holds the processed attachment image data.
type AttachmentImageResult struct {
	// Original contains the original image bytes (unchanged)
	Original []byte
	// Width of the original image
	Width int
	// Height of the original image
	Height int
}

// ProcessAttachmentImage reads an image and extracts metadata (dimensions).
// The original image is returned as-is (not re-encoded).
// Thumbnails are generated on-the-fly via the transform system.
// Uses default config values.
func ProcessAttachmentImage(input io.Reader) (*AttachmentImageResult, error) {
	return ProcessAttachmentImageWithConfig(input, DefaultConfig())
}

// ProcessAttachmentImageWithConfig reads an image and extracts metadata (dimensions).
// The original image is returned as-is (not re-encoded).
// Thumbnails are generated on-the-fly via the transform system.
// Returns an error if the input exceeds cfg.MaxUploadSize or cannot be decoded.
func ProcessAttachmentImageWithConfig(input io.Reader, cfg Config) (*AttachmentImageResult, error) {
	// Read all input into memory (limited to MaxUploadSize)
	limitedReader := io.LimitReader(input, cfg.MaxUploadSize+1)
	originalBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}
	if int64(len(originalBytes)) > cfg.MaxUploadSize {
		return nil, fmt.Errorf("image exceeds maximum size of %d bytes", cfg.MaxUploadSize)
	}

	// Decode the image to get dimensions (applies EXIF orientation for correct dimensions)
	img, _, err := imageorient.Decode(bytes.NewReader(originalBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	bounds := img.Bounds()

	return &AttachmentImageResult{
		Original: originalBytes,
		Width:    bounds.Dx(),
		Height:   bounds.Dy(),
	}, nil
}

// resizeToFit resizes an image to fit within the specified maximum width and height
// while maintaining aspect ratio. If the image is already smaller, it's returned as-is.
func resizeToFit(img image.Image, maxWidth, maxHeight int) image.Image {
	bounds := img.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	// If image is already within bounds, return as-is
	if srcWidth <= maxWidth && srcHeight <= maxHeight {
		return img
	}

	// Calculate the scaling factor to fit within bounds
	widthRatio := float64(maxWidth) / float64(srcWidth)
	heightRatio := float64(maxHeight) / float64(srcHeight)
	ratio := widthRatio
	if heightRatio < widthRatio {
		ratio = heightRatio
	}

	// Calculate new dimensions
	newWidth := int(float64(srcWidth) * ratio)
	newHeight := int(float64(srcHeight) * ratio)

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	// Use high-quality CatmullRom interpolation for resizing
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, xdraw.Over, nil)

	return dst
}

// hasTransparency checks if an image contains any non-opaque pixels.
func hasTransparency(img image.Image) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a < 0xffff {
				return true
			}
		}
	}
	return false
}

// TransformImage transforms an image according to the specified dimensions and fit mode.
// For animated GIFs, converts to animated WebP with proper frame compositing.
// For images with transparency, returns WebP to preserve alpha.
// For opaque images, returns JPEG for smaller file sizes.
func TransformImage(data []byte, width, height int, fit FitMode) (*TransformResult, error) {
	return TransformImageWithOptions(data, width, height, fit, TransformOptions{
		JPEGQuality: DefaultTransformJPEGQuality,
	})
}

// TransformImageWithOptions transforms an image with explicit encoding options.
func TransformImageWithOptions(data []byte, width, height int, fit FitMode, options TransformOptions) (*TransformResult, error) {
	if options.JPEGQuality < 1 || options.JPEGQuality > 100 {
		return nil, fmt.Errorf("invalid JPEG quality: %d", options.JPEGQuality)
	}

	// Check if this is an animated GIF - handle specially to preserve animation
	if IsAnimatedGIF(data) {
		return transformAnimatedGIF(data, width, height, fit)
	}

	// Decode the input image (applies EXIF orientation)
	img, _, err := imageorient.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	var transformed image.Image

	switch fit {
	case FitContain:
		// Fit within bounds, preserve aspect ratio (same as resizeToFit)
		transformed = resizeToFit(img, width, height)

	case FitCover:
		// Fill bounds and center-crop excess
		transformed = resizeToCover(img, width, height)

	case FitExact:
		// Stretch to exact dimensions
		transformed = resizeToExact(img, width, height)

	default:
		return nil, fmt.Errorf("invalid fit mode: %s", fit)
	}

	// Encode to WebP if the image has transparency (JPEG doesn't support alpha),
	// otherwise use JPEG for smaller file sizes.
	var buf bytes.Buffer
	if hasTransparency(transformed) {
		if err := nativewebp.Encode(&buf, transformed, nil); err != nil {
			return nil, fmt.Errorf("failed to encode to webp: %w", err)
		}
		return &TransformResult{
			Reader:      bytes.NewReader(buf.Bytes()),
			ContentType: "image/webp",
		}, nil
	}

	if err := jpeg.Encode(&buf, transformed, &jpeg.Options{Quality: options.JPEGQuality}); err != nil {
		return nil, fmt.Errorf("failed to encode to jpeg: %w", err)
	}

	return &TransformResult{
		Reader:      bytes.NewReader(buf.Bytes()),
		ContentType: "image/jpeg",
	}, nil
}

// transformAnimatedGIF converts an animated GIF to animated WebP with proper
// frame compositing. GIF frames can be partial sub-rectangles with disposal
// methods that control how the canvas is updated between frames. This function
// composites frames correctly onto a running canvas before resizing, avoiding
// the palette destruction and compositing bugs inherent in frame-by-frame GIF
// resizing.
func transformAnimatedGIF(data []byte, width, height int, fit FitMode) (*TransformResult, error) {
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode animated GIF: %w", err)
	}

	if len(g.Image) == 0 {
		return nil, fmt.Errorf("GIF has no frames")
	}

	// Use Config dimensions (the logical canvas size), not first frame bounds
	// which may be a sub-rectangle.
	canvasWidth := g.Config.Width
	canvasHeight := g.Config.Height
	if canvasWidth == 0 || canvasHeight == 0 {
		canvasWidth = g.Image[0].Bounds().Max.X
		canvasHeight = g.Image[0].Bounds().Max.Y
	}

	// Calculate target dimensions
	var newWidth, newHeight int
	switch fit {
	case FitContain:
		if canvasWidth <= width && canvasHeight <= height {
			newWidth, newHeight = canvasWidth, canvasHeight
		} else {
			widthRatio := float64(width) / float64(canvasWidth)
			heightRatio := float64(height) / float64(canvasHeight)
			ratio := widthRatio
			if heightRatio < widthRatio {
				ratio = heightRatio
			}
			newWidth = int(float64(canvasWidth) * ratio)
			newHeight = int(float64(canvasHeight) * ratio)
		}
	case FitCover, FitExact:
		newWidth, newHeight = width, height
	default:
		return nil, fmt.Errorf("invalid fit mode: %s", fit)
	}

	// Composite GIF frames onto a running canvas, handling disposal methods
	composited := compositeGIFFrames(g)

	// Resize each composited frame and build WebP animation
	needsResize := newWidth != canvasWidth || newHeight != canvasHeight
	webpFrames := make([]image.Image, len(composited))

	for i, frame := range composited {
		if needsResize {
			resized := image.NewNRGBA(image.Rect(0, 0, newWidth, newHeight))
			xdraw.CatmullRom.Scale(resized, resized.Bounds(), frame, frame.Bounds(), xdraw.Over, nil)
			webpFrames[i] = resized
		} else {
			webpFrames[i] = frame
		}
	}

	// Convert GIF timing and loop semantics to WebP
	durations := make([]uint, len(g.Image))
	disposals := make([]uint, len(g.Image))
	for i := range g.Image {
		delay := 0
		if i < len(g.Delay) {
			delay = g.Delay[i]
		}
		durations[i] = uint(delay) * 10 // centiseconds → milliseconds
		disposals[i] = 0                // keep — compositing already resolved
	}

	ani := &nativewebp.Animation{
		Images:          webpFrames,
		Durations:       durations,
		Disposals:       disposals,
		LoopCount:       convertGIFLoopCount(g.LoopCount),
		BackgroundColor: 0, // transparent
	}

	var buf bytes.Buffer
	if err := nativewebp.EncodeAll(&buf, ani, nil); err != nil {
		return nil, fmt.Errorf("failed to encode animated WebP: %w", err)
	}

	return &TransformResult{
		Reader:      bytes.NewReader(buf.Bytes()),
		ContentType: "image/webp",
	}, nil
}

// compositeGIFFrames composites GIF frames onto a running canvas, correctly
// handling disposal methods and sub-rectangle frames. Returns full-canvas
// NRGBA snapshots for each frame, ready for resizing and WebP encoding.
func compositeGIFFrames(g *gif.GIF) []*image.NRGBA {
	canvasWidth := g.Config.Width
	canvasHeight := g.Config.Height
	if canvasWidth == 0 || canvasHeight == 0 {
		canvasWidth = g.Image[0].Bounds().Max.X
		canvasHeight = g.Image[0].Bounds().Max.Y
	}

	canvas := image.NewNRGBA(image.Rect(0, 0, canvasWidth, canvasHeight))
	var previous *image.NRGBA
	result := make([]*image.NRGBA, len(g.Image))

	for i, frame := range g.Image {
		disposal := byte(0)
		if g.Disposal != nil && i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}

		// For DisposalPrevious, snapshot canvas BEFORE drawing this frame
		if disposal == gif.DisposalPrevious {
			previous = image.NewNRGBA(canvas.Bounds())
			copy(previous.Pix, canvas.Pix)
		}

		// Draw frame onto canvas (Over respects GIF transparent index)
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)

		// Snapshot the composited canvas for this frame
		result[i] = image.NewNRGBA(canvas.Bounds())
		copy(result[i].Pix, canvas.Pix)

		// Apply disposal for the next frame
		switch disposal {
		case gif.DisposalBackground:
			// Clear the frame's rectangle to transparent
			draw.Draw(canvas, frame.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			// Restore canvas from snapshot taken before this frame
			if previous != nil {
				copy(canvas.Pix, previous.Pix)
			}
			// DisposalNone (0x01) and unspecified (0x00): leave canvas as-is
		}
	}

	return result
}

// convertGIFLoopCount converts a GIF loop count to a WebP loop count.
// GIF: 0=forever, -1=once, N=play N+1 times.
// WebP: 0=infinite, N=play N times.
func convertGIFLoopCount(gifLoop int) uint16 {
	switch {
	case gifLoop == 0:
		return 0 // infinite
	case gifLoop < 0:
		return 1 // play once
	default:
		n := gifLoop + 1
		if n > 65535 {
			n = 65535
		}
		return uint16(n)
	}
}

// resizeToCover resizes and crops an image to fill the target dimensions while
// preserving aspect ratio. The image is center-cropped if necessary.
func resizeToCover(img image.Image, targetWidth, targetHeight int) image.Image {
	bounds := img.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	// Calculate the scaling factor to cover the entire target area
	widthRatio := float64(targetWidth) / float64(srcWidth)
	heightRatio := float64(targetHeight) / float64(srcHeight)
	ratio := widthRatio
	if heightRatio > widthRatio {
		ratio = heightRatio
	}

	// Calculate scaled dimensions (will be >= target dimensions)
	scaledWidth := int(float64(srcWidth) * ratio)
	scaledHeight := int(float64(srcHeight) * ratio)

	// Create intermediate scaled image
	scaled := image.NewRGBA(image.Rect(0, 0, scaledWidth, scaledHeight))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), img, bounds, xdraw.Over, nil)

	// Calculate crop offset to center the image
	cropX := (scaledWidth - targetWidth) / 2
	cropY := (scaledHeight - targetHeight) / 2

	// Create destination image with target dimensions
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))

	// Copy the center portion
	draw.Draw(
		dst,
		dst.Bounds(),
		scaled,
		image.Point{X: cropX, Y: cropY},
		draw.Src,
	)

	return dst
}

// resizeToExact stretches an image to exactly match the target dimensions,
// potentially distorting the aspect ratio.
func resizeToExact(img image.Image, targetWidth, targetHeight int) image.Image {
	bounds := img.Bounds()

	// Create destination image with exact target dimensions
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))

	// Scale to exact dimensions using CatmullRom interpolation
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, xdraw.Over, nil)

	return dst
}
