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

func (e *StringLengthError) Is(target error) bool {
	return target == ErrInvalidArgument
}

func validateStringMaxLength(field, value string, max int) error {
	if len(value) > max {
		return &StringLengthError{Field: field, Max: max}
	}
	return nil
}

// ValidateDisplayName accepts single-line Unicode presentation text. A name must
// contain a letter, number, punctuation mark, or symbol, not just combining marks
// or whitespace. Joiners and emoji tags are the only permitted format characters.
// Callers trim the name and check its length in Unicode code points separately.
// An empty name is permitted here for creation paths that default to the login;
// profile update paths reject empty names before they call this function.
func ValidateDisplayName(name string) error {
	if name == "" {
		return nil // Empty check is handled elsewhere
	}

	visible := false
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return ErrDisplayNameInvalidCharacter
		}
		if unicode.Is(unicode.Cf, r) && !isDisplayNameFormatChar(r) {
			return ErrDisplayNameInvalidCharacter
		}
		// Hangul fillers are letters and Braille blank is a symbol, but neither
		// supplies visible content. Joiners and variation selectors do not either.
		base := unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
		visible = visible || (base && r != '\u2800' && !unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r))
	}
	if !visible {
		return ErrDisplayNameInvalidCharacter
	}
	return nil
}

// isDisplayNameFormatChar permits script joiners and the emoji tag alphabet,
// including CANCEL TAG. Other format characters can conceal or reorder names.
func isDisplayNameFormatChar(r rune) bool {
	return r == '\u200C' || r == '\u200D' || (r >= '\U000E0020' && r <= '\U000E007F')
}

// NormalizeDisplayName trims whitespace and normalizes the display name.
// Returns the normalized name. Use ValidateDisplayName after normalizing.
func NormalizeDisplayName(name string) string {
	return strings.TrimSpace(name)
}

// ValidateLogin validates a login/username for allowed characters and length.
// Allowed: ASCII letters, digits, periods, underscores, hyphens.
// Must start with a letter or digit and must not end with a period.
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

	if strings.HasSuffix(login, ".") {
		return ErrLoginInvalidCharacter
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
