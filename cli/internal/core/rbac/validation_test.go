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
		// Valid role names — lowercase letters only
		{"simple name", "admin", nil},
		{"moderator", "moderator", nil},
		{"single char", "a", nil},
		{"max length", "abcdefghijklmnopqrstuvwxyzabcdef", nil}, // 32 chars

		// Invalid — contains numbers
		{"with numbers", "tier2", ErrInvalidRoleName},
		{"ends with number", "admin1", ErrInvalidRoleName},

		// Invalid — contains dashes
		{"with dashes", "power-user", ErrInvalidRoleName},
		{"hyphenated", "super-admin", ErrInvalidRoleName},
		{"legacy instance prefix", "instance-admin", ErrInvalidRoleName},

		// Invalid — other characters
		{"empty", "", ErrInvalidRoleName},
		{"starts with number", "2tier", ErrInvalidRoleName},
		{"starts with dash", "-admin", ErrInvalidRoleName},
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
