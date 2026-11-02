package memory

import (
	"encoding/json"
	"errors"
)

// listingPage preserves means metadata and diagnostics, adding an explicit
// continuation offset only when a nonempty bounded page has more matches.
// Listing never populates read hashes and therefore cannot authorize updates.
func listingPage(out []byte, offset, limit int) (string, error) {
	var page struct {
		Documents   []json.RawMessage `json:"documents"`
		Diagnostics []json.RawMessage `json:"diagnostics"`
		HasMore     *bool             `json:"has_more"`
	}
	if json.Unmarshal(out, &page) != nil || page.Documents == nil || page.Diagnostics == nil || page.HasMore == nil || len(page.Documents) > limit || (*page.HasMore && len(page.Documents) == 0) {
		return "", errors.New("invalid collection listing")
	}
	result := map[string]any{"documents": page.Documents, "diagnostics": page.Diagnostics, "has_more": *page.HasMore, "offset": offset, "limit": limit}
	if *page.HasMore {
		result["next_offset"] = offset + len(page.Documents)
	}
	data, err := json.Marshal(result)
	return string(data), err
}
