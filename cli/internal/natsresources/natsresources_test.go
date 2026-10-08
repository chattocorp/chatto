package natsresources

import "testing"

func TestStreamNames(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		resource Resource
		want     string
	}{
		{Resource{Name: EVT, Kind: KindStream}, "EVT"},
		{Resource{Name: RuntimeState, Kind: KindKeyValue}, "KV_RUNTIME_STATE"},
		{Resource{Name: ServerAssets, Kind: KindObjectStore}, "OBJ_SERVER_ASSETS"},
	} {
		if got := tt.resource.StreamName(); got != tt.want {
			t.Errorf("%s stream name = %q, want %q", tt.resource.Name, got, tt.want)
		}
	}
}

func TestEveryResourceHasAConsistentBackupPolicy(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool)
	for _, resources := range [][]Resource{current, legacy} {
		for _, resource := range resources {
			if seen[resource.StreamName()] {
				t.Errorf("%s is registered twice", resource.StreamName())
			}
			seen[resource.StreamName()] = true
			skips := resource.Backup != BackupInclude
			if skips != (resource.SkipReason != "") {
				t.Errorf("%s has backup policy %v and skip reason %q", resource.Name, resource.Backup, resource.SkipReason)
			}
		}
	}
}

func TestForStreamFindsLegacyResources(t *testing.T) {
	t.Parallel()

	resource, ok := ForStream("KV_AUTH_TOKENS")
	if !ok || resource.Backup != BackupSkip {
		t.Fatalf("ForStream(KV_AUTH_TOKENS) = %+v, %v; want a skipped legacy bucket", resource, ok)
	}
	if _, ok := ForStream("SPACE_abc123_EVENTS"); ok {
		t.Fatal("ForStream found an unknown stream")
	}
	for _, resource := range Current() {
		if resource.Name == "AUTH_TOKENS" {
			t.Fatal("Current includes a legacy resource")
		}
	}
}
