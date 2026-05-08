package subjects

import "sync/atomic"

// Primary-aware dispatch for the #330 phase 4d subject migration.
//
// Each space-stream subject helper checks `shouldUseServerSubjects(spaceID)`
// and either produces the legacy per-space `space.{id}.>` shape (for
// non-primary, non-DM spaces) or the consolidated `server.>` shape (for the
// configured primary and the DM system space).
//
// Until `SetPrimarySpaceID` is called, the primary is unset and only the DM
// space ID matches the server-subject path. Production wiring calls
// `SetPrimarySpaceID` from `core.SetPrimarySpaceID` so the singleton tracks
// whichever space the deployment is using as primary.

// dmSpaceID is the well-known ID of the DM system space. Duplicated from the
// `core` package to keep the subjects package free of core imports.
const dmSpaceID = "DM"

// primarySpaceID stores the configured primary space ID. Pointer-of-string
// so the unset state is unambiguous.
var primarySpaceID atomic.Pointer[string]

// SetPrimarySpaceID records the deployment's primary space ID. Subsequent
// subject construction for that space (and for the DM system space) uses
// the consolidated `server.>` shape rather than the per-space `space.>`
// shape. Safe to call concurrently; safe to call repeatedly with the same
// value.
//
// Pass an empty string to clear the primary (used by tests).
func SetPrimarySpaceID(id string) {
	if id == "" {
		primarySpaceID.Store(nil)
		return
	}
	primarySpaceID.Store(&id)
}

// PrimarySpaceID returns the currently-set primary space ID, or "" if unset.
func PrimarySpaceID() string {
	if p := primarySpaceID.Load(); p != nil {
		return *p
	}
	return ""
}

// shouldUseServerSubjects reports whether subjects for the given space
// should use the consolidated `server.>` shape. True for the configured
// primary and for the DM system space, but ONLY once the primary has
// been set via SetPrimarySpaceID. Until then, all subjects (including
// DM) stay on the legacy `space.{id}.>` shape.
//
// Gating DM on the singleton is deliberate: the `SERVER_EVENTS` stream
// is created at the same time the primary is set. Auto-routing DM
// before SERVER_EVENTS exists would direct publishes to a subject no
// stream accepts.
func shouldUseServerSubjects(spaceID string) bool {
	p := primarySpaceID.Load()
	if p == nil {
		return false
	}
	return spaceID == dmSpaceID || spaceID == *p
}

// roomKind returns the kind segment that appears in `server.room.{kind}.>`
// subjects: "dm" for the DM system space, "channel" for everything else.
//
// Only meaningful when `shouldUseServerSubjects(spaceID)` is true; non-
// server-subject paths embed the spaceID directly and don't carry a kind
// segment.
func roomKind(spaceID string) string {
	if spaceID == dmSpaceID {
		return "dm"
	}
	return "channel"
}
