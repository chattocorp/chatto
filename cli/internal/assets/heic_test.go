package assets

import (
	"bytes"
	"image"
	"io"
	"os"
	"testing"
)

func TestHEICImageProcessing(t *testing.T) {
	data, err := os.ReadFile("testdata/photo.heic")
	if err != nil {
		t.Fatal(err)
	}
	info, err := InspectImage(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "heic" || info.Width != 512 || info.Height != 512 {
		t.Fatalf("unexpected image metadata: %+v", info)
	}
	result, err := TransformImage(data, 100, 100, FitContain)
	if err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(output))
	if err != nil {
		t.Fatal(err)
	}
	if result.ContentType != "image/jpeg" || img.Bounds().Dx() > 100 || img.Bounds().Dy() > 100 {
		t.Fatalf("unexpected transform: %s, %v", result.ContentType, img.Bounds())
	}
}

func TestHEICInspectionRejectsInvalidContainers(t *testing.T) {
	data, err := os.ReadFile("testdata/photo.heic")
	if err != nil {
		t.Fatal(err)
	}
	sequence := append(bytes.Clone(data), []byte{0, 0, 0, 8, 'm', 'o', 'o', 'v'}...)
	for name, input := range map[string][]byte{
		"sequence": sequence, "truncated": data[:32],
		"invalid box size": append(bytes.Clone(data), []byte{0, 0, 0, 4, 'f', 'r', 'e', 'e'}...),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := InspectImage(bytes.NewReader(input)); err == nil {
				t.Fatal("invalid HEIC accepted")
			}
			if _, err := TransformImage(input, 100, 100, FitContain); err == nil {
				t.Fatal("invalid HEIC transformed")
			}
		})
	}
}

func TestHEIFGenericBrand(t *testing.T) {
	data, err := os.ReadFile("testdata/photo.heic")
	if err != nil {
		t.Fatal(err)
	}
	copy(data[8:12], "mif1")
	if _, err := InspectImage(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if _, err := TransformImage(data, 100, 100, FitContain); err != nil {
		t.Fatal(err)
	}
}
