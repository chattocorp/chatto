package core

import "testing"

func TestProjectionRegistryDrivesAdminStates(t *testing.T) {
	core, _ := setupTestCore(t)

	if len(core.projections) != 12 {
		t.Fatalf("registered projections = %d, want 12", len(core.projections))
	}

	registryNames := make(map[string]struct{}, len(core.projections))
	for _, projection := range core.projections {
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
		registryNames[projection.name] = struct{}{}
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

	states, err := core.ProjectionAdminStates(testContext(t))
	if err != nil {
		t.Fatalf("ProjectionAdminStates: %v", err)
	}
	if len(states) != len(core.projections) {
		t.Fatalf("admin states = %d, registered projections = %d", len(states), len(core.projections))
	}
	for _, state := range states {
		if _, ok := registryNames[state.Name]; !ok {
			t.Fatalf("admin state %q not found in projection registry", state.Name)
		}
		delete(registryNames, state.Name)
	}
	if len(registryNames) != 0 {
		t.Fatalf("registered projections missing admin states: %v", registryNames)
	}
}
