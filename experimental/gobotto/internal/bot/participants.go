package bot

import (
	"context"
	"slices"
	"time"
)

// participantProfile is a separate model-context projection. Profile changes do
// not change message fingerprints or the durable inbox. Names are untrusted data;
// only the server user ID participates in authorization.
type participantProfile struct {
	UserID      string `json:"userId"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Resolved    bool   `json:"resolved"`
}

// resolveParticipants refreshes triggering authors each turn and thread authors
// when context loads. Missing/failed lookups remain explicit, without blocking a
// reply or falling back to stale profile data. Only names and IDs reach the model.
func resolveParticipants(ctx context.Context, client Client, ids []string) []participantProfile {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	ids = slices.DeleteFunc(ids, func(id string) bool { return id == "" })
	profiles := make([]participantProfile, 0, len(ids))
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for offset := 0; offset < len(ids); offset += 100 {
		batch := ids[offset:min(offset+100, len(ids))]
		result, err := client.Profiles(ctx, batch)
		for _, id := range batch {
			entry := participantProfile{UserID: id}
			if profile, ok := result[id]; err == nil && ok && profile.ID == id {
				entry.Username, entry.DisplayName, entry.Resolved = profile.Login, profile.DisplayName, true
			}
			profiles = append(profiles, entry)
		}
	}
	return profiles
}
