package events_test

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	. "hmans.de/chatto/pkg/events"
)

type multiSubjectTargetProjection struct{}

func (*multiSubjectTargetProjection) Subjects() []string {
	return []string{"evt.target.a", "evt.target.*.b", "evt.target.missing"}
}

func (*multiSubjectTargetProjection) Apply(codecTestEvent, uint64) error { return nil }

func TestProjectorCurrentTargetSeqUsesHighestMatchingFilter(t *testing.T) {
	js, err := jetstream.New(startTestNATS(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := testContext(t)
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: "TARGET", Subjects: []string{"evt.target.>"}})
	if err != nil {
		t.Fatal(err)
	}
	projector := NewDecodedProjector(js, stream, &multiSubjectTargetProjection{},
		func([]byte) (DecodedEvent[codecTestEvent], error) {
			return DecodedEvent[codecTestEvent]{}, nil
		}, testLogger())

	for _, subject := range []string{"evt.target.x.b", "evt.target.a", "evt.target.y.b", "evt.target.unmatched"} {
		if _, err := js.Publish(ctx, subject, []byte(subject)); err != nil {
			t.Fatalf("publish %s: %v", subject, err)
		}
	}

	// The last filter has no message, and the unmatched subject has the
	// highest stream sequence. Neither can affect the target.
	got, err := projector.CurrentTargetSeq(ctx)
	if err != nil {
		t.Fatalf("CurrentTargetSeq: %v", err)
	}
	if got != 3 {
		t.Fatalf("CurrentTargetSeq = %d, want 3", got)
	}
}
