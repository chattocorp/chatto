package subjects

import "testing"

// Tests for the consolidated `server.>` subject namespace introduced in
// #330 phase 4d. The legacy tests in subjects_test.go cover the same
// functions when the primary singleton is unset; these tests exercise
// the server-format branch.
//
// The primary singleton is package-level state, so these tests do not
// run in parallel and use t.Cleanup to restore it.

// setPrimaryForTest installs primaryID for the duration of the test.
func setPrimaryForTest(t *testing.T, primaryID string) {
	t.Helper()
	prev := PrimarySpaceID()
	SetPrimarySpaceID(primaryID)
	t.Cleanup(func() { SetPrimarySpaceID(prev) })
}

func TestShouldUseServerSubjects(t *testing.T) {
	setPrimaryForTest(t, "Sprimary")

	for _, tc := range []struct {
		name    string
		spaceID string
		want    bool
	}{
		{"primary", "Sprimary", true},
		{"DM", "DM", true},
		{"non-primary", "Sother", false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldUseServerSubjects(tc.spaceID); got != tc.want {
				t.Errorf("shouldUseServerSubjects(%q) = %v, want %v", tc.spaceID, got, tc.want)
			}
		})
	}
}

func TestShouldUseServerSubjects_PrimaryUnset(t *testing.T) {
	// No setPrimaryForTest — singleton stays at whatever the test runner
	// inherited, then we explicitly clear it.
	SetPrimarySpaceID("")
	t.Cleanup(func() { SetPrimarySpaceID("") })

	// Until SetPrimarySpaceID is called, nothing routes to server
	// subjects — even DM stays on the legacy `space.DM.>` shape so that
	// publishes still land on the (still-existing) `SPACE_DM_EVENTS`
	// stream. Auto-routing DM before SERVER_EVENTS exists is what gets
	// you "nats: no response from stream" errors.
	if shouldUseServerSubjects("Sany") {
		t.Error("expected false for arbitrary space when primary unset")
	}
	if shouldUseServerSubjects("DM") {
		t.Error("DM should NOT route server-side until primary is set (SERVER_EVENTS doesn't exist yet)")
	}
}

func TestRoomKind(t *testing.T) {
	for _, tc := range []struct {
		spaceID string
		want    string
	}{
		{"DM", "dm"},
		{"Sprimary", "channel"},
		{"Sother", "channel"},
	} {
		if got := roomKind(tc.spaceID); got != tc.want {
			t.Errorf("roomKind(%q) = %q, want %q", tc.spaceID, got, tc.want)
		}
	}
}

// ===== Constructor tests (server format) =====

func TestServerFormat_Constructors(t *testing.T) {
	setPrimaryForTest(t, "Sprimary")

	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{"SpaceEvent primary strips member_ prefix", SpaceEvent("Sprimary", "member_deleted"), "server.member.deleted"},
		{"SpaceEvent DM strips member_ prefix", SpaceEvent("DM", "member_deleted"), "server.member.deleted"},
		{"SpaceEvent passes already-bare verb through", SpaceEvent("Sprimary", "joined"), "server.member.joined"},
		{"SpaceAllEvents primary", SpaceAllEvents("Sprimary"), "server.>"},
		{"SpaceRoomMessage primary channel", SpaceRoomMessage("Sprimary", "Rroom", "Eevt"), "server.room.channel.Rroom.msg.Eevt"},
		{"SpaceRoomMessage DM", SpaceRoomMessage("DM", "Rroom", "Eevt"), "server.room.dm.Rroom.msg.Eevt"},
		{"SpaceRoomThread primary", SpaceRoomThread("Sprimary", "Rroom", "Eroot", "Erep"), "server.room.channel.Rroom.msg.Eroot.replies.Erep"},
		{"SpaceRoomThreadFilter DM", SpaceRoomThreadFilter("DM", "Rroom", "Eroot"), "server.room.dm.Rroom.msg.Eroot.replies.>"},
		{"SpaceRoomThreadLookup primary", SpaceRoomThreadLookup("Sprimary", "Rroom", "Erep"), "server.room.channel.Rroom.msg.*.replies.Erep"},
		{"SpaceRoomAllThreads primary", SpaceRoomAllThreads("Sprimary", "Rroom"), "server.room.channel.Rroom.msg.*.replies.>"},
		{"SpaceRoomMeta primary", SpaceRoomMeta("Sprimary", "Rroom"), "server.room.channel.Rroom.meta"},
		{"SpaceRoomMeta DM", SpaceRoomMeta("DM", "Rroom"), "server.room.dm.Rroom.meta"},
		{"SpaceRoomAllMessages primary", SpaceRoomAllMessages("Sprimary", "Rroom"), "server.room.channel.Rroom.msg.>"},
		{"SpaceRoomRootMessages primary", SpaceRoomRootMessages("Sprimary", "Rroom"), "server.room.channel.Rroom.msg.*"},
		{"SpaceRoomAllEvents primary", SpaceRoomAllEvents("Sprimary", "Rroom"), "server.room.channel.Rroom.>"},
		{"SpaceRoomAllEvents DM", SpaceRoomAllEvents("DM", "Rroom"), "server.room.dm.Rroom.>"},
		{"SpaceAllRoomEvents primary", SpaceAllRoomEvents("Sprimary"), "server.room.channel.>"},
		{"SpaceAllRoomEvents DM", SpaceAllRoomEvents("DM"), "server.room.dm.>"},
		{"LiveSpaceAllEvents primary", LiveSpaceAllEvents("Sprimary"), "live.server.>"},
		{"LiveSpaceAllEvents DM", LiveSpaceAllEvents("DM"), "live.server.>"},
		{"LiveSpaceLevelEvents primary", LiveSpaceLevelEvents("Sprimary"), "live.server.member.>"},
		{"LiveSpaceEvent primary strips member_ prefix", LiveSpaceEvent("Sprimary", "member_deleted"), "live.server.member.deleted"},
		{"LiveSpaceRoomEvent primary", LiveSpaceRoomEvent("Sprimary", "Rroom", "reaction_added"), "live.server.room.channel.Rroom.reaction_added"},
		{"LiveSpaceRoomEvent DM", LiveSpaceRoomEvent("DM", "Rroom", "reaction_added"), "live.server.room.dm.Rroom.reaction_added"},
		{"LiveSpaceRoomAllEvents primary", LiveSpaceRoomAllEvents("Sprimary"), "live.server.room.channel.>"},
		{"LiveSpaceRoomReactionEvents primary", LiveSpaceRoomReactionEvents("Sprimary"), "live.server.room.channel.*.reaction_*"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestServerFormat_Filters(t *testing.T) {
	setPrimaryForTest(t, "Sprimary")

	gotRoot := SpaceRoomRootEventsFilters("Sprimary", "Rroom")
	wantRoot := []string{"server.room.channel.Rroom.msg.*", "server.room.channel.Rroom.meta"}
	if !equalStrings(gotRoot, wantRoot) {
		t.Errorf("SpaceRoomRootEventsFilters(primary): got %v, want %v", gotRoot, wantRoot)
	}

	gotAll := SpaceAllRoomEventsFilters("DM")
	wantAll := []string{"server.room.dm.*.msg.>", "server.room.dm.*.meta"}
	if !equalStrings(gotAll, wantAll) {
		t.Errorf("SpaceAllRoomEventsFilters(DM): got %v, want %v", gotAll, wantAll)
	}
}

// Non-primary spaces still use the legacy format even when primary is set.
func TestNonPrimary_StaysLegacy(t *testing.T) {
	setPrimaryForTest(t, "Sprimary")

	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{"SpaceRoomMessage", SpaceRoomMessage("Sother", "Rroom", "Eevt"), "space.Sother.room.Rroom.msg.Eevt"},
		{"SpaceRoomMeta", SpaceRoomMeta("Sother", "Rroom"), "space.Sother.room.Rroom.meta"},
		{"LiveSpaceAllEvents", LiveSpaceAllEvents("Sother"), "live.space.Sother.>"},
		{"SpaceAllEvents", SpaceAllEvents("Sother"), "space.Sother.>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

// ===== Parser tests (both formats) =====

func TestParsers_AcceptBothFormats(t *testing.T) {
	cases := []struct {
		name        string
		subject     string
		wantRoom    string
		wantEventID string
		wantRootID  string // empty if not a thread reply
		isRoot      bool
		isMeta      bool
		isThread    bool
	}{
		{
			name:        "legacy root message",
			subject:     "space.Sprimary.room.Rroom.msg.Eevt",
			wantRoom:    "Rroom",
			wantEventID: "Eevt",
			isRoot:      true,
		},
		{
			name:        "server root message channel",
			subject:     "server.room.channel.Rroom.msg.Eevt",
			wantRoom:    "Rroom",
			wantEventID: "Eevt",
			isRoot:      true,
		},
		{
			name:        "server root message DM",
			subject:     "server.room.dm.Rroom.msg.Eevt",
			wantRoom:    "Rroom",
			wantEventID: "Eevt",
			isRoot:      true,
		},
		{
			name:        "legacy thread reply",
			subject:     "space.Sprimary.room.Rroom.msg.Eroot.replies.Erep",
			wantRoom:    "Rroom",
			wantEventID: "Erep",
			wantRootID:  "Eroot",
			isThread:    true,
		},
		{
			name:        "server thread reply",
			subject:     "server.room.channel.Rroom.msg.Eroot.replies.Erep",
			wantRoom:    "Rroom",
			wantEventID: "Erep",
			wantRootID:  "Eroot",
			isThread:    true,
		},
		{
			name:     "legacy meta",
			subject:  "space.Sprimary.room.Rroom.meta",
			wantRoom: "Rroom",
			isMeta:   true,
		},
		{
			name:     "server meta DM",
			subject:  "server.room.dm.Rroom.meta",
			wantRoom: "Rroom",
			isMeta:   true,
		},
		{
			name:    "server space-level (not a room)",
			subject: "server.member.deleted",
		},
		{
			name:    "legacy space-level (not a room)",
			subject: "space.Sprimary.member_deleted",
		},
		{
			name:    "garbage",
			subject: "wat",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseRoomIDFromSubject(tc.subject); got != tc.wantRoom {
				t.Errorf("ParseRoomIDFromSubject = %q, want %q", got, tc.wantRoom)
			}
			if got := ParseEventIDFromSubject(tc.subject); got != tc.wantEventID {
				t.Errorf("ParseEventIDFromSubject = %q, want %q", got, tc.wantEventID)
			}
			gotRoot, gotOK := ParseThreadRootEventIDFromSubject(tc.subject)
			wantOK := tc.wantRootID != ""
			if gotOK != wantOK || gotRoot != tc.wantRootID {
				t.Errorf("ParseThreadRootEventIDFromSubject = (%q, %v), want (%q, %v)", gotRoot, gotOK, tc.wantRootID, wantOK)
			}
			if got := IsRootMessageSubject(tc.subject); got != tc.isRoot {
				t.Errorf("IsRootMessageSubject = %v, want %v", got, tc.isRoot)
			}
			if got := IsMetaSubject(tc.subject); got != tc.isMeta {
				t.Errorf("IsMetaSubject = %v, want %v", got, tc.isMeta)
			}
			if got := IsThreadSubject(tc.subject); got != tc.isThread {
				t.Errorf("IsThreadSubject = %v, want %v", got, tc.isThread)
			}
		})
	}
}

// ===== Round-trip: construct then parse =====

func TestRoundTrip_ServerFormat(t *testing.T) {
	setPrimaryForTest(t, "Sprimary")

	for _, tc := range []struct {
		name    string
		spaceID string
		roomID  string
		eventID string
	}{
		{"primary channel", "Sprimary", "Rroom1", "Eevt1"},
		{"DM", "DM", "Rroom2", "Eevt2"},
	} {
		t.Run(tc.name+"/root", func(t *testing.T) {
			subject := SpaceRoomMessage(tc.spaceID, tc.roomID, tc.eventID)
			if got := ParseRoomIDFromSubject(subject); got != tc.roomID {
				t.Errorf("room: got %q, want %q (subject=%q)", got, tc.roomID, subject)
			}
			if got := ParseEventIDFromSubject(subject); got != tc.eventID {
				t.Errorf("event: got %q, want %q (subject=%q)", got, tc.eventID, subject)
			}
			if !IsRootMessageSubject(subject) {
				t.Errorf("IsRootMessageSubject(%q) = false", subject)
			}
		})

		t.Run(tc.name+"/thread", func(t *testing.T) {
			subject := SpaceRoomThread(tc.spaceID, tc.roomID, "Eroot", tc.eventID)
			if got := ParseRoomIDFromSubject(subject); got != tc.roomID {
				t.Errorf("room: got %q, want %q (subject=%q)", got, tc.roomID, subject)
			}
			if got := ParseEventIDFromSubject(subject); got != tc.eventID {
				t.Errorf("event: got %q, want %q (subject=%q)", got, tc.eventID, subject)
			}
			gotRoot, ok := ParseThreadRootEventIDFromSubject(subject)
			if !ok || gotRoot != "Eroot" {
				t.Errorf("root: got (%q, %v) want (Eroot, true) (subject=%q)", gotRoot, ok, subject)
			}
			if !IsThreadSubject(subject) {
				t.Errorf("IsThreadSubject(%q) = false", subject)
			}
		})

		t.Run(tc.name+"/meta", func(t *testing.T) {
			subject := SpaceRoomMeta(tc.spaceID, tc.roomID)
			if got := ParseRoomIDFromSubject(subject); got != tc.roomID {
				t.Errorf("room: got %q, want %q (subject=%q)", got, tc.roomID, subject)
			}
			if !IsMetaSubject(subject) {
				t.Errorf("IsMetaSubject(%q) = false", subject)
			}
		})
	}
}

// ===== Singleton behavior =====

func TestSetPrimarySpaceID_ClearsOnEmpty(t *testing.T) {
	setPrimaryForTest(t, "Sprimary")
	if PrimarySpaceID() != "Sprimary" {
		t.Fatalf("expected primary to be Sprimary, got %q", PrimarySpaceID())
	}
	SetPrimarySpaceID("")
	if PrimarySpaceID() != "" {
		t.Errorf("expected primary cleared, got %q", PrimarySpaceID())
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
