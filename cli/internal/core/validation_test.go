package core

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateDisplayName(t *testing.T) {
	valid := []string{
		"", // Creation paths can default an omitted name to the login.
		"John Doe", "Mary-Jane", "O'Brien", "Dr. Smith", "Cool_User", "Player123",
		"Müller", "François", "Иван Петров", "たなか", "王小明", "김철수", "محمد علي",
		"דוד כהן", "Αλέξανδρος", "สมชาย", "राजेश कुमार", "John 田中",
		"ChattoBot [DEV]", "[DEV] ChattoBot", "(away) Alice", "「田中」", "{Bot}",
		"John; DROP TABLE", "user@domain", "Hello!", "Last, First", "A/B", "A\\B",
		`Pre"quoted"`, "A&B", "star*", "100%", "John<3", "A+B", "co`de`",
		"!!!", "-Alice", "_Alice", "🎮 Gamer", "🦄", "🇺🇸", "🕹️", "👩‍💻",
		"👨‍👩‍👧‍👦", "1️⃣", "#️⃣", "🏴\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F",
		"John  Doe", "John   Doe", "A\u00A0B", "e\u0301", "\u0301Alice",
		"می\u200Cروم", "क्\u200Dष", strings.Repeat("田", 30),
	}
	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			if err := ValidateDisplayName(name); err != nil {
				t.Fatalf("ValidateDisplayName(%q) = %v, want nil", name, err)
			}
		})
	}
	invalid := []string{
		"John\nDoe", "John\tDoe", "John\rDoe", "John\x00Doe", "John\x07Doe", "John\u0085Doe",
		"John\u2028Doe", "John\u2029Doe",
		"John\u200BDoe", "John\u200EDoe", "John\u200FDoe", "John\uFEFFDoe", "John\u2060Doe",
		"John\u00ADDoe", "John\u061CDoe", "John\u202EDoe", "John\u2066Doe", "John\u2069Doe",
		" ", "  ", "\u0301", "\u200C\u200D", "\uFE0F", "\U000E0067\U000E007F",
		"\u115F", "\u1160", "\u3164", "\uFFA0", "\u2800",
	}
	for _, name := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := ValidateDisplayName(name); err != ErrDisplayNameInvalidCharacter {
				t.Fatalf("ValidateDisplayName(%q) = %v, want invalid character", name, err)
			}
		})
	}
}

func TestNormalizeDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no change needed", "Alice", "Alice"},
		{"trim leading space", " Alice", "Alice"},
		{"trim trailing space", "Alice ", "Alice"},
		{"trim both", " Alice ", "Alice"},
		{"trim multiple leading", "   Alice", "Alice"},
		{"trim multiple trailing", "Alice   ", "Alice"},
		{"preserve internal space", "John Doe", "John Doe"},
		{"empty string", "", ""},
		{"only spaces", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeDisplayName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeDisplayName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateLogin(t *testing.T) {
	tests := []struct {
		name    string
		login   string
		wantErr error
	}{
		// Valid logins
		{"simple lowercase", "alice", nil},
		{"with digits", "alice123", nil},
		{"with period", "alice.bob", nil},
		{"with underscore", "alice_bob", nil},
		{"with hyphen", "alice-bob", nil},
		{"mixed case", "Alice", nil},
		{"all digits except first letter", "a1234567890", nil},
		{"starts with digit", "1alice", nil},
		{"min length", "ab", nil},
		{"max length", strings.Repeat("a", MaxLoginLength), nil},

		// Invalid - too short
		{"empty", "", ErrLoginTooShort},
		{"single char", "a", ErrLoginTooShort},

		// Invalid - too long
		{"over max", strings.Repeat("a", MaxLoginLength+1), ErrLoginTooLong},

		// Invalid - starts with punctuation
		{"starts with period", ".alice", ErrLoginInvalidCharacter},
		{"starts with underscore", "_alice", ErrLoginInvalidCharacter},
		{"starts with hyphen", "-alice", ErrLoginInvalidCharacter},
		{"ends with period", "alice.", ErrLoginInvalidCharacter},
		{"ends with consecutive periods", "alice..", ErrLoginInvalidCharacter},
		{"minimum length ending with period", "a.", ErrLoginInvalidCharacter},

		// Invalid - disallowed characters
		{"with space", "alice bob", ErrLoginInvalidCharacter},
		{"with at sign", "alice@bob", ErrLoginInvalidCharacter},
		{"with exclamation", "alice!", ErrLoginInvalidCharacter},
		{"with slash", "alice/bob", ErrLoginInvalidCharacter},
		{"with emoji", "alice😀", ErrLoginInvalidCharacter},
		{"with unicode letter", "алиса", ErrLoginInvalidCharacter},
		{"with hash", "#alice", ErrLoginInvalidCharacter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogin(tt.login)
			if err != tt.wantErr {
				t.Errorf("ValidateLogin(%q) = %v, want %v", tt.login, err, tt.wantErr)
			}
		})
	}
}

func TestHasVisibleContent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Invisible content - should return false
		{"empty string", "", false},
		{"space only", " ", false},
		{"multiple spaces", "   ", false},
		{"tab only", "\t", false},
		{"newline only", "\n", false},
		{"mixed whitespace", " \t\n\r ", false},
		{"zero-width space only", "\u200B", false},
		{"multiple zero-width spaces", "\u200B\u200B\u200B", false},
		{"zero-width joiner only", "\u200D", false},
		{"zero-width non-joiner only", "\u200C", false},
		{"mixed zero-width chars", "\u200B\u200C\u200D", false},
		{"word joiner only", "\u2060", false},
		{"BOM only", "\uFEFF", false},
		{"soft hyphen only", "\u00AD", false},
		{"LTR mark only", "\u200E", false},
		{"RTL mark only", "\u200F", false},
		{"mixed invisible chars", "\u200B \u200C\t\u200D\n\u2060", false},
		{"whitespace and invisible chars", "  \u200B  \u200C  ", false},

		// Visible content - should return true
		{"single letter", "a", true},
		{"word", "hello", true},
		{"sentence", "Hello, world!", true},
		{"digits", "12345", true},
		{"emoji only", "😀", true},
		{"multiple emoji", "🎉🎊🎈", true},
		{"punctuation", "!!!", true},
		{"text with leading space", " hello", true},
		{"text with trailing space", "hello ", true},
		{"text with invisible chars mixed", "\u200Bhello\u200B", true},
		{"emoji with invisible chars", "\u200B😀\u200B", true},
		{"Japanese characters", "田中", true},
		{"Chinese characters", "你好", true},
		{"Arabic text", "مرحبا", true},
		{"Hebrew text", "שלום", true},
		{"Cyrillic text", "Привет", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasVisibleContent(tt.input)
			if got != tt.want {
				t.Errorf("HasVisibleContent(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func assertStringLengthError(t *testing.T, err error, field string, max int) {
	t.Helper()
	var lengthErr *StringLengthError
	if !errors.As(err, &lengthErr) {
		t.Fatalf("error = %v, want *StringLengthError", err)
	}
	if lengthErr.Field != field || lengthErr.Max != max {
		t.Fatalf("StringLengthError = {Field:%q Max:%d}, want {Field:%q Max:%d}", lengthErr.Field, lengthErr.Max, field, max)
	}
}
