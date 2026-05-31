package subjects

import "testing"

func TestRoomMessage(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		roomID   string
		eventID  string
		expected string
	}{
		{
			name:     "channel root message",
			kind:     "channel",
			roomID:   "R7IFBV0AV1UBYTK",
			eventID:  "E8ShdnxI4BouAIl",
			expected: "server.room.channel.R7IFBV0AV1UBYTK.msg.E8ShdnxI4BouAIl",
		},
		{
			name:     "dm root message",
			kind:     "dm",
			roomID:   "Rdm123abc",
			eventID:  "Eevt456",
			expected: "server.room.dm.Rdm123abc.msg.Eevt456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RoomMessage(tt.kind, tt.roomID, tt.eventID)
			if got != tt.expected {
				t.Errorf("RoomMessage() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRoomThread(t *testing.T) {
	got := RoomThread("channel", "R7IFBV0AV1UBYTK", "Eroot", "Eevt")
	expected := "server.room.channel.R7IFBV0AV1UBYTK.msg.Eroot.replies.Eevt"
	if got != expected {
		t.Errorf("RoomThread() = %q, want %q", got, expected)
	}
}

func TestRoomThreadFilter(t *testing.T) {
	got := RoomThreadFilter("channel", "R1", "Eroot")
	expected := "server.room.channel.R1.msg.Eroot.replies.>"
	if got != expected {
		t.Errorf("RoomThreadFilter() = %q, want %q", got, expected)
	}
}

func TestRoomThreadLookup(t *testing.T) {
	got := RoomThreadLookup("channel", "R1", "Eevt")
	expected := "server.room.channel.R1.msg.*.replies.Eevt"
	if got != expected {
		t.Errorf("RoomThreadLookup() = %q, want %q", got, expected)
	}
}

func TestRoomAllThreads(t *testing.T) {
	got := RoomAllThreads("channel", "R1")
	expected := "server.room.channel.R1.msg.*.replies.>"
	if got != expected {
		t.Errorf("RoomAllThreads() = %q, want %q", got, expected)
	}
}

func TestRoomMeta(t *testing.T) {
	got := RoomMeta("dm", "Rdm")
	expected := "server.room.dm.Rdm.meta"
	if got != expected {
		t.Errorf("RoomMeta() = %q, want %q", got, expected)
	}
}

func TestRoomAllMessages(t *testing.T) {
	got := RoomAllMessages("channel", "R1")
	expected := "server.room.channel.R1.msg.>"
	if got != expected {
		t.Errorf("RoomAllMessages() = %q, want %q", got, expected)
	}
}

func TestRoomRootMessages(t *testing.T) {
	got := RoomRootMessages("channel", "R1")
	expected := "server.room.channel.R1.msg.*"
	if got != expected {
		t.Errorf("RoomRootMessages() = %q, want %q", got, expected)
	}
}

func TestRoomAllEvents(t *testing.T) {
	got := RoomAllEvents("channel", "R1")
	expected := "server.room.channel.R1.>"
	if got != expected {
		t.Errorf("RoomAllEvents() = %q, want %q", got, expected)
	}
}

func TestAllRoomEvents(t *testing.T) {
	got := AllRoomEvents("dm")
	expected := "server.room.dm.>"
	if got != expected {
		t.Errorf("AllRoomEvents() = %q, want %q", got, expected)
	}
}

func TestRoomRootEventsFilters(t *testing.T) {
	got := RoomRootEventsFilters("channel", "R1")
	expected := []string{
		"server.room.channel.R1.msg.*",
		"server.room.channel.R1.meta",
	}
	if len(got) != len(expected) {
		t.Fatalf("RoomRootEventsFilters() returned %d elements, want %d", len(got), len(expected))
	}
	for i, v := range got {
		if v != expected[i] {
			t.Errorf("RoomRootEventsFilters()[%d] = %q, want %q", i, v, expected[i])
		}
	}
}

func TestAllRoomEventsFilters(t *testing.T) {
	got := AllRoomEventsFilters("channel")
	expected := []string{
		"server.room.channel.*.msg.>",
		"server.room.channel.*.meta",
	}
	if len(got) != len(expected) {
		t.Fatalf("AllRoomEventsFilters() returned %d elements, want %d", len(got), len(expected))
	}
	for i, v := range got {
		if v != expected[i] {
			t.Errorf("AllRoomEventsFilters()[%d] = %q, want %q", i, v, expected[i])
		}
	}
}

func TestMember(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		expected  string
	}{
		{
			name:      "raw verb",
			eventType: "joined",
			expected:  "server.member.joined",
		},
		{
			name:      "with member_ prefix stripped",
			eventType: "member_left",
			expected:  "server.member.left",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Member(tt.eventType)
			if got != tt.expected {
				t.Errorf("Member(%q) = %q, want %q", tt.eventType, got, tt.expected)
			}
		})
	}
}

func TestAllEvents(t *testing.T) {
	if got := AllEvents(); got != "server.>" {
		t.Errorf("AllEvents() = %q, want %q", got, "server.>")
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
			subject:  "server.room.channel.R1.msg.evt123",
			expected: "R1",
		},
		{
			name:     "thread reply",
			subject:  "server.room.channel.R1.msg.evt123.replies.evt456",
			expected: "R1",
		},
		{
			name:     "meta event",
			subject:  "server.room.dm.R1.meta",
			expected: "R1",
		},
		{
			name:     "live sync root message",
			subject:  "live.sync.room.channel.R1.msg.evt123",
			expected: "R1",
		},
		{
			name:     "live sync transient room event",
			subject:  "live.sync.room.dm.R1.user_typing",
			expected: "R1",
		},
		{
			name:     "legacy live.server room event ignored",
			subject:  "live.server.room.dm.R1.reaction_added",
			expected: "",
		},
		{
			name:     "member event (not a room)",
			subject:  "server.member.joined",
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
			subject:         "server.room.channel.R1.msg.Eroot.replies.Eevt",
			expectedEventID: "Eroot",
			expectedOK:      true,
		},
		{
			name:            "live sync thread reply",
			subject:         "live.sync.room.channel.R1.msg.Eroot.replies.Eevt",
			expectedEventID: "Eroot",
			expectedOK:      true,
		},
		{
			name:            "root message",
			subject:         "server.room.channel.R1.msg.evt123",
			expectedEventID: "",
			expectedOK:      false,
		},
		{
			name:            "meta event",
			subject:         "server.room.channel.R1.meta",
			expectedEventID: "",
			expectedOK:      false,
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
		{name: "root message", subject: "server.room.channel.R1.msg.evt123", expected: true},
		{name: "live sync root message", subject: "live.sync.room.channel.R1.msg.evt123", expected: true},
		{name: "thread reply", subject: "server.room.channel.R1.msg.evt123.replies.evt456", expected: false},
		{name: "legacy live.server root message ignored", subject: "live.server.room.channel.R1.msg.evt123", expected: false},
		{name: "meta event", subject: "server.room.channel.R1.meta", expected: false},
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
		{name: "meta event", subject: "server.room.channel.R1.meta", expected: true},
		{name: "live sync meta event", subject: "live.sync.room.channel.R1.meta", expected: true},
		{name: "legacy live.server meta event ignored", subject: "live.server.room.channel.R1.meta", expected: false},
		{name: "root message", subject: "server.room.channel.R1.msg.evt123", expected: false},
		{name: "thread reply", subject: "server.room.channel.R1.msg.evt123.replies.evt456", expected: false},
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
		{name: "thread reply", subject: "server.room.channel.R1.msg.evt123.replies.evt456", expected: true},
		{name: "live sync thread reply", subject: "live.sync.room.channel.R1.msg.evt123.replies.evt456", expected: true},
		{name: "legacy live.server thread reply ignored", subject: "live.server.room.channel.R1.msg.evt123.replies.evt456", expected: false},
		{name: "root message", subject: "server.room.channel.R1.msg.evt123", expected: false},
		{name: "meta event", subject: "server.room.channel.R1.meta", expected: false},
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
		{name: "root message", subject: "server.room.channel.R1.msg.evt123", expected: "evt123"},
		{name: "live sync root message", subject: "live.sync.room.channel.R1.msg.evt123", expected: "evt123"},
		{name: "thread reply", subject: "server.room.channel.R1.msg.evt123.replies.evt456", expected: "evt456"},
		{name: "legacy live.server root message ignored", subject: "live.server.room.channel.R1.msg.evt123", expected: ""},
		{name: "meta event (no event ID)", subject: "server.room.channel.R1.meta", expected: ""},
		{name: "member event", subject: "server.member.joined", expected: ""},
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

func TestLiveSubjects(t *testing.T) {
	cases := map[string]string{
		"LiveSyncAllEvents":   "live.sync.>",
		"LiveSyncUserEvent":   "live.sync.user.U1.settings_updated",
		"LiveSyncRoomEvent":   "live.sync.room.channel.R1.typing",
		"LiveSyncConfigEvent": "live.sync.config.updated",
		"LiveSyncMember":      "live.sync.member.left",
	}
	got := map[string]string{
		"LiveSyncAllEvents":   LiveSyncAllEvents(),
		"LiveSyncUserEvent":   LiveSyncUserEvent("U1", "settings_updated"),
		"LiveSyncRoomEvent":   LiveSyncRoomEvent("channel", "R1", "typing"),
		"LiveSyncConfigEvent": LiveSyncConfigEvent("updated"),
		"LiveSyncMember":      LiveSyncMember("member_left"),
	}
	for k, want := range cases {
		if got[k] != want {
			t.Errorf("%s = %q, want %q", k, got[k], want)
		}
	}
}
