// Package signedurl provides HMAC-signed URL path generation and verification.
// It creates tamper-proof URL path components by signing parameters with
// HMAC-SHA256, and verifies signatures on the way back in.
package signedurl

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// AttachmentLocator is the payload encoded into an attachment URL. It
// carries everything the HTTP handler needs to authorize and serve the
// attachment without any standalone-index lookup:
//
//   - RoomID is checked against the requester's room membership.
//   - Exactly one of BodyKey or VideoOrigin identifies where the source
//     of truth lives: a `MessageBody` keyed by BodyKey (for body-embedded
//     attachments), or a `VideoProcessingState` keyed by VideoOrigin
//     (for variants and thumbnails generated from a parent video).
//   - AttachmentID identifies the specific attachment within that source.
//
// JSON keys are single letters to keep URLs short.
type AttachmentLocator struct {
	RoomID       string `json:"r"`
	BodyKey      string `json:"b,omitempty"`
	VideoOrigin  string `json:"v,omitempty"`
	AttachmentID string `json:"a"`
}

// Validate returns an error if the locator is missing required fields
// or specifies an inconsistent source. Both Sign and Parse run this so
// invalid locators never make it onto a URL or out of one.
func (l AttachmentLocator) Validate() error {
	if l.RoomID == "" {
		return errors.New("locator: missing room id")
	}
	if l.AttachmentID == "" {
		return errors.New("locator: missing attachment id")
	}
	hasBody := l.BodyKey != ""
	hasVideo := l.VideoOrigin != ""
	if hasBody == hasVideo {
		return errors.New("locator: must specify exactly one of body_key or video_origin")
	}
	return nil
}

// SignedAttachmentLocator encodes a locator as `{base64payload}.{hexHMAC}`.
// The result is a single URL path segment.
func SignedAttachmentLocator(secret string, loc AttachmentLocator) (string, error) {
	if err := loc.Validate(); err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(loc)
	if err != nil {
		return "", fmt.Errorf("marshal locator: %w", err)
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payloadB64))
	signature := hex.EncodeToString(h.Sum(nil)[:16])
	return payloadB64 + "." + signature, nil
}

// ParseSignedAttachmentLocator verifies the signature on `signed` and
// returns the decoded locator. Returns an error if the signature is
// invalid, the payload is malformed, or the locator fails Validate.
func ParseSignedAttachmentLocator(secret, signed string) (*AttachmentLocator, error) {
	parts := strings.SplitN(signed, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid signed locator format")
	}
	payloadB64, signature := parts[0], parts[1]

	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payloadB64))
	expectedSig := hex.EncodeToString(h.Sum(nil)[:16])
	if !hmac.Equal([]byte(expectedSig), []byte(signature)) {
		return nil, errors.New("invalid signature")
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 payload: %w", err)
	}
	var loc AttachmentLocator
	if err := json.Unmarshal(payloadJSON, &loc); err != nil {
		return nil, fmt.Errorf("invalid payload JSON: %w", err)
	}
	if err := loc.Validate(); err != nil {
		return nil, err
	}
	return &loc, nil
}

// TransformParams holds the parameters for an image transformation.
type TransformParams struct {
	Width  int    `json:"w"`
	Height int    `json:"h"`
	Fit    string `json:"f"`
}

// SignedTransformPath generates a signed path component for an image transformation URL.
// Returns a string in the format: {base64params}.{signature}
// where base64params is base64url-encoded JSON: {"w":width,"h":height,"f":"fit"}
// and signature is a truncated HMAC-SHA256 of {resourceID1}/{resourceID2}/{base64params}
//
// The resourceID1 and resourceID2 parameters are opaque strings that identify the resource.
// This function has no knowledge of what they represent.
func SignedTransformPath(secret, resourceID1, resourceID2 string, width, height int, fit string) string {
	// Encode params as JSON then base64url
	params := TransformParams{Width: width, Height: height, Fit: fit}
	paramsJSON, _ := json.Marshal(params)
	paramsB64 := base64.RawURLEncoding.EncodeToString(paramsJSON)

	// Sign: {resourceID1}/{resourceID2}/{paramsB64}
	message := fmt.Sprintf("%s/%s/%s", resourceID1, resourceID2, paramsB64)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	// Use first 16 bytes (32 hex chars) for shorter URLs while still secure
	signature := hex.EncodeToString(h.Sum(nil)[:16])

	return paramsB64 + "." + signature
}

// ParseSignedTransformPath parses and verifies a signed transform path.
// Input format: {base64params}.{signature}
// Returns the transform params if valid, or an error if invalid.
//
// The resourceID1 and resourceID2 parameters are opaque strings that identify the resource.
// This function has no knowledge of what they represent.
func ParseSignedTransformPath(secret, resourceID1, resourceID2, signedPath string) (*TransformParams, error) {
	// Split into params and signature
	parts := strings.SplitN(signedPath, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid signed path format")
	}
	paramsB64, signature := parts[0], parts[1]

	// Verify signature first (constant-time comparison)
	message := fmt.Sprintf("%s/%s/%s", resourceID1, resourceID2, paramsB64)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	expectedSig := hex.EncodeToString(h.Sum(nil)[:16])
	if !hmac.Equal([]byte(expectedSig), []byte(signature)) {
		return nil, fmt.Errorf("invalid signature")
	}

	// Decode base64 params
	paramsJSON, err := base64.RawURLEncoding.DecodeString(paramsB64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 params: %w", err)
	}

	// Parse JSON
	var params TransformParams
	if err := json.Unmarshal(paramsJSON, &params); err != nil {
		return nil, fmt.Errorf("invalid params JSON: %w", err)
	}

	// Validate params
	if params.Width < 1 || params.Width > 2048 {
		return nil, fmt.Errorf("width out of range [1, 2048]: %d", params.Width)
	}
	if params.Height < 1 || params.Height > 2048 {
		return nil, fmt.Errorf("height out of range [1, 2048]: %d", params.Height)
	}
	if params.Fit != "contain" && params.Fit != "cover" && params.Fit != "exact" {
		return nil, fmt.Errorf("invalid fit mode: %s", params.Fit)
	}

	return &params, nil
}
