package events

import "testing"

func TestSubjectMatchesFilter(t *testing.T) {
	cases := []struct {
		filter  string
		subject string
		want    bool
	}{
		{"evt.room.>", "evt.room.R1.user_joined", true},
		{"evt.room.*.user_joined", "evt.room.R1.user_joined", true},
		{"evt.room.*.user_joined", "evt.room.R1.message_posted", false},
		{"evt.room.*.user_joined", "evt.room.R1.extra.user_joined", false},
		{"evt.room.R1.user_joined", "evt.room.R1.user_joined", true},
		{"evt.room.R1.user_joined", "evt.room.R2.user_joined", false},
		{"evt.room.>", "evt.room", false},
		{">", "evt.room.R1.user_joined", true},
		{"", "evt.room.R1.user_joined", false},
		{"evt.room.>", "", false},
		// Token-count mismatches in both directions.
		{"evt.room.*", "evt.room.R1.extra", false},
		{"evt.room.R1.extra", "evt.room.R1", false},
		// '*' spans exactly one token.
		{"*", "single", true},
		{"*", "", false},
		// '>' is only meaningful as the final filter token.
		{"evt.>.joined", "evt.a.joined", false},
		{">.", "anything.here", false},
		{">", "one", true},
		// '>' must leave a nonempty unconsumed subject tail.
		{"a.b.>", "a.b", false},
		{"a.b.>", "a.b.", false},
		// Empty tokens never match: dots delimit strictly.
		{".a", ".a", false},
		{"a..b", "a..b", false},
		{"a.b", "a.b.", false},
		// A literal '*' token in a subject matches only a literal '*'.
		{"a.*", "a.*", true},
		{"a.x", "a.*", false},
	}
	for _, test := range cases {
		t.Run(test.filter+" matches "+test.subject, func(t *testing.T) {
			if got := subjectMatchesFilter(test.filter, test.subject); got != test.want {
				t.Fatalf("subjectMatchesFilter(%q, %q) = %v, want %v", test.filter, test.subject, got, test.want)
			}
		})
	}
}

func TestCompiledSubjectFilterMatchesWithoutAllocations(t *testing.T) {
	matcher := compileSubjectFilter("evt.room.*.user_joined")
	allocations := testing.AllocsPerRun(1000, func() {
		if !matcher.matches("evt.room.R1.user_joined") {
			t.Fatal("expected compiled filter to match")
		}
		if matcher.matches("evt.room.R1.message_posted") {
			t.Fatal("expected compiled filter not to match")
		}
	})
	if allocations != 0 {
		t.Fatalf("compiled matcher allocations = %v, want 0", allocations)
	}
}

func TestStreamSequenceFromReply(t *testing.T) {
	cases := []struct {
		name    string
		reply   string
		want    uint64
		wantErr bool
	}{
		{name: "v2 with domain and token", reply: "$JS.ACK.domain.hash-123.stream.cons.100.200.150.123456789.100.token", want: 200},
		{name: "v2 without trailing token", reply: "$JS.ACK.domain.hash-123.stream.cons.100.201.150.123456789.100", want: 201},
		{name: "v2 underscore domain", reply: "$JS.ACK._.hash-123.stream.cons.100.202.150.123456789.100.token", want: 202},
		{name: "v1", reply: "$JS.ACK.stream.cons.100.203.150.123456789.100", want: 203},
		{name: "invalid prefix", reply: "$ABC.123.stream.cons.100.200.150.123456789.100", wantErr: true},
		{name: "invalid token count", reply: "$JS.ACK.stream.cons.100.200.150.123456789.100.extra", wantErr: true},
		{name: "non numeric sequence", reply: "$JS.ACK.stream.cons.100.not-a-seq.150.123456789.100", wantErr: true},
		{name: "empty", wantErr: true},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := streamSequenceFromReply(test.reply)
			if test.wantErr {
				if err == nil {
					t.Fatalf("streamSequenceFromReply(%q) error = nil, want error", test.reply)
				}
				return
			}
			if err != nil {
				t.Fatalf("streamSequenceFromReply(%q) error = %v", test.reply, err)
			}
			if got != test.want {
				t.Fatalf("streamSequenceFromReply(%q) = %d, want %d", test.reply, got, test.want)
			}
		})
	}
}

func TestStreamSequenceFromReplyDoesNotAllocate(t *testing.T) {
	const reply = "$JS.ACK.domain.hash-123.stream.cons.100.200.150.123456789.100.token"
	allocations := testing.AllocsPerRun(1000, func() {
		got, err := streamSequenceFromReply(reply)
		if err != nil || got != 200 {
			t.Fatalf("streamSequenceFromReply() = %d, %v", got, err)
		}
	})
	if allocations != 0 {
		t.Fatalf("streamSequenceFromReply allocations = %v, want 0", allocations)
	}
}

type subjectCountingProjection struct {
	calls int
}

func (p *subjectCountingProjection) Subjects() []string {
	p.calls++
	return []string{"evt.room.>", "evt.user.*.user_key_shredded"}
}

func (*subjectCountingProjection) Apply(string, uint64) error {
	return nil
}

func TestProjectorCachesProjectionSubjects(t *testing.T) {
	projection := &subjectCountingProjection{}
	projector := NewDecodedProjector(
		nil,
		nil,
		projection,
		func([]byte) (DecodedEvent[string], error) {
			return DecodedEvent[string]{}, nil
		},
		nil,
	)

	for range 10 {
		_ = projector.Subjects()
		_ = projector.ReplaySubjects()
		if !projector.consumesSubject("evt.room.R1.message_posted") {
			t.Fatal("expected projector to consume room subject")
		}
		if projector.consumesSubject("evt.config.server.server_name_changed") {
			t.Fatal("expected projector not to consume config subject")
		}
	}
	if projection.calls != 1 {
		t.Fatalf("Subjects calls = %d, want 1", projection.calls)
	}
}
