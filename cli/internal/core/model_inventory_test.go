package core

import (
	"slices"
	"testing"
)

func TestModelInventoryUsesStableKeys(t *testing.T) {
	want := []string{
		"chatto_core",
		"event_publisher",
		"config_model",
		"notification_preferences_model",
		"message_model",
		"reaction_model",
		"room_timeline_read_model",
		"read_state_model",
		"thread_follow_model",
		"room_model",
		"user_model",
		"rbac_model",
		"mentionables_model",
		"presence_model",
		"my_events_model",
		"call_model",
		"media_model",
		"asset_model",
	}

	got := ModelKeys()
	if !slices.Equal(got, want) {
		t.Fatalf("ModelKeys() = %q, want %q", got, want)
	}
	for _, key := range got {
		if !registryKeyPattern.MatchString(key) {
			t.Fatalf("registered model has invalid key %q", key)
		}
	}

	got[0] = "changed"
	if ModelKeys()[0] != want[0] {
		t.Fatal("ModelKeys() exposed mutable inventory state")
	}
}
