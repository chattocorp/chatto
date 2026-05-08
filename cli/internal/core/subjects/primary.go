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
// should use the consolidated `server.>` shape.
//
// True for:
//   - The DM system space, unconditionally. The `SERVER_EVENTS` stream
//     is eager-created in core.newStorage, so by the time any caller
//     constructs a DM subject, the stream that accepts `server.>`
//     exists. DM is the primary-as-Server shape from day one — there
//     was never a per-space DM identity in the application model.
//   - The configured primary space, once SetPrimarySpaceID has been
//     called. Until then, primary-space callers (rare in tests, never
//     in production after #354 phase 4d) fall through to the legacy
//     `space.{id}.>` shape.
func shouldUseServerSubjects(spaceID string) bool {
	if spaceID == dmSpaceID {
		return true
	}
	if p := primarySpaceID.Load(); p != nil {
		return *p == spaceID
	}
	return false
}

// UsesServerSubjects is the public counterpart to shouldUseServerSubjects.
// Callers outside this package (notably core's stream routing) need to
// agree with subject construction about whether a given space is in the
// server-format world; exposing this predicate is the single source of
// truth for that decision.
func UsesServerSubjects(spaceID string) bool {
	return shouldUseServerSubjects(spaceID)
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
