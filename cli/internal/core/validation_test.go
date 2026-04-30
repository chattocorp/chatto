package core

import (
	"strings"
	"testing"
)

func TestValidateDisplayName(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		wantErr     error
	}{
		// Valid names - basic
		{"simple ASCII", "John Doe", nil},
		{"single word", "Alice", nil},
		{"with hyphen", "Mary-Jane", nil},
		{"with apostrophe", "O'Brien", nil},
		{"with period", "Dr. Smith", nil},
		{"with underscore", "Cool_User", nil},
		{"with digits", "Player123", nil},

		// Valid names - international
		{"German umlaut", "Müller", nil},
		{"French accents", "François", nil},
		{"Spanish tilde", "Señor García", nil},
		{"Russian Cyrillic", "Иван Петров", nil},
		{"Japanese hiragana", "たなか", nil},
		{"Japanese kanji", "田中太郎", nil},
		{"Chinese characters", "王小明", nil},
		{"Korean hangul", "김철수", nil},
		{"Arabic script", "محمد علي", nil},
		{"Hebrew script", "דוד כהן", nil},
		{"Greek letters", "Αλέξανδρος", nil},
		{"Thai script", "สมชาย", nil},
		{"Hindi Devanagari", "राजेश कुमार", nil},

		// Valid names - emoji
		{"emoji suffix", "Alice 🚀", nil},
		{"trailing emoji after digit", "Player1 ⭐", nil},

		// Valid names - mixed scripts
		{"mixed Latin-Japanese", "John 田中", nil},
		{"mixed with emoji", "Müller 🎵", nil},

		// Valid edge cases
		{"empty string", "", nil}, // Empty check handled elsewhere
		{"single char", "A", nil},

		// Invalid - must start with letter or digit (avatar placeholder uses first char)
		{"emoji prefix", "🎮 Gamer", ErrDisplayNameInvalidStart},
		{"emoji only", "🦄", ErrDisplayNameInvalidStart},
		{"flag emoji prefix", "🇺🇸 American", ErrDisplayNameInvalidStart},
		{"single emoji", "😀", ErrDisplayNameInvalidStart},
		{"multiple emoji prefix", "🌟 Star ⭐", ErrDisplayNameInvalidStart},
		{"starts with hyphen", "-Alice", ErrDisplayNameInvalidStart},
		{"starts with apostrophe", "'Alice", ErrDisplayNameInvalidStart},
		{"starts with period", ".Alice", ErrDisplayNameInvalidStart},
		{"starts with underscore", "_Alice", ErrDisplayNameInvalidStart},
		{"starts with tilde", "~user", ErrDisplayNameInvalidStart},
		{"starts with backtick", "`code", ErrDisplayNameInvalidStart},
		{"starts with plus", "+Alice", ErrDisplayNameInvalidStart},
		{"starts with equals", "=Alice", ErrDisplayNameInvalidStart},
		{"starts with caret", "^Alice", ErrDisplayNameInvalidStart},
		{"starts with pipe", "|Alice", ErrDisplayNameInvalidStart},
		{"starts with angle bracket", "<Alice", ErrDisplayNameInvalidStart},

		// Invalid - control characters
		{"with newline", "John\nDoe", ErrDisplayNameInvalidCharacter},
		{"with tab", "John\tDoe", ErrDisplayNameInvalidCharacter},
		{"with carriage return", "John\rDoe", ErrDisplayNameInvalidCharacter},
		{"with null byte", "John\x00Doe", ErrDisplayNameInvalidCharacter},
		{"with bell", "John\x07Doe", ErrDisplayNameInvalidCharacter},

		// Invalid - zero-width characters
		{"with ZWSP", "John\u200BDoe", ErrDisplayNameInvalidCharacter},
		{"with ZWNJ", "John\u200CDoe", ErrDisplayNameInvalidCharacter},
		{"with ZWJ", "John\u200DDoe", ErrDisplayNameInvalidCharacter},
		{"with LTR mark", "John\u200EDoe", ErrDisplayNameInvalidCharacter},
		{"with RTL mark", "John\u200FDoe", ErrDisplayNameInvalidCharacter},
		{"with BOM", "John\uFEFFDoe", ErrDisplayNameInvalidCharacter},
		{"with word joiner", "John\u2060Doe", ErrDisplayNameInvalidCharacter},

		// Invalid - consecutive spaces
		{"double space", "John  Doe", ErrDisplayNameInvalidCharacter},
		{"triple space", "John   Doe", ErrDisplayNameInvalidCharacter},
		{"multiple double spaces", "A  B  C", ErrDisplayNameInvalidCharacter},

		// Disallowed punctuation - these are not in letter/digit/mark/symbol categories
		{"with curly brace", "John{test}", ErrDisplayNameInvalidCharacter},
		{"with semicolon", "John; DROP TABLE", ErrDisplayNameInvalidCharacter},
		{"with at sign", "user@domain", ErrDisplayNameInvalidCharacter},
		{"with hash", "Pre#hashtag", ErrDisplayNameInvalidCharacter},
		{"with exclamation", "Hello!", ErrDisplayNameInvalidCharacter},
		{"with question mark", "Who?", ErrDisplayNameInvalidCharacter},
		{"with comma", "Last, First", ErrDisplayNameInvalidCharacter},
		{"with colon", "Title: Name", ErrDisplayNameInvalidCharacter},
		{"with slash", "A/B", ErrDisplayNameInvalidCharacter},
		{"with backslash", "A\\B", ErrDisplayNameInvalidCharacter},
		{"with quotes", `Pre"quoted"`, ErrDisplayNameInvalidCharacter},
		{"with parentheses", "Pre(name)", ErrDisplayNameInvalidCharacter},
		{"with square brackets", "Pre[name]", ErrDisplayNameInvalidCharacter},
		{"with ampersand", "A&B", ErrDisplayNameInvalidCharacter},
		{"with asterisk", "star*", ErrDisplayNameInvalidCharacter},
		{"with percent", "100%", ErrDisplayNameInvalidCharacter},

		// These are Unicode symbols (allowed alongside emoji)
		// They're harmless when properly escaped in rendering, as long as
		// they're not the first character.
		{"with angle bracket (symbol)", "John<3", nil},
		{"with equals (symbol)", "a=b", nil},
		{"with pipe (symbol)", "A|B", nil},
		{"with caret (symbol)", "A^B", nil},
		{"with tilde (symbol)", "u~ser", nil},
		{"with backtick (symbol)", "co`de`", nil},
		{"with plus (symbol)", "A+B", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDisplayName(tt.displayName)
			if err != tt.wantErr {
				t.Errorf("ValidateDisplayName(%q) = %v, want %v", tt.displayName, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDisplayName_LongInternationalName(t *testing.T) {
	// A name that uses multi-byte characters but is under the 32-character limit
	// Japanese characters are typically 3 bytes each in UTF-8
	japaneseName := strings.Repeat("田", 30) // 90 bytes, 30 characters
	if err := ValidateDisplayName(japaneseName); err != nil {
		t.Errorf("ValidateDisplayName(%q) = %v, want nil", japaneseName, err)
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

func TestIsZeroWidthChar(t *testing.T) {
	zeroWidthChars := []rune{
		'\u200B', '\u200C', '\u200D', '\u200E', '\u200F',
		'\u2060', '\u2061', '\u2062', '\u2063', '\u2064',
		'\uFEFF',
	}

	for _, r := range zeroWidthChars {
		if !isZeroWidthChar(r) {
			t.Errorf("isZeroWidthChar(%U) = false, want true", r)
		}
	}

	normalChars := []rune{'A', 'z', '0', ' ', '田', '😀', '-', '_'}
	for _, r := range normalChars {
		if isZeroWidthChar(r) {
			t.Errorf("isZeroWidthChar(%U) = true, want false", r)
		}
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
