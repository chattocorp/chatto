package core

import (
	"fmt"
	"strings"
	"unicode"
)

// StringLengthError is returned when a persisted user-controlled string exceeds
// the field's durable storage limit.
type StringLengthError struct {
	Field string
	Max   int
}

func (e *StringLengthError) Error() string {
	return fmt.Sprintf("%s cannot exceed %d bytes", e.Field, e.Max)
}

func validateStringMaxLength(field, value string, max int) error {
	if len(value) > max {
		return &StringLengthError{Field: field, Max: max}
	}
	return nil
}

// ValidateDisplayName validates a display name for allowed characters.
// Allowed: letters (any script), digits, marks (diacritics), emoji/symbols,
// space, hyphen, apostrophe, period, underscore.
// Disallowed: control characters, zero-width characters, consecutive spaces.
//
// This function does NOT check length - callers should check len(name) <= MaxDisplayNameLength separately.
// The name should be trimmed before calling this function.
func ValidateDisplayName(name string) error {
	if name == "" {
		return nil // Empty check is handled elsewhere
	}

	prevWasSpace := false
	for i, r := range name {
		if i == 0 && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return ErrDisplayNameInvalidStart
		}
		// Check for consecutive spaces
		if r == ' ' {
			if prevWasSpace {
				return ErrDisplayNameInvalidCharacter
			}
			prevWasSpace = true
			continue
		}
		prevWasSpace = false

		// Check for zero-width characters
		if isZeroWidthChar(r) {
			return ErrDisplayNameInvalidCharacter
		}

		// Check for control characters
		if unicode.IsControl(r) {
			return ErrDisplayNameInvalidCharacter
		}

		// Whitelist: letters, digits, marks (diacritics), symbols (includes emoji),
		// and common name punctuation
		if unicode.IsLetter(r) ||
			unicode.IsDigit(r) ||
			unicode.IsMark(r) ||
			unicode.IsSymbol(r) ||
			r == '-' || r == '\'' || r == '.' || r == '_' {
			continue
		}

		return ErrDisplayNameInvalidCharacter
	}
	return nil
}

// isZeroWidthChar returns true for zero-width and invisible formatting characters
// that could cause display confusion.
func isZeroWidthChar(r rune) bool {
	switch r {
	case '\u200B', // Zero Width Space
		'\u200C', // Zero Width Non-Joiner
		'\u200D', // Zero Width Joiner
		'\u200E', // Left-to-Right Mark
		'\u200F', // Right-to-Left Mark
		'\u2060', // Word Joiner
		'\u2061', // Function Application
		'\u2062', // Invisible Times
		'\u2063', // Invisible Separator
		'\u2064', // Invisible Plus
		'\uFEFF': // Byte Order Mark / Zero Width No-Break Space
		return true
	}
	return false
}

// NormalizeDisplayName trims whitespace and normalizes the display name.
// Returns the normalized name. Use ValidateDisplayName after normalizing.
func NormalizeDisplayName(name string) string {
	return strings.TrimSpace(name)
}

// ValidateLogin validates a login/username for allowed characters and length.
// Allowed: ASCII letters, digits, periods, underscores, hyphens.
// Must start with a letter or digit.
// Length: MinLoginLength to MaxLoginLength characters.
func ValidateLogin(login string) error {
	if len(login) < MinLoginLength {
		return ErrLoginTooShort
	}
	if len(login) > MaxLoginLength {
		return ErrLoginTooLong
	}

	for i, r := range login {
		isLetterOrDigit := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')

		if i == 0 && !isLetterOrDigit {
			return ErrLoginInvalidCharacter
		}

		if !isLetterOrDigit && r != '.' && r != '_' && r != '-' {
			return ErrLoginInvalidCharacter
		}
	}

	return nil
}

// ValidatePassword validates that a password meets length requirements.
// Length is measured in bytes (len), not Unicode characters, so the upper bound
// also bounds the work done by bcrypt and storage cost.
func ValidatePassword(password string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if len(password) > MaxPasswordLength {
		return ErrPasswordTooLong
	}
	return nil
}

// HasVisibleContent returns true if the string contains at least one visible character.
// A visible character is one that is not whitespace, not a format character (Cf),
// and not a control character (Cc).
//
// This is used to validate message content, rejecting messages that contain only
// invisible Unicode characters like zero-width spaces (U+200B), zero-width joiners
// (U+200C, U+200D), soft hyphens (U+00AD), word joiners (U+2060), etc.
func HasVisibleContent(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) && !unicode.Is(unicode.Cf, r) && !unicode.Is(unicode.Cc, r) {
			return true
		}
	}
	return false
}
