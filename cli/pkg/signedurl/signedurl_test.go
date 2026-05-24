package signedurl_test

import (
	"testing"
	"time"

	"hmans.de/chatto/pkg/signedurl"
)

func TestSignedTransformPath(t *testing.T) {
	secret := "test-secret-key-1234567890"
	spaceID := "space123"
	attachmentID := "attach456"
	width := 640
	height := 512
	fit := "contain"

	// Generate signed path
	signedPath := signedurl.SignedTransformPath(secret, spaceID, attachmentID, width, height, fit)

	// Should contain a dot separating params and signature
	if len(signedPath) == 0 {
		t.Error("Signed path should not be empty")
	}

	// Should be deterministic
	signedPath2 := signedurl.SignedTransformPath(secret, spaceID, attachmentID, width, height, fit)
	if signedPath != signedPath2 {
		t.Errorf("Signed path is not deterministic: %s != %s", signedPath, signedPath2)
	}

	// Different parameters should produce different paths
	signedPathDiff := signedurl.SignedTransformPath(secret, spaceID, attachmentID, 800, height, fit)
	if signedPath == signedPathDiff {
		t.Error("Different width should produce different signed path")
	}
}

func TestParseSignedTransformPath(t *testing.T) {
	secret := "test-secret-key-1234567890"
	spaceID := "space123"
	attachmentID := "attach456"
	width := 640
	height := 512
	fit := "contain"

	// Generate a valid signed path
	signedPath := signedurl.SignedTransformPath(secret, spaceID, attachmentID, width, height, fit)

	// Parse it back
	params, err := signedurl.ParseSignedTransformPath(secret, spaceID, attachmentID, signedPath)
	if err != nil {
		t.Fatalf("Failed to parse valid signed path: %v", err)
	}

	// Verify params
	if params.Width != width {
		t.Errorf("Width mismatch: expected %d, got %d", width, params.Width)
	}
	if params.Height != height {
		t.Errorf("Height mismatch: expected %d, got %d", height, params.Height)
	}
	if params.Fit != fit {
		t.Errorf("Fit mismatch: expected %s, got %s", fit, params.Fit)
	}
}

func TestParseSignedTransformPath_InvalidSignature(t *testing.T) {
	secret := "test-secret-key-1234567890"
	spaceID := "space123"
	attachmentID := "attach456"

	// Generate a valid signed path
	signedPath := signedurl.SignedTransformPath(secret, spaceID, attachmentID, 640, 512, "contain")

	// Try with wrong secret
	_, err := signedurl.ParseSignedTransformPath("wrong-secret", spaceID, attachmentID, signedPath)
	if err == nil {
		t.Error("Should fail with wrong secret")
	}

	// Try with wrong spaceID
	_, err = signedurl.ParseSignedTransformPath(secret, "wrong-space", attachmentID, signedPath)
	if err == nil {
		t.Error("Should fail with wrong spaceID")
	}

	// Try with wrong attachmentID
	_, err = signedurl.ParseSignedTransformPath(secret, spaceID, "wrong-attachment", signedPath)
	if err == nil {
		t.Error("Should fail with wrong attachmentID")
	}
}

func TestParseSignedTransformPath_InvalidFormat(t *testing.T) {
	secret := "test-secret"
	spaceID := "sp"
	attachmentID := "at"

	// Missing dot separator
	_, err := signedurl.ParseSignedTransformPath(secret, spaceID, attachmentID, "nodothere")
	if err == nil {
		t.Error("Should fail with invalid format (no dot)")
	}

	// Invalid base64
	_, err = signedurl.ParseSignedTransformPath(secret, spaceID, attachmentID, "!!!invalid.abc123")
	if err == nil {
		t.Error("Should fail with invalid base64")
	}
}

func TestParseSignedTransformPath_InvalidParams(t *testing.T) {
	secret := "test-secret"
	spaceID := "sp"
	attachmentID := "at"

	tests := []struct {
		name   string
		width  int
		height int
		fit    string
	}{
		{"width zero", 0, 100, "contain"},
		{"width negative", -1, 100, "contain"},
		{"width too large", 3000, 100, "contain"},
		{"width at boundary+1", 2049, 100, "contain"},
		{"height zero", 100, 0, "contain"},
		{"height negative", 100, -1, "contain"},
		{"height too large", 100, 3000, "contain"},
		{"height at boundary+1", 100, 2049, "contain"},
		{"invalid fit mode", 100, 100, "invalid"},
		{"empty fit mode", 100, 100, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signedPath := signedurl.SignedTransformPath(secret, spaceID, attachmentID, tt.width, tt.height, tt.fit)
			_, err := signedurl.ParseSignedTransformPath(secret, spaceID, attachmentID, signedPath)
			if err == nil {
				t.Errorf("expected error for %s (w=%d, h=%d, fit=%q)", tt.name, tt.width, tt.height, tt.fit)
			}
		})
	}
}

func TestParseSignedTransformPath_ValidBoundaries(t *testing.T) {
	secret := "test-secret"
	spaceID := "sp"
	attachmentID := "at"

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"min dimensions", 1, 1},
		{"max dimensions", 2048, 2048},
		{"min width max height", 1, 2048},
		{"max width min height", 2048, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signedPath := signedurl.SignedTransformPath(secret, spaceID, attachmentID, tt.width, tt.height, "contain")
			params, err := signedurl.ParseSignedTransformPath(secret, spaceID, attachmentID, signedPath)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if params.Width != tt.width {
				t.Errorf("width: got %d, want %d", params.Width, tt.width)
			}
			if params.Height != tt.height {
				t.Errorf("height: got %d, want %d", params.Height, tt.height)
			}
		})
	}
}

func TestSignedAttachmentLocator_RoundTrip(t *testing.T) {
	secret := "test-secret-key-1234567890"
	exp := time.Now().Add(time.Hour).Unix()

	tests := []struct {
		name string
		loc  signedurl.AttachmentLocator
	}{
		{
			name: "body attachment",
			loc: signedurl.AttachmentLocator{
				RoomID: "Rabc", BodyKey: "Uxyz.E123", AttachmentID: "Aqwe",
				UserID: "Uviewer", ExpiresAt: exp,
			},
		},
		{
			name: "video variant",
			loc: signedurl.AttachmentLocator{
				RoomID: "Rabc", VideoOrigin: "Aorigvid", AttachmentID: "Avariant",
				UserID: "Uviewer", ExpiresAt: exp,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signed, err := signedurl.SignedAttachmentLocator(secret, tt.loc)
			if err != nil {
				t.Fatalf("SignedAttachmentLocator: %v", err)
			}
			got, err := signedurl.ParseSignedAttachmentLocator(secret, signed)
			if err != nil {
				t.Fatalf("ParseSignedAttachmentLocator: %v", err)
			}
			if *got != tt.loc {
				t.Errorf("round-trip mismatch: got %+v, want %+v", *got, tt.loc)
			}
		})
	}
}

func TestSignedAttachmentLocator_InvalidLocator(t *testing.T) {
	secret := "test-secret"
	exp := time.Now().Add(time.Hour).Unix()

	tests := []struct {
		name string
		loc  signedurl.AttachmentLocator
	}{
		{"missing room", signedurl.AttachmentLocator{BodyKey: "U.E", AttachmentID: "A", UserID: "U", ExpiresAt: exp}},
		{"missing attachment", signedurl.AttachmentLocator{RoomID: "R", BodyKey: "U.E", UserID: "U", ExpiresAt: exp}},
		{"missing source", signedurl.AttachmentLocator{RoomID: "R", AttachmentID: "A", UserID: "U", ExpiresAt: exp}},
		{"both sources", signedurl.AttachmentLocator{RoomID: "R", BodyKey: "U.E", VideoOrigin: "Av", AttachmentID: "A", UserID: "U", ExpiresAt: exp}},
		{"missing user", signedurl.AttachmentLocator{RoomID: "R", BodyKey: "U.E", AttachmentID: "A", ExpiresAt: exp}},
		{"missing expiry", signedurl.AttachmentLocator{RoomID: "R", BodyKey: "U.E", AttachmentID: "A", UserID: "U"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := signedurl.SignedAttachmentLocator(secret, tt.loc); err == nil {
				t.Error("expected error from SignedAttachmentLocator")
			}
		})
	}
}

func TestParseSignedAttachmentLocator_InvalidSignature(t *testing.T) {
	secret := "test-secret"
	loc := signedurl.AttachmentLocator{RoomID: "R", BodyKey: "U.E", AttachmentID: "A", UserID: "U", ExpiresAt: time.Now().Add(time.Hour).Unix()}

	signed, err := signedurl.SignedAttachmentLocator(secret, loc)
	if err != nil {
		t.Fatalf("SignedAttachmentLocator: %v", err)
	}

	if _, err := signedurl.ParseSignedAttachmentLocator("wrong-secret", signed); err == nil {
		t.Error("expected error with wrong secret")
	}

	// Tamper with the signature
	tampered := signed[:len(signed)-2] + "00"
	if _, err := signedurl.ParseSignedAttachmentLocator(secret, tampered); err == nil {
		t.Error("expected error with tampered signature")
	}

	// Tamper with the payload (sig no longer matches)
	tampered2 := "QQ" + signed[2:]
	if _, err := signedurl.ParseSignedAttachmentLocator(secret, tampered2); err == nil {
		t.Error("expected error with tampered payload")
	}
}

func TestAttachmentLocator_Expired(t *testing.T) {
	loc := signedurl.AttachmentLocator{ExpiresAt: 1000}
	if loc.Expired(999) {
		t.Error("expected not expired one second before deadline")
	}
	if !loc.Expired(1000) {
		t.Error("expected expired at deadline")
	}
	if !loc.Expired(1001) {
		t.Error("expected expired after deadline")
	}
}

func TestParseSignedAttachmentLocator_InvalidFormat(t *testing.T) {
	secret := "test-secret"

	cases := []string{
		"",
		"nodothere",
		"!!!.abc",
	}
	for _, c := range cases {
		if _, err := signedurl.ParseSignedAttachmentLocator(secret, c); err == nil {
			t.Errorf("expected error parsing %q", c)
		}
	}
}

func TestSignedTransformPath_AllFitModes(t *testing.T) {
	secret := "test-secret"
	spaceID := "sp"
	attachmentID := "at"
	width := 100
	height := 100

	fitModes := []string{"contain", "cover", "exact"}
	for _, fit := range fitModes {
		signedPath := signedurl.SignedTransformPath(secret, spaceID, attachmentID, width, height, fit)
		params, err := signedurl.ParseSignedTransformPath(secret, spaceID, attachmentID, signedPath)
		if err != nil {
			t.Errorf("Fit mode %s failed: %v", fit, err)
			continue
		}
		if params.Fit != fit {
			t.Errorf("Fit mode mismatch: expected %s, got %s", fit, params.Fit)
		}
	}
}
