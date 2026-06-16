package core

import "testing"

func TestServiceRegistryUsesStableKeys(t *testing.T) {
	core, _ := setupTestCore(t)

	services := core.ServiceMetadata()
	if len(services) != 13 {
		t.Fatalf("registered services = %d, want 13", len(services))
	}

	keys := make(map[string]string, len(services))
	names := make(map[string]struct{}, len(services))
	for _, service := range services {
		if service.Key == "" {
			t.Fatal("registered service has empty key")
		}
		if !registryKeyPattern.MatchString(service.Key) {
			t.Fatalf("registered service %q has invalid key %q", service.Name, service.Key)
		}
		if service.Name == "" {
			t.Fatalf("registered service %q has empty name", service.Key)
		}
		if existingName, exists := keys[service.Key]; exists {
			t.Fatalf("duplicate service registration key %q for %q and %q", service.Key, existingName, service.Name)
		}
		if _, exists := names[service.Name]; exists {
			t.Fatalf("duplicate service registration name %q", service.Name)
		}
		keys[service.Key] = service.Name
		names[service.Name] = struct{}{}
	}

	for key, name := range map[string]string{
		"chatto_core":          "Chatto Core",
		"event_publisher":      "Event Publisher",
		"config_service":       "Config Service",
		"config_manager":       "Config Manager",
		"room_service":         "Room Service",
		"user_service":         "User Service",
		"rbac_service":         "RBAC Service",
		"mentionables_service": "Mentionables Service",
		"presence_service":     "Presence Service",
		"my_events_service":    "My Events Service",
		"call_service":         "Call Service",
		"media_service":        "Media Service",
		"asset_service":        "Asset Service",
	} {
		if got, ok := keys[key]; !ok || got != name {
			t.Fatalf("service registration %q = %q, %v; want %q, true", key, got, ok, name)
		}
	}
}
