package assets

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"strings"
	"testing"

	_ "golang.org/x/image/webp" // Register WebP decoder for tests
)

// createTestImage creates a test PNG image with the specified dimensions.
func createTestImage(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with a gradient to make it interesting
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{
				R: uint8(x * 255 / width),
				G: uint8(y * 255 / height),
				B: 128,
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// createTransparentTestImage creates a test PNG image with transparency.
// A checkerboard pattern where half the pixels are fully transparent.
func createTransparentTestImage(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	for y := range height {
		for x := range width {
			if (x+y)%2 == 0 {
				img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
			} else {
				img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 0})
			}
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// createTestImageReader creates a test PNG image as io.Reader (for ProcessAvatarImage tests).
func createTestImageReader(width, height int) io.Reader {
	return bytes.NewReader(createTestImage(width, height))
}

// createAnimatedGIF creates a test animated GIF with the specified dimensions and frame count.
func createAnimatedGIF(width, height, frames int) []byte {
	g := &gif.GIF{
		Image: make([]*image.Paletted, frames),
		Delay: make([]int, frames),
	}

	palette := []color.Color{
		color.RGBA{0, 0, 0, 255},       // black
		color.RGBA{255, 0, 0, 255},     // red
		color.RGBA{0, 255, 0, 255},     // green
		color.RGBA{0, 0, 255, 255},     // blue
		color.RGBA{255, 255, 255, 255}, // white
	}

	for i := range frames {
		frame := image.NewPaletted(image.Rect(0, 0, width, height), palette)
		// Fill each frame with a different color
		colorIdx := uint8((i % 4) + 1)
		for y := range height {
			for x := range width {
				frame.SetColorIndex(x, y, colorIdx)
			}
		}
		g.Image[i] = frame
		g.Delay[i] = 10 // 100ms delay
	}

	var buf bytes.Buffer
	gif.EncodeAll(&buf, g)
	return buf.Bytes()
}

// createStaticGIF creates a single-frame GIF.
func createStaticGIF(width, height int) []byte {
	return createAnimatedGIF(width, height, 1)
}

func TestProcessAvatarImage_ValidPNG(t *testing.T) {
	input := createTestImageReader(100, 100)

	result, err := ProcessAvatarImage(input)
	if err != nil {
		t.Fatalf("ProcessAvatarImage() error = %v", err)
	}

	if result == nil {
		t.Fatal("ProcessAvatarImage() returned nil reader")
	}

	// Read the result and verify it's not empty
	data, err := io.ReadAll(result)
	if err != nil {
		t.Fatalf("Failed to read result: %v", err)
	}

	if len(data) == 0 {
		t.Error("ProcessAvatarImage() returned empty data")
	}

	// WebP files start with "RIFF" magic bytes
	if len(data) < 4 || string(data[0:4]) != "RIFF" {
		t.Error("ProcessAvatarImage() did not return WebP format")
	}
}

func TestProcessAvatarImage_ResizesLargeImage(t *testing.T) {
	// Create an image larger than MaxAvatarDim
	input := createTestImageReader(512, 512)

	result, err := ProcessAvatarImage(input)
	if err != nil {
		t.Fatalf("ProcessAvatarImage() error = %v", err)
	}

	// Decode the result to check dimensions
	data, _ := io.ReadAll(result)

	// Decode the WebP to verify dimensions
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode result: %v", err)
	}

	bounds := decoded.Bounds()
	if bounds.Dx() > MaxAvatarDim || bounds.Dy() > MaxAvatarDim {
		t.Errorf("Image not resized: got %dx%d, max should be %d",
			bounds.Dx(), bounds.Dy(), MaxAvatarDim)
	}
}

func TestProcessAvatarImage_PreservesSmallImage(t *testing.T) {
	// Create an image smaller than MaxAvatarDim
	input := createTestImageReader(64, 64)

	result, err := ProcessAvatarImage(input)
	if err != nil {
		t.Fatalf("ProcessAvatarImage() error = %v", err)
	}

	data, _ := io.ReadAll(result)
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode result: %v", err)
	}

	bounds := decoded.Bounds()
	if bounds.Dx() != 64 || bounds.Dy() != 64 {
		t.Errorf("Small image was resized: got %dx%d, expected 64x64",
			bounds.Dx(), bounds.Dy())
	}
}

func TestProcessAvatarImage_PreservesAspectRatio(t *testing.T) {
	// Create a wide image (2:1 aspect ratio)
	input := createTestImageReader(800, 400)

	result, err := ProcessAvatarImage(input)
	if err != nil {
		t.Fatalf("ProcessAvatarImage() error = %v", err)
	}

	data, _ := io.ReadAll(result)
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode result: %v", err)
	}

	bounds := decoded.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Should be resized to fit within 256x256 while maintaining 2:1 ratio
	// Expected: 256x128
	if width != 256 || height != 128 {
		t.Errorf("Aspect ratio not preserved: got %dx%d, expected 256x128",
			width, height)
	}
}

func TestProcessAvatarImage_InvalidInput(t *testing.T) {
	// Pass invalid data
	input := strings.NewReader("not an image")

	_, err := ProcessAvatarImage(input)
	if err == nil {
		t.Error("ProcessAvatarImage() should error on invalid input")
	}
}

func TestProcessAvatarImage_EmptyInput(t *testing.T) {
	input := strings.NewReader("")

	_, err := ProcessAvatarImage(input)
	if err == nil {
		t.Error("ProcessAvatarImage() should error on empty input")
	}
}

func TestResizeToFit_NoResizeNeeded(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	result := resizeToFit(img, 256, 256)

	// Should return the same image (no resize)
	if result.Bounds().Dx() != 100 || result.Bounds().Dy() != 100 {
		t.Errorf("Image was unnecessarily resized: got %dx%d",
			result.Bounds().Dx(), result.Bounds().Dy())
	}
}

func TestResizeToFit_WidthConstrained(t *testing.T) {
	// 400x200 image (2:1 ratio) should become 256x128
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))

	result := resizeToFit(img, 256, 256)

	bounds := result.Bounds()
	if bounds.Dx() != 256 || bounds.Dy() != 128 {
		t.Errorf("Wrong resize: got %dx%d, expected 256x128",
			bounds.Dx(), bounds.Dy())
	}
}

func TestResizeToFit_HeightConstrained(t *testing.T) {
	// 200x400 image (1:2 ratio) should become 128x256
	img := image.NewRGBA(image.Rect(0, 0, 200, 400))

	result := resizeToFit(img, 256, 256)

	bounds := result.Bounds()
	if bounds.Dx() != 128 || bounds.Dy() != 256 {
		t.Errorf("Wrong resize: got %dx%d, expected 128x256",
			bounds.Dx(), bounds.Dy())
	}
}

// ============================================================================
// TransformImage Tests
// ============================================================================

func TestTransformImage_FitContain(t *testing.T) {
	tests := []struct {
		name           string
		targetWidth    int
		targetHeight   int
		expectedWidth  int
		expectedHeight int
	}{
		{
			name:           "Scale down preserving aspect ratio (width constrained)",
			targetWidth:    200,
			targetHeight:   200,
			expectedWidth:  200,
			expectedHeight: 100, // Maintains 2:1 ratio
		},
		{
			name:           "Scale down preserving aspect ratio (height constrained)",
			targetWidth:    300,
			targetHeight:   100,
			expectedWidth:  200, // Maintains 2:1 ratio
			expectedHeight: 100,
		},
		{
			name:           "No upscaling if already smaller",
			targetWidth:    500,
			targetHeight:   500,
			expectedWidth:  400, // Original size
			expectedHeight: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh test image for each subtest (2:1 aspect ratio)
			testImg := createTestImage(400, 200)

			result, err := TransformImage(testImg, tt.targetWidth, tt.targetHeight, FitContain)
			if err != nil {
				t.Fatalf("TransformImage failed: %v", err)
			}

			data, _ := io.ReadAll(result.Reader)
			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Failed to decode transformed image: %v", err)
			}

			bounds := img.Bounds()
			gotWidth := bounds.Dx()
			gotHeight := bounds.Dy()

			if gotWidth != tt.expectedWidth || gotHeight != tt.expectedHeight {
				t.Errorf("Expected dimensions %dx%d, got %dx%d",
					tt.expectedWidth, tt.expectedHeight, gotWidth, gotHeight)
			}
		})
	}
}

func TestTransformImage_FitCover(t *testing.T) {
	tests := []struct {
		name         string
		targetWidth  int
		targetHeight int
	}{
		{
			name:         "Square crop from landscape",
			targetWidth:  200,
			targetHeight: 200,
		},
		{
			name:         "Portrait crop from landscape",
			targetWidth:  100,
			targetHeight: 200,
		},
		{
			name:         "Landscape crop smaller",
			targetWidth:  300,
			targetHeight: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh test image for each subtest (2:1 aspect ratio)
			testImg := createTestImage(400, 200)

			result, err := TransformImage(testImg, tt.targetWidth, tt.targetHeight, FitCover)
			if err != nil {
				t.Fatalf("TransformImage failed: %v", err)
			}

			data, _ := io.ReadAll(result.Reader)
			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Failed to decode transformed image: %v", err)
			}

			bounds := img.Bounds()
			gotWidth := bounds.Dx()
			gotHeight := bounds.Dy()

			// FitCover should produce exact dimensions
			if gotWidth != tt.targetWidth || gotHeight != tt.targetHeight {
				t.Errorf("Expected exact dimensions %dx%d, got %dx%d",
					tt.targetWidth, tt.targetHeight, gotWidth, gotHeight)
			}
		})
	}
}

func TestTransformImage_FitExact(t *testing.T) {
	tests := []struct {
		name         string
		targetWidth  int
		targetHeight int
	}{
		{
			name:         "Stretch to square",
			targetWidth:  200,
			targetHeight: 200,
		},
		{
			name:         "Stretch to portrait",
			targetWidth:  100,
			targetHeight: 300,
		},
		{
			name:         "Scale down maintaining exact dimensions",
			targetWidth:  200,
			targetHeight: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh test image for each subtest (2:1 aspect ratio)
			testImg := createTestImage(400, 200)

			result, err := TransformImage(testImg, tt.targetWidth, tt.targetHeight, FitExact)
			if err != nil {
				t.Fatalf("TransformImage failed: %v", err)
			}

			data, _ := io.ReadAll(result.Reader)
			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Failed to decode transformed image: %v", err)
			}

			bounds := img.Bounds()
			gotWidth := bounds.Dx()
			gotHeight := bounds.Dy()

			// FitExact should always produce exact dimensions
			if gotWidth != tt.targetWidth || gotHeight != tt.targetHeight {
				t.Errorf("Expected exact dimensions %dx%d, got %dx%d",
					tt.targetWidth, tt.targetHeight, gotWidth, gotHeight)
			}
		})
	}
}

func TestTransformImage_InvalidFitMode(t *testing.T) {
	testImg := createTestImage(100, 100)

	_, err := TransformImage(testImg, 50, 50, FitMode("invalid"))
	if err == nil {
		t.Error("Expected error for invalid fit mode, got nil")
	}
}

func TestTransformImage_OutputFormat(t *testing.T) {
	// Create a test image
	testImg := createTestImage(200, 200)

	// Transform it
	result, err := TransformImage(testImg, 100, 100, FitContain)
	if err != nil {
		t.Fatalf("TransformImage failed: %v", err)
	}

	// Verify content type is JPEG
	if result.ContentType != "image/jpeg" {
		t.Errorf("Expected content type image/jpeg, got %s", result.ContentType)
	}

	// Read the result to verify it's valid JPEG
	data, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("Failed to read result: %v", err)
	}

	// JPEG files start with FFD8FF magic bytes
	if len(data) < 3 {
		t.Fatal("Output too short to be valid JPEG")
	}

	// Check JPEG magic bytes (SOI marker)
	if data[0] != 0xFF || data[1] != 0xD8 || data[2] != 0xFF {
		t.Error("Output does not have JPEG magic bytes (FFD8FF)")
	}
}

func TestTransformImageWithOptions_UsesSelectedJPEGQuality(t *testing.T) {
	testImg := createTestImage(1200, 800)

	defaultResult, err := TransformImage(testImg, 960, 400, FitContain)
	if err != nil {
		t.Fatalf("TransformImage failed: %v", err)
	}
	defaultData, err := io.ReadAll(defaultResult.Reader)
	if err != nil {
		t.Fatalf("Failed to read default transform: %v", err)
	}

	explicitDefaultResult, err := TransformImageWithOptions(testImg, 960, 400, FitContain, TransformOptions{
		JPEGQuality: DefaultTransformJPEGQuality,
	})
	if err != nil {
		t.Fatalf("TransformImageWithOptions default quality failed: %v", err)
	}
	explicitDefaultData, err := io.ReadAll(explicitDefaultResult.Reader)
	if err != nil {
		t.Fatalf("Failed to read explicit default transform: %v", err)
	}
	if !bytes.Equal(defaultData, explicitDefaultData) {
		t.Fatal("TransformImage did not preserve the default JPEG quality")
	}

	compressedResult, err := TransformImageWithOptions(testImg, 960, 400, FitContain, TransformOptions{
		JPEGQuality: 75,
	})
	if err != nil {
		t.Fatalf("TransformImageWithOptions quality 75 failed: %v", err)
	}
	compressedData, err := io.ReadAll(compressedResult.Reader)
	if err != nil {
		t.Fatalf("Failed to read compressed transform: %v", err)
	}
	if len(compressedData) >= len(defaultData) {
		t.Fatalf("quality 75 output size = %d, want less than quality %d output size %d", len(compressedData), DefaultTransformJPEGQuality, len(defaultData))
	}
}

func TestTransformImageWithOptions_RejectsInvalidJPEGQuality(t *testing.T) {
	testImg := createTestImage(100, 100)
	if _, err := TransformImageWithOptions(testImg, 50, 50, FitContain, TransformOptions{}); err == nil {
		t.Fatal("TransformImageWithOptions accepted zero JPEG quality")
	}
	if _, err := TransformImageWithOptions(testImg, 50, 50, FitContain, TransformOptions{JPEGQuality: 101}); err == nil {
		t.Fatal("TransformImageWithOptions accepted JPEG quality above 100")
	}
}

func TestTransformImage_TransparentPNG_OutputsWebP(t *testing.T) {
	// Create a PNG with transparent pixels
	testImg := createTransparentTestImage(200, 200)

	result, err := TransformImage(testImg, 100, 100, FitContain)
	if err != nil {
		t.Fatalf("TransformImage failed: %v", err)
	}

	// Verify content type is WebP (not JPEG, which would destroy transparency)
	if result.ContentType != "image/webp" {
		t.Errorf("Expected content type image/webp for transparent image, got %s", result.ContentType)
	}

	// Read the result and verify it's valid WebP
	data, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("Failed to read result: %v", err)
	}

	// WebP files start with "RIFF....WEBP" magic bytes
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		t.Error("Output does not have WebP magic bytes")
	}

	// Decode and verify dimensions
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode WebP output: %v", err)
	}

	bounds := decoded.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 100 {
		t.Errorf("Expected 100x100, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestTransformImage_TransparentPNG_PreservesAlpha(t *testing.T) {
	// Create a PNG with transparent pixels
	testImg := createTransparentTestImage(100, 100)

	result, err := TransformImage(testImg, 100, 100, FitContain)
	if err != nil {
		t.Fatalf("TransformImage failed: %v", err)
	}

	data, _ := io.ReadAll(result.Reader)
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode output: %v", err)
	}

	// Check that at least some pixels are transparent in the output
	hasTransparentPixel := false
	bounds := decoded.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := decoded.At(x, y).RGBA()
			if a < 0xffff {
				hasTransparentPixel = true
				break
			}
		}
		if hasTransparentPixel {
			break
		}
	}

	if !hasTransparentPixel {
		t.Error("Transparent pixels were not preserved in output")
	}
}

func TestTransformImage_OpaquePNG_OutputsJPEG(t *testing.T) {
	// Create a fully opaque PNG (no transparency)
	testImg := createTestImage(200, 200)

	result, err := TransformImage(testImg, 100, 100, FitContain)
	if err != nil {
		t.Fatalf("TransformImage failed: %v", err)
	}

	// Opaque images should still produce JPEG for smaller file sizes
	if result.ContentType != "image/jpeg" {
		t.Errorf("Expected content type image/jpeg for opaque image, got %s", result.ContentType)
	}

	data, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("Failed to read result: %v", err)
	}

	// JPEG files start with FFD8FF magic bytes
	if len(data) < 3 || data[0] != 0xFF || data[1] != 0xD8 || data[2] != 0xFF {
		t.Error("Output does not have JPEG magic bytes")
	}
}

func TestTransformImage_TransparentPNG_AllFitModes(t *testing.T) {
	fitModes := []FitMode{FitContain, FitCover, FitExact}

	for _, fit := range fitModes {
		t.Run(string(fit), func(t *testing.T) {
			testImg := createTransparentTestImage(200, 100)

			result, err := TransformImage(testImg, 100, 100, fit)
			if err != nil {
				t.Fatalf("TransformImage failed for %s: %v", fit, err)
			}

			if result.ContentType != "image/webp" {
				t.Errorf("Expected content type image/webp for transparent image with fit %s, got %s",
					fit, result.ContentType)
			}
		})
	}
}

func TestDetectImageContentType(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "JPEG",
			data:     []byte{0xFF, 0xD8, 0xFF, 0xE0},
			expected: "image/jpeg",
		},
		{
			name:     "WebP",
			data:     []byte("RIFF\x00\x00\x00\x00WEBP"),
			expected: "image/webp",
		},
		{
			name:     "GIF",
			data:     []byte("GIF89a"),
			expected: "image/gif",
		},
		{
			name:     "PNG",
			data:     []byte{0x89, 'P', 'N', 'G'},
			expected: "image/png",
		},
		{
			name:     "unknown",
			data:     []byte{0x00, 0x01, 0x02},
			expected: "application/octet-stream",
		},
		{
			name:     "empty",
			data:     []byte{},
			expected: "application/octet-stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectImageContentType(tt.data)
			if got != tt.expected {
				t.Errorf("DetectImageContentType() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestTransformImage_MinimalDimensions(t *testing.T) {
	testImg := createTestImage(100, 100)

	// Test 1x1 transformation
	result, err := TransformImage(testImg, 1, 1, FitExact)
	if err != nil {
		t.Fatalf("Failed to transform to 1x1: %v", err)
	}

	data, _ := io.ReadAll(result.Reader)
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode 1x1 image: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 1 || bounds.Dy() != 1 {
		t.Errorf("Expected 1x1, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestTransformImage_LargeDimensions(t *testing.T) {
	testImg := createTestImage(100, 100)

	// Test large transformation with FitContain (should not upscale)
	result, err := TransformImage(testImg, 2048, 2048, FitContain)
	if err != nil {
		t.Fatalf("Failed to transform to large size: %v", err)
	}

	data, _ := io.ReadAll(result.Reader)
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode large image: %v", err)
	}

	// FitContain should not upscale, so it should remain 100x100
	bounds := img.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 100 {
		t.Errorf("Expected 100x100 (no upscaling), got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestResizeToCover_Dimensions(t *testing.T) {
	// Create a 400x200 image
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"Square", 200, 200},
		{"Portrait", 100, 300},
		{"Landscape", 300, 150},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resizeToCover(img, tt.width, tt.height)

			bounds := result.Bounds()
			if bounds.Dx() != tt.width || bounds.Dy() != tt.height {
				t.Errorf("Expected %dx%d, got %dx%d",
					tt.width, tt.height, bounds.Dx(), bounds.Dy())
			}
		})
	}
}

func TestResizeToExact_Dimensions(t *testing.T) {
	// Create a 400x200 image
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"Stretch to square", 300, 300},
		{"Stretch to portrait", 100, 500},
		{"Shrink", 100, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resizeToExact(img, tt.width, tt.height)

			bounds := result.Bounds()
			if bounds.Dx() != tt.width || bounds.Dy() != tt.height {
				t.Errorf("Expected %dx%d, got %dx%d",
					tt.width, tt.height, bounds.Dx(), bounds.Dy())
			}
		})
	}
}

func TestAllFitModes(t *testing.T) {
	fitModes := []FitMode{FitContain, FitCover, FitExact}

	for _, fit := range fitModes {
		t.Run(string(fit), func(t *testing.T) {
			// Create a fresh test image for each fit mode
			testImg := createTestImage(200, 100)

			result, err := TransformImage(testImg, 100, 100, fit)
			if err != nil {
				t.Fatalf("TransformImage failed for %s: %v", fit, err)
			}

			// Just verify it produces valid output
			data, err := io.ReadAll(result.Reader)
			if err != nil {
				t.Fatalf("Failed to read result: %v", err)
			}

			if len(data) == 0 {
				t.Error("TransformImage returned empty data")
			}
		})
	}
}

// ============================================================================
// Animated GIF Tests
// ============================================================================

func TestIsAnimatedGIF_WithAnimatedGIF(t *testing.T) {
	data := createAnimatedGIF(100, 100, 5)
	if !IsAnimatedGIF(data) {
		t.Error("IsAnimatedGIF should return true for animated GIF")
	}
}

func TestIsAnimatedGIF_WithStaticGIF(t *testing.T) {
	data := createStaticGIF(100, 100)
	if IsAnimatedGIF(data) {
		t.Error("IsAnimatedGIF should return false for static GIF (1 frame)")
	}
}

func TestIsAnimatedGIF_WithPNG(t *testing.T) {
	data := createTestImage(100, 100)
	if IsAnimatedGIF(data) {
		t.Error("IsAnimatedGIF should return false for PNG")
	}
}

func TestIsAnimatedGIF_WithInvalidData(t *testing.T) {
	data := []byte("not an image")
	if IsAnimatedGIF(data) {
		t.Error("IsAnimatedGIF should return false for invalid data")
	}
}

func TestTransformImage_AnimatedGIF_OutputsWebP(t *testing.T) {
	data := createAnimatedGIF(200, 200, 3)

	result, err := TransformImage(data, 100, 100, FitContain)
	if err != nil {
		t.Fatalf("TransformImage failed: %v", err)
	}

	// Animated GIFs should be converted to animated WebP
	if result.ContentType != "image/webp" {
		t.Errorf("Expected content type image/webp, got %s", result.ContentType)
	}

	outputData, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("Failed to read result: %v", err)
	}

	// Verify WebP magic bytes: "RIFF....WEBP"
	if len(outputData) < 12 || string(outputData[0:4]) != "RIFF" || string(outputData[8:12]) != "WEBP" {
		t.Error("Output does not have WebP magic bytes")
	}
}

func TestTransformImage_AnimatedGIF_PreservesFrameCount(t *testing.T) {
	frameCount := 5
	data := createAnimatedGIF(200, 200, frameCount)

	result, err := TransformImage(data, 100, 100, FitContain)
	if err != nil {
		t.Fatalf("TransformImage failed: %v", err)
	}

	outputData, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("Failed to read result: %v", err)
	}

	// Count ANMF chunks in the WebP output to verify frame count
	got := countANMFChunks(outputData)
	if got != frameCount {
		t.Errorf("Expected %d ANMF frames, got %d", frameCount, got)
	}
}

func TestTransformImage_AnimatedGIF_ResizesDimensions(t *testing.T) {
	data := createAnimatedGIF(400, 200, 3)

	result, err := TransformImage(data, 200, 200, FitContain)
	if err != nil {
		t.Fatalf("TransformImage failed: %v", err)
	}

	outputData, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("Failed to read result: %v", err)
	}

	// golang.org/x/image/webp can't decode animated WebP, so extract
	// canvas dimensions from the VP8X chunk directly.
	// With FitContain and 2:1 aspect ratio fitting into 200x200,
	// the result should be 200x100.
	w, h := animatedWebPCanvasSize(outputData)
	if w != 200 || h != 100 {
		t.Errorf("Expected 200x100, got %dx%d", w, h)
	}
}

func TestTransformImage_AnimatedGIF_NoResizeNeeded(t *testing.T) {
	data := createAnimatedGIF(100, 100, 3)

	// Request larger than actual size with FitContain (no upscaling)
	result, err := TransformImage(data, 200, 200, FitContain)
	if err != nil {
		t.Fatalf("TransformImage failed: %v", err)
	}

	// Even without resizing, animated GIFs are converted to WebP
	if result.ContentType != "image/webp" {
		t.Errorf("Expected content type image/webp, got %s", result.ContentType)
	}

	outputData, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("Failed to read result: %v", err)
	}

	// Verify dimensions are preserved (no upscaling)
	w, h := animatedWebPCanvasSize(outputData)
	if w != 100 || h != 100 {
		t.Errorf("Expected 100x100, got %dx%d", w, h)
	}
}

func TestTransformImage_StaticGIF_ConvertsToJPEG(t *testing.T) {
	// Static GIF (1 frame) should be converted to JPEG like other images
	data := createStaticGIF(100, 100)

	result, err := TransformImage(data, 50, 50, FitContain)
	if err != nil {
		t.Fatalf("TransformImage failed: %v", err)
	}

	// Static GIF should be converted to JPEG
	if result.ContentType != "image/jpeg" {
		t.Errorf("Expected content type image/jpeg for static GIF, got %s", result.ContentType)
	}
}

// ============================================================================
// EXIF Orientation Tests
// ============================================================================

// testJPEGWithOrientation6 is a minimal JPEG with EXIF orientation tag set to 6.
// The raw pixel data is 70x50, but with orientation correction it should appear as 50x70.
// Source: https://github.com/disintegration/imageorient/blob/master/testdata/orientation_6.jpg
var testJPEGWithOrientation6 = mustDecodeBase64(`
/9j/4QBiRXhpZgAATU0AKgAAAAgABQESAAMAAAABAAYAAAEaAAUAAAABAAAASgEbAAUAAAABAAAA
UgEoAAMAAAABAAIAAAITAAMAAAABAAEAAAAAAAAAAABIAAAAAQAAAEgAAAAB/9sAhAACAQEBAQEC
AQEBAgICAgIEAwICAgIFBAQDBAYFBgYGBQYGBgcJCAYHCQcGBggLCAkKCgoKCgYICwwLCgwJCgoK
AQICAgICAgUDAwUKBwYHCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoK
CgoKCgoKCgr/wAARCAAyAEYDASIAAhEBAxEB/8QBogAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcI
CQoLEAACAQMDAgQDBQUEBAAAAX0BAgMABBEFEiExQQYTUWEHInEUMoGRoQgjQrHBFVLR8CQzYnKC
CQoWFxgZGiUmJygpKjQ1Njc4OTpDREVGR0hJSlNUVVZXWFlaY2RlZmdoaWpzdHV2d3h5eoOEhYaH
iImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4eLj5OXm5+jp
6vHy8/T19vf4+foBAAMBAQEBAQEBAQEAAAAAAAABAgMEBQYHCAkKCxEAAgECBAQDBAcFBAQAAQJ3
AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5
OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaan
qKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIR
AxEAPwD9/KKK/nB/4L1f8F6v+CsP7F3/AAVi+K37NH7NH7Vn/CNeCfDX9hf2Jon/AAguhXn2b7Ro
Wn3U3766sZZn3TTyv8znG7AwoAAB/R9RX5A/8GpX/BUf9uz/AIKUf8L6/wCG1vjn/wAJp/whf/CL
f8Iz/wAUzpenfY/tn9r/AGj/AI8LaDzN/wBlg+/u27PlxubP1/8A8F6v2o/jt+xd/wAEnvit+0v+
zR45/wCEa8beGv7C/sTW/wCzLW8+zfaNd0+1m/c3UUsL7oZ5U+ZDjdkYYAgA+v6K/kC/4ijv+C6/
/R83/mMvDH/ysr+v2gAooooAKKKKACv5Av8Ag6O/5Tr/ABz/AO5Z/wDUY0mv6/a/kC/4Ojv+U6/x
z/7ln/1GNJoA+/8A/gxj/wCbov8AuSf/AHP19/8A/B0d/wAoKPjn/wByz/6k+k1+QH/BqV/wVH/Y
T/4Jr/8AC+v+G1vjn/whf/Caf8It/wAIz/xTOqaj9s+x/wBr/aP+PC2n8vZ9qg+/t3b/AJc7Wx9g
f8F6v+C9X/BJ79tH/gk98Vv2aP2aP2rP+El8beJf7C/sTRP+EF12z+0/Z9d0+6m/fXVjFCm2GCV/
mcZ24GWIBAP5wa/v8r+AOv6/f+Io7/ghR/0fN/5jLxP/APKygD7/AKK+AP8AiKO/4IUf9Hzf+Yy8
T/8Aysr6A/YY/wCCo/7Cf/BSj/hKf+GKfjn/AMJp/wAIX9h/4Sb/AIpnVNO+x/bPtH2f/j/toPM3
/ZZ/ubtuz5sblyAe/wBFFFABX8gX/B0d/wAp1/jn/wByz/6jGk1/X7RQB/AHRX9/lFAH8AdFf3+U
UAfwB1+/3/BjH/zdF/3JP/ufr9/qKACiiigAooooAKKKKACiiigAooooAKKKKAP/2Q==
`)

func mustDecodeBase64(s string) []byte {
	// Remove whitespace
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\t", "")
	s = strings.ReplaceAll(s, " ", "")

	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to decode base64: %v", err))
	}
	return data
}

func TestTransformImage_ExifOrientation(t *testing.T) {
	// This JPEG has EXIF orientation tag 6 (90° CCW rotation).
	// Raw pixel data is 70x50, but after orientation correction it should be 50x70.
	data := testJPEGWithOrientation6

	// Transform with dimensions larger than the corrected size (no resize, just orientation fix)
	result, err := TransformImage(data, 100, 100, FitContain)
	if err != nil {
		t.Fatalf("TransformImage failed: %v", err)
	}

	// Decode the output to verify dimensions
	outputData, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("Failed to read result: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(outputData))
	if err != nil {
		t.Fatalf("Failed to decode output: %v", err)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// With orientation correction: 50x70 (not 70x50)
	// FitContain won't upscale, so should get the corrected dimensions
	if width != 50 || height != 70 {
		t.Errorf("EXIF orientation not applied correctly: got %dx%d, expected 50x70", width, height)
	}
}

func TestProcessAttachmentImage_ExifOrientation(t *testing.T) {
	// Test that ProcessAttachmentImage reports corrected dimensions
	data := testJPEGWithOrientation6

	result, err := ProcessAttachmentImageWithConfig(bytes.NewReader(data), DefaultConfig())
	if err != nil {
		t.Fatalf("ProcessAttachmentImageWithConfig failed: %v", err)
	}

	// With orientation correction, dimensions should be 50x70 (not 70x50)
	if result.Width != 50 || result.Height != 70 {
		t.Errorf("EXIF orientation not applied to dimensions: got %dx%d, expected 50x70",
			result.Width, result.Height)
	}
}

// ============================================================================
// GIF Compositing Tests
// ============================================================================

// countANMFChunks counts the number of ANMF (animation frame) chunks in WebP data.
func countANMFChunks(data []byte) int {
	count := 0
	anmf := []byte("ANMF")
	for i := 0; i+4 <= len(data); i++ {
		if bytes.Equal(data[i:i+4], anmf) {
			count++
		}
	}
	return count
}

// animatedWebPCanvasSize extracts the canvas width and height from an animated
// WebP file's VP8X chunk. Returns (0, 0) if the format is unexpected.
func animatedWebPCanvasSize(data []byte) (width, height int) {
	// RIFF header: "RIFF" (4) + size (4) + "WEBP" (4) = 12 bytes
	// VP8X chunk: fourcc (4) + chunk size (4) + data (10) starting at offset 12
	// VP8X data layout: flags (1) + reserved (3) + canvas_width_minus_one (3 LE) + canvas_height_minus_one (3 LE)
	if len(data) < 30 || string(data[12:16]) != "VP8X" {
		return 0, 0
	}
	w := int(data[24]) | int(data[25])<<8 | int(data[26])<<16
	h := int(data[27]) | int(data[28])<<8 | int(data[29])<<16
	return w + 1, h + 1
}

// createAnimatedGIFWithSubFrames creates a GIF with specific sub-rectangle frames
// and disposal methods for testing the compositing logic.
func createAnimatedGIFWithSubFrames(canvasW, canvasH int, frames []testSubFrame) []byte {
	g := &gif.GIF{
		Image:    make([]*image.Paletted, len(frames)),
		Delay:    make([]int, len(frames)),
		Disposal: make([]byte, len(frames)),
		Config: image.Config{
			Width:  canvasW,
			Height: canvasH,
		},
	}

	for i, f := range frames {
		frame := image.NewPaletted(f.rect, f.palette)
		// Fill frame with the specified color index
		for y := f.rect.Min.Y; y < f.rect.Max.Y; y++ {
			for x := f.rect.Min.X; x < f.rect.Max.X; x++ {
				frame.SetColorIndex(x, y, f.colorIdx)
			}
		}
		g.Image[i] = frame
		g.Delay[i] = f.delay
		g.Disposal[i] = f.disposal
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		panic(fmt.Sprintf("failed to encode test GIF: %v", err))
	}
	return buf.Bytes()
}

type testSubFrame struct {
	rect     image.Rectangle
	palette  []color.Color
	colorIdx uint8
	disposal byte
	delay    int
}

var testPalette = []color.Color{
	color.NRGBA{0, 0, 0, 0},       // 0: transparent
	color.NRGBA{255, 0, 0, 255},   // 1: red
	color.NRGBA{0, 255, 0, 255},   // 2: green
	color.NRGBA{0, 0, 255, 255},   // 3: blue
	color.NRGBA{255, 255, 0, 255}, // 4: yellow
}

func TestCompositeGIFFrames_FullCanvas(t *testing.T) {
	// Two full-canvas frames with DisposalNone — each should be the full color
	data := createAnimatedGIFWithSubFrames(10, 10, []testSubFrame{
		{rect: image.Rect(0, 0, 10, 10), palette: testPalette, colorIdx: 1, disposal: gif.DisposalNone, delay: 10},
		{rect: image.Rect(0, 0, 10, 10), palette: testPalette, colorIdx: 2, disposal: gif.DisposalNone, delay: 10},
	})

	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode test GIF: %v", err)
	}

	composited := compositeGIFFrames(g)

	if len(composited) != 2 {
		t.Fatalf("Expected 2 frames, got %d", len(composited))
	}

	// Frame 0 should be red
	r0, g0, b0, a0 := composited[0].At(5, 5).RGBA()
	if r0>>8 != 255 || g0>>8 != 0 || b0>>8 != 0 || a0>>8 != 255 {
		t.Errorf("Frame 0 pixel (5,5): expected red, got RGBA(%d,%d,%d,%d)", r0>>8, g0>>8, b0>>8, a0>>8)
	}

	// Frame 1 should be green (overwrites red since DisposalNone)
	r1, g1, b1, a1 := composited[1].At(5, 5).RGBA()
	if r1>>8 != 0 || g1>>8 != 255 || b1>>8 != 0 || a1>>8 != 255 {
		t.Errorf("Frame 1 pixel (5,5): expected green, got RGBA(%d,%d,%d,%d)", r1>>8, g1>>8, b1>>8, a1>>8)
	}
}

func TestCompositeGIFFrames_SubRectangles(t *testing.T) {
	// Frame 0: full red canvas. Frame 1: small green patch at (2,2)-(5,5).
	// With DisposalNone, frame 1 should show green in the patch and red elsewhere.
	data := createAnimatedGIFWithSubFrames(10, 10, []testSubFrame{
		{rect: image.Rect(0, 0, 10, 10), palette: testPalette, colorIdx: 1, disposal: gif.DisposalNone, delay: 10},
		{rect: image.Rect(2, 2, 5, 5), palette: testPalette, colorIdx: 2, disposal: gif.DisposalNone, delay: 10},
	})

	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode test GIF: %v", err)
	}

	composited := compositeGIFFrames(g)

	// Frame 1: pixel inside the sub-rect should be green
	r, gr, b, a := composited[1].At(3, 3).RGBA()
	if r>>8 != 0 || gr>>8 != 255 || b>>8 != 0 || a>>8 != 255 {
		t.Errorf("Frame 1 pixel (3,3) inside sub-rect: expected green, got RGBA(%d,%d,%d,%d)", r>>8, gr>>8, b>>8, a>>8)
	}

	// Frame 1: pixel outside the sub-rect should still be red (from frame 0)
	r, gr, b, a = composited[1].At(0, 0).RGBA()
	if r>>8 != 255 || gr>>8 != 0 || b>>8 != 0 || a>>8 != 255 {
		t.Errorf("Frame 1 pixel (0,0) outside sub-rect: expected red, got RGBA(%d,%d,%d,%d)", r>>8, gr>>8, b>>8, a>>8)
	}
}

func TestCompositeGIFFrames_DisposalBackground(t *testing.T) {
	// Frame 0: full red canvas with DisposalBackground.
	// Frame 1: small green patch at (2,2)-(5,5).
	// After frame 0's disposal, the canvas should be cleared to transparent,
	// so frame 1 should only show green in the patch and transparent elsewhere.
	data := createAnimatedGIFWithSubFrames(10, 10, []testSubFrame{
		{rect: image.Rect(0, 0, 10, 10), palette: testPalette, colorIdx: 1, disposal: gif.DisposalBackground, delay: 10},
		{rect: image.Rect(2, 2, 5, 5), palette: testPalette, colorIdx: 2, disposal: gif.DisposalNone, delay: 10},
	})

	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode test GIF: %v", err)
	}

	composited := compositeGIFFrames(g)

	// Frame 0 should still be red (disposal applies AFTER snapshot)
	r0, _, _, _ := composited[0].At(5, 5).RGBA()
	if r0>>8 != 255 {
		t.Errorf("Frame 0 pixel (5,5): expected red")
	}

	// Frame 1: pixel inside sub-rect should be green
	r1, g1, b1, a1 := composited[1].At(3, 3).RGBA()
	if r1>>8 != 0 || g1>>8 != 255 || b1>>8 != 0 || a1>>8 != 255 {
		t.Errorf("Frame 1 pixel (3,3): expected green, got RGBA(%d,%d,%d,%d)", r1>>8, g1>>8, b1>>8, a1>>8)
	}

	// Frame 1: pixel outside sub-rect should be transparent (canvas was cleared)
	_, _, _, a1out := composited[1].At(0, 0).RGBA()
	if a1out != 0 {
		t.Errorf("Frame 1 pixel (0,0): expected transparent (alpha=0), got alpha=%d", a1out>>8)
	}
}

func TestCompositeGIFFrames_DisposalPrevious(t *testing.T) {
	// Frame 0: full red canvas with DisposalNone.
	// Frame 1: green patch with DisposalPrevious.
	// Frame 2: blue patch — should see red underneath (restored from before frame 1).
	data := createAnimatedGIFWithSubFrames(10, 10, []testSubFrame{
		{rect: image.Rect(0, 0, 10, 10), palette: testPalette, colorIdx: 1, disposal: gif.DisposalNone, delay: 10},
		{rect: image.Rect(2, 2, 5, 5), palette: testPalette, colorIdx: 2, disposal: gif.DisposalPrevious, delay: 10},
		{rect: image.Rect(6, 6, 9, 9), palette: testPalette, colorIdx: 3, disposal: gif.DisposalNone, delay: 10},
	})

	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode test GIF: %v", err)
	}

	composited := compositeGIFFrames(g)

	// Frame 1: green patch visible
	_, g1, _, _ := composited[1].At(3, 3).RGBA()
	if g1>>8 != 255 {
		t.Errorf("Frame 1 pixel (3,3): expected green")
	}

	// Frame 2: the green patch area should be red again (canvas restored to pre-frame-1)
	r2, g2, b2, _ := composited[2].At(3, 3).RGBA()
	if r2>>8 != 255 || g2>>8 != 0 || b2>>8 != 0 {
		t.Errorf("Frame 2 pixel (3,3): expected red (restored), got RGBA(%d,%d,%d)", r2>>8, g2>>8, b2>>8)
	}

	// Frame 2: blue patch visible
	_, _, b2blue, _ := composited[2].At(7, 7).RGBA()
	if b2blue>>8 != 255 {
		t.Errorf("Frame 2 pixel (7,7): expected blue")
	}
}

func TestCompositeGIFFrames_NilDisposal(t *testing.T) {
	// GIF with no Disposal slice — should behave like DisposalNone
	g := &gif.GIF{
		Image: []*image.Paletted{
			image.NewPaletted(image.Rect(0, 0, 10, 10), testPalette),
			image.NewPaletted(image.Rect(0, 0, 10, 10), testPalette),
		},
		Delay:    []int{10, 10},
		Disposal: nil, // no disposal set
		Config:   image.Config{Width: 10, Height: 10},
	}

	// Fill frames
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			g.Image[0].SetColorIndex(x, y, 1) // red
			g.Image[1].SetColorIndex(x, y, 2) // green
		}
	}

	composited := compositeGIFFrames(g)

	if len(composited) != 2 {
		t.Fatalf("Expected 2 frames, got %d", len(composited))
	}

	// Should not panic and frames should composite normally
	r0, _, _, _ := composited[0].At(5, 5).RGBA()
	if r0>>8 != 255 {
		t.Errorf("Frame 0: expected red")
	}

	_, g1, _, _ := composited[1].At(5, 5).RGBA()
	if g1>>8 != 255 {
		t.Errorf("Frame 1: expected green")
	}
}

// ============================================================================
// GIF→WebP Conversion Integration Tests
// ============================================================================

func TestTransformImage_AnimatedGIF_SubRectangles(t *testing.T) {
	// End-to-end: a GIF with sub-rectangle frames should produce valid animated WebP
	data := createAnimatedGIFWithSubFrames(100, 100, []testSubFrame{
		{rect: image.Rect(0, 0, 100, 100), palette: testPalette, colorIdx: 1, disposal: gif.DisposalNone, delay: 10},
		{rect: image.Rect(20, 20, 50, 50), palette: testPalette, colorIdx: 2, disposal: gif.DisposalNone, delay: 10},
		{rect: image.Rect(60, 60, 90, 90), palette: testPalette, colorIdx: 3, disposal: gif.DisposalBackground, delay: 10},
	})

	result, err := TransformImage(data, 50, 50, FitContain)
	if err != nil {
		t.Fatalf("TransformImage failed: %v", err)
	}

	if result.ContentType != "image/webp" {
		t.Errorf("Expected image/webp, got %s", result.ContentType)
	}

	outputData, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("Failed to read result: %v", err)
	}

	if countANMFChunks(outputData) != 3 {
		t.Errorf("Expected 3 ANMF frames, got %d", countANMFChunks(outputData))
	}

	// Verify dimensions from VP8X canvas size
	w, h := animatedWebPCanvasSize(outputData)
	if w != 50 || h != 50 {
		t.Errorf("Expected 50x50, got %dx%d", w, h)
	}
}

// ============================================================================
// Loop Count Conversion Tests
// ============================================================================

func TestConvertGIFLoopCount(t *testing.T) {
	tests := []struct {
		name     string
		gifLoop  int
		expected uint16
	}{
		{"infinite", 0, 0},
		{"once", -1, 1},
		{"negative", -100, 1},
		{"play twice", 1, 2},
		{"play 6 times", 5, 6},
		{"large value", 70000, 65535},
		{"max uint16 minus 1", 65534, 65535},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertGIFLoopCount(tt.gifLoop)
			if got != tt.expected {
				t.Errorf("convertGIFLoopCount(%d) = %d, want %d", tt.gifLoop, got, tt.expected)
			}
		})
	}
}
