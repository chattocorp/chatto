package subjects

import "testing"

// Per ADR-029 (PR 6 of the Phase 2 refactor) all stored event subjects drop
// the `space.{spaceId}.` prefix. Old form: `space.{s}.room.{r}.msg.{e}`.
// New form: `room.{r}.msg.{e}`. The compatibility shim functions still take
// a spaceID parameter; the parameter is ignored.

func TestSpaceRoomMessage(t *testing.T) {
	tests := []struct {
		name     string
		spaceID  string
		roomID   string
		eventID  string
		expected string
	}{
		{
			name:     "basic message subject",
			spaceID:  "space1",
			roomID:   "room1",
			eventID:  "evt123",
			expected: "room.room1.msg.evt123",
		},
		{
			name:     "with nanoid-style IDs",
			spaceID:  "Sp6IQDs4Hm6gLIb",
			roomID:   "R7IFBV0AV1UBYTK",
			eventID:  "E8ShdnxI4BouAIl",
			expected: "room.R7IFBV0AV1UBYTK.msg.E8ShdnxI4BouAIl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SpaceRoomMessage(tt.spaceID, tt.roomID, tt.eventID)
			if got != tt.expected {
				t.Errorf("SpaceRoomMessage() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSpaceRoomThread(t *testing.T) {
	tests := []struct {
		name        string
		spaceID     string
		roomID      string
		rootEventID string
		eventID     string
		expected    string
	}{
		{
			name:        "basic thread subject",
			spaceID:     "space1",
			roomID:      "room1",
			rootEventID: "evt123",
			eventID:     "evt456",
			expected:    "room.room1.msg.evt123.replies.evt456",
		},
		{
			name:        "with nanoid-style IDs",
			spaceID:     "Sp6IQDs4Hm6gLIb",
			roomID:      "R7IFBV0AV1UBYTK",
			rootEventID: "E7RootEventId",
			eventID:     "E8ShdnxI4BouAIl",
			expected:    "room.R7IFBV0AV1UBYTK.msg.E7RootEventId.replies.E8ShdnxI4BouAIl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SpaceRoomThread(tt.spaceID, tt.roomID, tt.rootEventID, tt.eventID)
			if got != tt.expected {
				t.Errorf("SpaceRoomThread() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSpaceRoomThreadFilter(t *testing.T) {
	got := SpaceRoomThreadFilter("space1", "room1", "evt123")
	expected := "room.room1.msg.evt123.replies.>"
	if got != expected {
		t.Errorf("SpaceRoomThreadFilter() = %q, want %q", got, expected)
	}
}

func TestSpaceRoomThreadLookup(t *testing.T) {
	got := SpaceRoomThreadLookup("space1", "room1", "evt456")
	expected := "room.room1.msg.*.replies.evt456"
	if got != expected {
		t.Errorf("SpaceRoomThreadLookup() = %q, want %q", got, expected)
	}
}

func TestSpaceRoomAllThreads(t *testing.T) {
	got := SpaceRoomAllThreads("space1", "room1")
	expected := "room.room1.msg.*.replies.>"
	if got != expected {
		t.Errorf("SpaceRoomAllThreads() = %q, want %q", got, expected)
	}
}

func TestSpaceRoomRootMessages(t *testing.T) {
	got := SpaceRoomRootMessages("space1", "room1")
	expected := "room.room1.msg.*"
	if got != expected {
		t.Errorf("SpaceRoomRootMessages() = %q, want %q", got, expected)
	}
}

func TestSpaceRoomAllEvents(t *testing.T) {
	got := SpaceRoomAllEvents("space1", "room1")
	expected := "room.room1.>"
	if got != expected {
		t.Errorf("SpaceRoomAllEvents() = %q, want %q", got, expected)
	}
}

func TestParseRoomIDFromSubject(t *testing.T) {
	tests := []struct {
		name     string
		subject  string
		expected string
	}{
		{
			name:     "root message",
			subject:  "room.room1.msg.evt123",
			expected: "room1",
		},
		{
			name:     "thread reply",
			subject:  "room.room1.msg.evt123.replies.evt456",
			expected: "room1",
		},
		{
			name:     "meta event",
			subject:  "room.room1.meta",
			expected: "room1",
		},
		{
			name:     "server-level event (joined) is not a room event",
			subject:  "joined",
			expected: "",
		},
		{
			name:     "server-level event (member_deleted) is not a room event",
			subject:  "member_deleted",
			expected: "",
		},
		{
			name:     "invalid subject",
			subject:  "invalid.subject",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRoomIDFromSubject(tt.subject)
			if got != tt.expected {
				t.Errorf("ParseRoomIDFromSubject(%q) = %q, want %q", tt.subject, got, tt.expected)
			}
		})
	}
}

func TestParseThreadRootEventIDFromSubject(t *testing.T) {
	tests := []struct {
		name            string
		subject         string
		expectedEventID string
		expectedOK      bool
	}{
		{
			name:            "thread reply",
			subject:         "room.room1.msg.evt123.replies.evt456",
			expectedEventID: "evt123",
			expectedOK:      true,
		},
		{
			name:            "root message",
			subject:         "room.room1.msg.evt123",
			expectedEventID: "",
			expectedOK:      false,
		},
		{
			name:            "meta event",
			subject:         "room.room1.meta",
			expectedEventID: "",
			expectedOK:      false,
		},
		{
			name:            "nanoid-style IDs",
			subject:         "room.R7IFBV0AV1UBYTK.msg.E7RootEventId.replies.E8ShdnxI4BouAIl",
			expectedEventID: "E7RootEventId",
			expectedOK:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventID, ok := ParseThreadRootEventIDFromSubject(tt.subject)
			if eventID != tt.expectedEventID || ok != tt.expectedOK {
				t.Errorf("ParseThreadRootEventIDFromSubject(%q) = (%q, %v), want (%q, %v)", tt.subject, eventID, ok, tt.expectedEventID, tt.expectedOK)
			}
		})
	}
}

func TestIsRootMessageSubject(t *testing.T) {
	tests := []struct {
		name     string
		subject  string
		expected bool
	}{
		{
			name:     "root message",
			subject:  "room.room1.msg.evt123",
			expected: true,
		},
		{
			name:     "thread reply",
			subject:  "room.room1.msg.evt123.replies.evt456",
			expected: false,
		},
		{
			name:     "meta event",
			subject:  "room.room1.meta",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRootMessageSubject(tt.subject)
			if got != tt.expected {
				t.Errorf("IsRootMessageSubject(%q) = %v, want %v", tt.subject, got, tt.expected)
			}
		})
	}
}

func TestIsMetaSubject(t *testing.T) {
	tests := []struct {
		name     string
		subject  string
		expected bool
	}{
		{
			name:     "meta event",
			subject:  "room.room1.meta",
			expected: true,
		},
		{
			name:     "root message",
			subject:  "room.room1.msg.evt123",
			expected: false,
		},
		{
			name:     "thread reply",
			subject:  "room.room1.msg.evt123.replies.evt456",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMetaSubject(tt.subject)
			if got != tt.expected {
				t.Errorf("IsMetaSubject(%q) = %v, want %v", tt.subject, got, tt.expected)
			}
		})
	}
}

func TestIsThreadSubject(t *testing.T) {
	tests := []struct {
		name     string
		subject  string
		expected bool
	}{
		{
			name:     "thread reply",
			subject:  "room.room1.msg.evt123.replies.evt456",
			expected: true,
		},
		{
			name:     "root message",
			subject:  "room.room1.msg.evt123",
			expected: false,
		},
		{
			name:     "meta event",
			subject:  "room.room1.meta",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsThreadSubject(tt.subject)
			if got != tt.expected {
				t.Errorf("IsThreadSubject(%q) = %v, want %v", tt.subject, got, tt.expected)
			}
		})
	}
}

func TestParseEventIDFromSubject(t *testing.T) {
	tests := []struct {
		name     string
		subject  string
		expected string
	}{
		{
			name:     "root message",
			subject:  "room.room1.msg.evt123",
			expected: "evt123",
		},
		{
			name:     "thread reply",
			subject:  "room.room1.msg.evt123.replies.evt456",
			expected: "evt456",
		},
		{
			name:     "meta event (no event ID)",
			subject:  "room.room1.meta",
			expected: "",
		},
		{
			name:     "server-level event",
			subject:  "joined",
			expected: "",
		},
		{
			name:     "nanoid-style event ID",
			subject:  "room.R7IFBV0AV1UBYTK.msg.E8ShdnxI4BouAIl",
			expected: "E8ShdnxI4BouAIl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseEventIDFromSubject(tt.subject)
			if got != tt.expected {
				t.Errorf("ParseEventIDFromSubject(%q) = %q, want %q", tt.subject, got, tt.expected)
			}
		})
	}
}

func TestSpaceRoomRootEventsFilters(t *testing.T) {
	got := SpaceRoomRootEventsFilters("space1", "room1")
	expected := []string{
		"room.room1.msg.*",
		"room.room1.meta",
	}
	if len(got) != len(expected) {
		t.Fatalf("SpaceRoomRootEventsFilters() returned %d elements, want %d", len(got), len(expected))
	}
	for i, v := range got {
		if v != expected[i] {
			t.Errorf("SpaceRoomRootEventsFilters()[%d] = %q, want %q", i, v, expected[i])
		}
	}
}

func TestSpaceAllRoomEventsFilters(t *testing.T) {
	got := SpaceAllRoomEventsFilters("space1")
	expected := []string{
		"room.*.msg.>",
		"room.*.meta",
	}
	if len(got) != len(expected) {
		t.Fatalf("SpaceAllRoomEventsFilters() returned %d elements, want %d", len(got), len(expected))
	}
	for i, v := range got {
		if v != expected[i] {
			t.Errorf("SpaceAllRoomEventsFilters()[%d] = %q, want %q", i, v, expected[i])
		}
	}
}

func TestChatEventsSubjects(t *testing.T) {
	got := ChatEventsSubjects()
	expected := []string{"room.>", "joined", "left", "member_deleted"}
	if len(got) != len(expected) {
		t.Fatalf("ChatEventsSubjects() returned %d elements, want %d", len(got), len(expected))
	}
	for i, v := range got {
		if v != expected[i] {
			t.Errorf("ChatEventsSubjects()[%d] = %q, want %q", i, v, expected[i])
		}
	}
}

func TestServerLevelStructuralSubjects(t *testing.T) {
	if SpaceEvent("ignored", "joined") != "joined" {
		t.Errorf("SpaceEvent should produce bare event type, got %q", SpaceEvent("ignored", "joined"))
	}
	if ServerJoinedSubject != "joined" {
		t.Errorf("ServerJoinedSubject = %q, want \"joined\"", ServerJoinedSubject)
	}
	if ServerLeftSubject != "left" {
		t.Errorf("ServerLeftSubject = %q, want \"left\"", ServerLeftSubject)
	}
	if ServerMemberDeletedSubject != "member_deleted" {
		t.Errorf("ServerMemberDeletedSubject = %q, want \"member_deleted\"", ServerMemberDeletedSubject)
	}
}

func TestLiveSubjectsCompatShims(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"LiveUserEvent", LiveUserEvent("U1", "joined"), "live.user.U1.joined"},
		{"LiveInstanceUserEvent shim", LiveInstanceUserEvent("U1", "joined"), "live.user.U1.joined"},
		{"LiveServerEvent", LiveServerEvent("space_updated"), "live.server.space_updated"},
		{"LiveSpaceEvent shim", LiveSpaceEvent("ignored", "space_updated"), "live.server.space_updated"},
		{"LiveServerRoomEvent", LiveServerRoomEvent("R1", "reaction_added"), "live.server.room.R1.reaction_added"},
		{"LiveSpaceRoomEvent shim", LiveSpaceRoomEvent("ignored", "R1", "reaction_added"), "live.server.room.R1.reaction_added"},
		{"LiveServerConfigUpdated", LiveServerConfigUpdated(), "live.server.config.updated"},
		{"LiveInstanceConfigUpdated shim", LiveInstanceConfigUpdated(), "live.server.config.updated"},
		{"LiveInstanceSpaceEvent shim", LiveInstanceSpaceEvent("ignored", "new_message"), "live.server.new_message"},
		{"LiveServerLevelEvents", LiveServerLevelEvents(), "live.server.*"},
		{"LiveServerRoomAllEvents", LiveServerRoomAllEvents(), "live.server.room.>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}
