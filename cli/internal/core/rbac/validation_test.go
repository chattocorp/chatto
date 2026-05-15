package rbac

import (
	"errors"
	"testing"
)

func TestValidateRoleName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		// Valid role names — lowercase letters, digits, and dashes, must
		// start with a letter and end with a letter or digit.
		{"simple name", "admin", nil},
		{"moderator", "moderator", nil},
		{"single char", "a", nil},
		{"with numbers", "tier2", nil},
		{"ends with number", "admin1", nil},
		{"with dashes", "power-user", nil},
		{"hyphenated", "super-admin", nil},
		{"multiple dashes", "self-hosters-club", nil},
		{"max length", "abcdefghijklmnopqrstuvwxyzabcdef", nil}, // 32 chars

		// Invalid — format violations
		{"empty", "", ErrInvalidRoleName},
		{"starts with number", "2tier", ErrInvalidRoleName},
		{"starts with dash", "-admin", ErrInvalidRoleName},
		{"ends with dash", "admin-", ErrInvalidRoleName},
		{"uppercase", "Admin", ErrInvalidRoleName},
		{"mixed case", "PowerUser", ErrInvalidRoleName},
		{"spaces", "power user", ErrInvalidRoleName},
		{"underscore", "power_user", ErrInvalidRoleName},
		{"dot", "power.user", ErrInvalidRoleName},
		{"too long", "abcdefghijklmnopqrstuvwxyzabcdefg", ErrInvalidRoleName}, // 33 chars
		{"special chars", "admin!", ErrInvalidRoleName},
		{"unicode", "adminé", ErrInvalidRoleName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRoleName(tt.input)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("ValidateRoleName(%q) unexpected error = %v", tt.input, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateRoleName(%q) expected error %v, got nil", tt.input, tt.wantErr)
				} else if !errors.Is(err, tt.wantErr) {
					t.Errorf("ValidateRoleName(%q) error = %v, want %v", tt.input, err, tt.wantErr)
				}
			}
		})
	}
}
