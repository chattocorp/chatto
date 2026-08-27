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

func TestSignedAssetAccessTicket(t *testing.T) {
	secret := "test-secret"
	ticket := signedurl.AssetAccessTicket{
		AssetID:   "A123",
		UserID:    "U456",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		Width:     640,
		Height:    512,
		Fit:       "contain",
	}

	signed, err := signedurl.SignedAssetAccessTicket(secret, ticket)
	if err != nil {
		t.Fatalf("SignedAssetAccessTicket failed: %v", err)
	}
	parsed, err := signedurl.ParseSignedAssetAccessTicket(secret, signed)
	if err != nil {
		t.Fatalf("ParseSignedAssetAccessTicket failed: %v", err)
	}

	if parsed.AssetID != ticket.AssetID {
		t.Errorf("AssetID mismatch: got %q, want %q", parsed.AssetID, ticket.AssetID)
	}
	if parsed.UserID != ticket.UserID {
		t.Errorf("UserID mismatch: got %q, want %q", parsed.UserID, ticket.UserID)
	}
	if parsed.ExpiresAt != ticket.ExpiresAt {
		t.Errorf("ExpiresAt mismatch: got %d, want %d", parsed.ExpiresAt, ticket.ExpiresAt)
	}
	if !parsed.MatchesTransform(&signedurl.TransformParams{Width: 640, Height: 512, Fit: "contain"}) {
		t.Error("expected parsed ticket to match its transform params")
	}
	if parsed.MatchesTransform(&signedurl.TransformParams{Width: 641, Height: 512, Fit: "contain"}) {
		t.Error("expected parsed ticket not to match mutated transform params")
	}

	if _, err := signedurl.ParseSignedAssetAccessTicket("wrong-secret", signed); err == nil {
		t.Error("expected wrong secret to fail")
	}

	replacement := "0"
	if signed[len(signed)-1:] == replacement {
		replacement = "1"
	}
	tampered := signed[:len(signed)-1] + replacement
	if _, err := signedurl.ParseSignedAssetAccessTicket(secret, tampered); err == nil {
		t.Error("expected tampered ticket to fail")
	}
}

func TestAssetAccessTicket_Expired(t *testing.T) {
	ticket := signedurl.AssetAccessTicket{ExpiresAt: 1000}
	if ticket.Expired(999) {
		t.Error("expected not expired one second before deadline")
	}
	if !ticket.Expired(1000) {
		t.Error("expected expired at deadline")
	}
}

func TestSignedHLSAccessTicket(t *testing.T) {
	secret := "test-secret"
	ticket := signedurl.HLSAccessTicket{
		AssetID:   "A-video",
		UserID:    "U-viewer",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}

	signed, err := signedurl.SignedHLSAccessTicket(secret, ticket)
	if err != nil {
		t.Fatalf("SignedHLSAccessTicket failed: %v", err)
	}
	parsed, err := signedurl.ParseSignedHLSAccessTicket(secret, signed)
	if err != nil {
		t.Fatalf("ParseSignedHLSAccessTicket failed: %v", err)
	}
	if *parsed != ticket {
		t.Fatalf("parsed ticket = %#v, want %#v", parsed, ticket)
	}
	if err := parsed.Validate(); err != nil {
		t.Fatalf("valid ticket failed validation: %v", err)
	}
	if parsed.Expired(ticket.ExpiresAt - 1) {
		t.Fatal("ticket expired before its deadline")
	}
	if !parsed.Expired(ticket.ExpiresAt) {
		t.Fatal("ticket did not expire at its deadline")
	}
	if _, err := signedurl.ParseSignedHLSAccessTicket("wrong-secret", signed); err == nil {
		t.Fatal("wrong secret unexpectedly parsed HLS ticket")
	}

	tampered := signed[:len(signed)-1] + "0"
	if tampered == signed {
		tampered = signed[:len(signed)-1] + "1"
	}
	if _, err := signedurl.ParseSignedHLSAccessTicket(secret, tampered); err == nil {
		t.Fatal("tampered HLS ticket unexpectedly parsed")
	}
	if _, err := signedurl.ParseSignedAssetAccessTicket(secret, signed); err == nil {
		t.Fatal("HLS ticket unexpectedly parsed as an asset ticket")
	}
}

func TestHLSAccessTicketValidate(t *testing.T) {
	for _, tt := range []struct {
		name   string
		ticket signedurl.HLSAccessTicket
	}{
		{name: "missing asset", ticket: signedurl.HLSAccessTicket{UserID: "U", ExpiresAt: 1}},
		{name: "missing user", ticket: signedurl.HLSAccessTicket{AssetID: "A", ExpiresAt: 1}},
		{name: "missing expiry", ticket: signedurl.HLSAccessTicket{AssetID: "A", UserID: "U"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.ticket.Validate(); err == nil {
				t.Fatal("invalid ticket passed validation")
			}
		})
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
