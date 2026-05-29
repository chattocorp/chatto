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
//   - RoomID is checked against the signed user's current room membership.
//   - Exactly one of BodyKey or VideoOrigin identifies where the source
//     of truth lives: a `MessageBody` keyed by BodyKey (for body-embedded
//     attachments), or a projected `AssetProcessingSucceededEvent` keyed by
//     VideoOrigin (for variants and thumbnails generated from a parent video).
//   - AttachmentID identifies the specific attachment within that source.
//   - UserID is the user the URL was issued for. The HTTP handler trusts
//     this claim (it's covered by the HMAC) and checks current room
//     membership for that user — no session or bearer token required.
//   - ExpiresAt is the Unix-second deadline after which the URL stops
//     working. Lets URLs be safely exposed to clients that can't carry
//     a session cookie (cross-origin <img>, mobile share sheets, etc.).
//
// JSON keys are single letters to keep URLs short.
type AttachmentLocator struct {
	RoomID       string `json:"r"`
	BodyKey      string `json:"b,omitempty"`
	VideoOrigin  string `json:"v,omitempty"`
	AttachmentID string `json:"a"`
	UserID       string `json:"u"`
	ExpiresAt    int64  `json:"e"`
}

// Validate returns an error if the locator is missing required fields
// or specifies an inconsistent source. Both Sign and Parse run this so
// invalid locators never make it onto a URL or out of one.
//
// Source-of-truth hint rules:
//   - At most one of BodyKey / VideoOrigin may be set.
//   - When neither is set, the handler resolves the asset directly from
//     the asset projection by AttachmentID. This is the new (asset-as-
//     aggregate) form; BodyKey and VideoOrigin are legacy hints retained
//     for URLs minted before the asset aggregate redesign.
func (l AttachmentLocator) Validate() error {
	if l.RoomID == "" {
		return errors.New("locator: missing room id")
	}
	if l.AttachmentID == "" {
		return errors.New("locator: missing attachment id")
	}
	if l.UserID == "" {
		return errors.New("locator: missing user id")
	}
	if l.ExpiresAt == 0 {
		return errors.New("locator: missing expiry")
	}
	if l.BodyKey != "" && l.VideoOrigin != "" {
		return errors.New("locator: body_key and video_origin are mutually exclusive")
	}
	return nil
}

// Expired reports whether the locator's deadline has passed relative to
// `now`. Callers pass `time.Now().Unix()` in production code and a
// fixed value in tests.
func (l AttachmentLocator) Expired(now int64) bool {
	return l.ExpiresAt <= now
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

// AssetAccessTicket authorizes a viewer to read one asset. The asset identity
// still lives in the URL path; the ticket is only an access credential.
type AssetAccessTicket struct {
	AssetID   string `json:"a"`
	UserID    string `json:"u"`
	ExpiresAt int64  `json:"e"`
	Width     int    `json:"w,omitempty"`
	Height    int    `json:"h,omitempty"`
	Fit       string `json:"f,omitempty"`
}

func (t AssetAccessTicket) Validate() error {
	if t.AssetID == "" {
		return errors.New("asset ticket: missing asset id")
	}
	if t.UserID == "" {
		return errors.New("asset ticket: missing user id")
	}
	if t.ExpiresAt == 0 {
		return errors.New("asset ticket: missing expiry")
	}
	hasTransform := t.Width != 0 || t.Height != 0 || t.Fit != ""
	if hasTransform {
		if err := validateTransformParams(t.Width, t.Height, t.Fit); err != nil {
			return fmt.Errorf("asset ticket: %w", err)
		}
	}
	return nil
}

func (t AssetAccessTicket) MatchesTransform(params *TransformParams) bool {
	if params == nil {
		return t.Width == 0 && t.Height == 0 && t.Fit == ""
	}
	return t.Width == params.Width && t.Height == params.Height && t.Fit == params.Fit
}

func (t AssetAccessTicket) Expired(now int64) bool {
	return t.ExpiresAt <= now
}

// SignedAssetAccessTicket encodes an asset access ticket as
// `{base64payload}.{hexHMAC}`. It is intended for the `access` query parameter
// on stable asset URLs.
func SignedAssetAccessTicket(secret string, ticket AssetAccessTicket) (string, error) {
	if err := ticket.Validate(); err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(ticket)
	if err != nil {
		return "", fmt.Errorf("marshal asset ticket: %w", err)
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payloadB64))
	signature := hex.EncodeToString(h.Sum(nil)[:16])
	return payloadB64 + "." + signature, nil
}

// ParseSignedAssetAccessTicket verifies and decodes an asset access ticket.
func ParseSignedAssetAccessTicket(secret, signed string) (*AssetAccessTicket, error) {
	parts := strings.SplitN(signed, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid signed asset ticket format")
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
	var ticket AssetAccessTicket
	if err := json.Unmarshal(payloadJSON, &ticket); err != nil {
		return nil, fmt.Errorf("invalid payload JSON: %w", err)
	}
	if err := ticket.Validate(); err != nil {
		return nil, err
	}
	return &ticket, nil
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

	if err := validateTransformParams(params.Width, params.Height, params.Fit); err != nil {
		return nil, err
	}

	return &params, nil
}

func validateTransformParams(width, height int, fit string) error {
	if width < 1 || width > 2048 {
		return fmt.Errorf("width out of range [1, 2048]: %d", width)
	}
	if height < 1 || height > 2048 {
		return fmt.Errorf("height out of range [1, 2048]: %d", height)
	}
	if fit != "contain" && fit != "cover" && fit != "exact" {
		return fmt.Errorf("invalid fit mode: %s", fit)
	}
	return nil
}
