package core

import "testing"

func TestModelInventoryUsesStableKeys(t *testing.T) {
	core, _ := setupTestCore(t)

	models := core.ModelMetadata()
	if len(models) != 19 {
		t.Fatalf("registered models = %d, want 19", len(models))
	}

	keys := make(map[string]string, len(models))
	legacyKeys := make(map[string]string, len(models))
	names := make(map[string]struct{}, len(models))
	for _, model := range models {
		if model.Key == "" {
			t.Fatal("registered model has empty key")
		}
		if !registryKeyPattern.MatchString(model.Key) {
			t.Fatalf("registered model %q has invalid key %q", model.Name, model.Key)
		}
		if model.Name == "" {
			t.Fatalf("registered model %q has empty name", model.Key)
		}
		if model.LegacyServiceKey == "" {
			t.Fatalf("registered model %q has empty legacy service key", model.Key)
		}
		if !registryKeyPattern.MatchString(model.LegacyServiceKey) {
			t.Fatalf("registered model %q has invalid legacy service key %q", model.Name, model.LegacyServiceKey)
		}
		if existingName, exists := keys[model.Key]; exists {
			t.Fatalf("duplicate model registration key %q for %q and %q", model.Key, existingName, model.Name)
		}
		if existingName, exists := legacyKeys[model.LegacyServiceKey]; exists {
			t.Fatalf("duplicate legacy service key %q for %q and %q", model.LegacyServiceKey, existingName, model.Name)
		}
		if _, exists := names[model.Name]; exists {
			t.Fatalf("duplicate model registration name %q", model.Name)
		}
		keys[model.Key] = model.Name
		legacyKeys[model.LegacyServiceKey] = model.Name
		names[model.Name] = struct{}{}
	}

	for key, name := range map[string]string{
		"chatto_core":                    "Chatto Core",
		"event_publisher":                "Event Publisher",
		"config_model":                   "Config Model",
		"config_manager":                 "Config Manager",
		"notification_preferences_model": "Notification Preferences Model",
		"message_model":                  "Message Model",
		"reaction_model":                 "Reaction Model",
		"room_timeline_read_model":       "Room Timeline Read Model",
		"read_state_model":               "Read State Model",
		"thread_follow_model":            "Thread Follow Model",
		"room_model":                     "Room Model",
		"user_model":                     "User Model",
		"rbac_model":                     "RBAC Model",
		"mentionables_model":             "Mentionables Model",
		"presence_model":                 "Presence Model",
		"my_events_model":                "My Events Model",
		"call_model":                     "Call Model",
		"media_model":                    "Media Model",
		"asset_model":                    "Asset Model",
	} {
		if got, ok := keys[key]; !ok || got != name {
			t.Fatalf("model registration %q = %q, %v; want %q, true", key, got, ok, name)
		}
	}

	for key, name := range map[string]string{
		"config_service":                   "Config Model",
		"notification_preferences_service": "Notification Preferences Model",
		"message_service":                  "Message Model",
		"reaction_service":                 "Reaction Model",
		"room_timeline_read_service":       "Room Timeline Read Model",
		"read_state_service":               "Read State Model",
		"thread_follow_service":            "Thread Follow Model",
		"room_service":                     "Room Model",
		"user_service":                     "User Model",
		"rbac_service":                     "RBAC Model",
		"mentionables_service":             "Mentionables Model",
		"presence_service":                 "Presence Model",
		"my_events_service":                "My Events Model",
		"call_service":                     "Call Model",
		"media_service":                    "Media Model",
		"asset_service":                    "Asset Model",
	} {
		if got, ok := legacyKeys[key]; !ok || got != name {
			t.Fatalf("legacy service key %q = %q, %v; want %q, true", key, got, ok, name)
		}
	}
}
