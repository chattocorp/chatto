package core

import (
	"regexp"
	"testing"

	"hmans.de/chatto/internal/events"
)

var registryKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func TestProjectionRegistryDrivesAdminStates(t *testing.T) {
	core, _ := setupTestCore(t)

	if len(core.projections) != 13 {
		t.Fatalf("registered projections = %d, want 13", len(core.projections))
	}

	registryNames := make(map[string]struct{}, len(core.projections))
	registryKeys := make(map[string]string, len(core.projections))
	for _, projection := range core.projections {
		if projection.key == "" {
			t.Fatal("registered projection has empty key")
		}
		if !registryKeyPattern.MatchString(projection.key) {
			t.Fatalf("registered projection %q has invalid key %q", projection.name, projection.key)
		}
		if projection.name == "" {
			t.Fatal("registered projection has empty name")
		}
		if projection.projector == nil {
			t.Fatalf("projection %q has nil projector", projection.name)
		}
		if projection.estimate == nil {
			t.Fatalf("projection %q has nil estimate", projection.name)
		}
		if _, exists := registryNames[projection.name]; exists {
			t.Fatalf("duplicate projection registration name %q", projection.name)
		}
		if existingName, exists := registryKeys[projection.key]; exists {
			t.Fatalf("duplicate projection registration key %q for %q and %q", projection.key, existingName, projection.name)
		}
		registryNames[projection.name] = struct{}{}
		registryKeys[projection.key] = projection.name
	}

	if got, ok := registryKeys["content_keys"]; !ok || got != "Content Keys" {
		t.Fatalf("content_keys projection registration = %q, %v; want Content Keys, true", got, ok)
	}
	if _, ok := registryNames["Content Keys"]; !ok {
		t.Fatal("Content Keys projection is not registered")
	}
	if _, ok := registryNames["Room Directory"]; !ok {
		t.Fatal("Room Directory projection is not registered")
	}
	if _, ok := registryNames["Room Group Layout"]; !ok {
		t.Fatal("Room Group Layout projection is not registered")
	}
	if _, ok := registryNames["Call State"]; !ok {
		t.Fatal("Call State projection is not registered")
	}
	if _, ok := registryNames["Assets"]; !ok {
		t.Fatal("Assets projection is not registered")
	}
	if _, ok := registryNames["Mentionables"]; !ok {
		t.Fatal("Mentionables projection is not registered")
	}
	if _, ok := registryNames["User Auth"]; !ok {
		t.Fatal("User Auth projection is not registered")
	}

	states, err := core.ProjectionAdminStates(testContext(t))
	if err != nil {
		t.Fatalf("ProjectionAdminStates: %v", err)
	}
	if len(states) != len(core.projections) {
		t.Fatalf("admin states = %d, registered projections = %d", len(states), len(core.projections))
	}
	for _, state := range states {
		if state.Key == "" {
			t.Fatalf("admin state %q has empty key", state.Name)
		}
		if !registryKeyPattern.MatchString(state.Key) {
			t.Fatalf("admin state %q has invalid key %q", state.Name, state.Key)
		}
		if gotName, ok := registryKeys[state.Key]; !ok || gotName != state.Name {
			t.Fatalf("admin state key/name %q/%q not found in projection registry", state.Key, state.Name)
		}
		if _, ok := registryNames[state.Name]; !ok {
			t.Fatalf("admin state %q not found in projection registry", state.Name)
		}
		delete(registryNames, state.Name)
	}
	if len(registryNames) != 0 {
		t.Fatalf("registered projections missing admin states: %v", registryNames)
	}
}

func TestProjectionRegistryDefinesIndependentConsumers(t *testing.T) {
	core, _ := setupTestCore(t)

	seen := make(map[*events.Projector]string, len(core.projections))
	for _, registration := range core.projections {
		if previous, duplicate := seen[registration.projector]; duplicate {
			t.Fatalf("%s and %s share one projector", previous, registration.name)
		}
		seen[registration.projector] = registration.name
		if replaySubjects := registration.projector.ReplaySubjects(); len(replaySubjects) == 0 {
			t.Fatalf("%s projection has no physical replay filter", registration.name)
		}
	}
}
