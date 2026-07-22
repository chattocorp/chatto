package core

import (
	"strings"
	"testing"
)

func TestNewUserID(t *testing.T) {
	id := NewUserID()

	if !strings.HasPrefix(id, "U") {
		t.Errorf("NewUserID() should start with 'U', got %s", id)
	}

	// 1 prefix + 14 chars = 15
	if len(id) != 15 {
		t.Errorf("NewUserID() should be 15 characters, got %d", len(id))
	}
}

func TestNewRoomID(t *testing.T) {
	id := NewRoomID()

	if !strings.HasPrefix(id, "R") {
		t.Errorf("NewRoomID() should start with 'R', got %s", id)
	}

	if len(id) != 15 {
		t.Errorf("NewRoomID() should be 15 characters, got %d", len(id))
	}
}

func TestNewAssetID(t *testing.T) {
	id := NewAssetID()

	if !strings.HasPrefix(id, "A") {
		t.Errorf("NewAssetID() should start with 'A', got %s", id)
	}

	if len(id) != 15 {
		t.Errorf("NewAssetID() should be 15 characters, got %d", len(id))
	}
}

func TestNewEventID(t *testing.T) {
	id := NewEventID()

	if !strings.HasPrefix(id, "E") {
		t.Errorf("NewEventID() should start with 'E', got %s", id)
	}

	if len(id) != 15 {
		t.Errorf("NewEventID() should be 15 characters, got %d", len(id))
	}
}

func TestIDUniqueness(t *testing.T) {
	generated := make(map[string]bool)
	count := 1000

	for i := 0; i < count; i++ {
		id := NewUserID()
		if generated[id] {
			t.Errorf("Duplicate ID generated: %s", id)
		}
		generated[id] = true
	}

	if len(generated) != count {
		t.Errorf("Expected %d unique IDs, got %d", count, len(generated))
	}
}

func TestIDCharacters(t *testing.T) {
	validChars := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	id := NewUserID()
	// Skip prefix (first char), check rest
	for _, char := range id[1:] {
		if !strings.ContainsRune(validChars, char) {
			t.Errorf("Invalid character in ID: %c", char)
		}
	}
}
