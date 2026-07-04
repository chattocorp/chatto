package core

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

const (
	idAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	idLength   = 14
)

// newID generates a new unique ID using NanoID with the given prefix.
func newID(prefix string) string {
	id, err := gonanoid.Generate(idAlphabet, idLength)
	if err != nil {
		log.Fatal("Failed to generate ID", "error", err)
	}
	return prefix + id
}

// NewUserID generates a new user ID with "U" prefix.
func NewUserID() string {
	return newID("U")
}

// NewSpaceID generates a new space ID with "S" prefix.
func NewSpaceID() string {
	return newID("S")
}

// NewRoomID generates a new room ID with "R" prefix.
func NewRoomID() string {
	return newID("R")
}

// NewCallID generates a new voice call session ID with "C" prefix.
func NewCallID() string {
	return newID("C")
}

// NewRoomGroupID generates a new room-group ID with "G" prefix.
func NewRoomGroupID() string {
	return newID("G")
}

// NewSidebarLinkID generates a new sidebar-link ID with "L" prefix.
func NewSidebarLinkID() string {
	return newID("L")
}

// NewAssetID generates a new asset ID with "A" prefix.
func NewAssetID() string {
	return newID("A")
}

// NewPasswordResetToken generates a new password reset token with "PR" prefix.
func NewPasswordResetToken() string {
	return newID("PR")
}

// NewRegistrationToken generates a new registration completion token with "RG" prefix.
func NewRegistrationToken() string {
	return newID("RG")
}

// NewExternalIdentityCreateToken generates a pending external-identity account creation token.
func NewExternalIdentityCreateToken() string {
	return newID("EC")
}

// NewExternalIdentityLinkToken generates a pending external-identity account linking token.
func NewExternalIdentityLinkToken() string {
	return newID("EL")
}

// NewExternalIdentityLinkStartToken generates a provider-link browser handoff token.
func NewExternalIdentityLinkStartToken() string {
	return newID("ELS")
}

// NewVerificationCode generates a six-digit numeric code for email verification.
func NewVerificationCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", fmt.Errorf("generate verification code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// NewAccountDeletionToken generates a new account deletion confirmation token with "AD" prefix.
func NewAccountDeletionToken() string {
	return newID("AD")
}

// NewEventID generates a new event ID with "E" prefix.
func NewEventID() string {
	return newID("E")
}

// NewNotificationID generates a new notification ID with "N" prefix.
func NewNotificationID() string {
	return newID("N")
}

// NewAuthToken generates a new bearer auth token with "cht_AT" prefix.
// The "cht_" prefix makes tokens recognizable in logs and password managers.
func NewAuthToken() string {
	return "cht_" + newID("AT")
}

// NewLinkPreviewToken generates a composer link-preview token with "cht_LP" prefix.
func NewLinkPreviewToken() string {
	return "cht_" + newID("LP")
}

// NewCookieSessionID generates a new opaque cookie session ID with "cht_CS" prefix.
func NewCookieSessionID() string {
	return "cht_" + newID("CS")
}

// NewAuthCode generates a new OAuth authorization code with "cht_AC" prefix.
func NewAuthCode() string {
	return "cht_" + newID("AC")
}
