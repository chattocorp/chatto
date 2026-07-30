package events

import (
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/log"
)

type startupCompletionProjection struct {
	completions int
}

func (*startupCompletionProjection) Subjects() []string {
	return []string{"evt.test.created"}
}

func (*startupCompletionProjection) Apply(struct{}, uint64) error {
	return nil
}

func (p *startupCompletionProjection) CompleteStartupReplay() {
	p.completions++
}

func TestProjectorCompletesStartupReplayOnceAcrossReentry(t *testing.T) {
	projection := &startupCompletionProjection{}
	projector := NewDecodedProjector(
		nil,
		nil,
		projection,
		func([]byte) (DecodedEvent[struct{}], error) {
			return DecodedEvent[struct{}]{Event: struct{}{}, ID: "test"}, nil
		},
		log.New(io.Discard),
	)
	projector.started = true

	projector.maybeCompleteStartup(time.Now())
	projector.maybeCompleteStartup(time.Now())

	if projection.completions != 1 {
		t.Fatalf("startup replay completions = %d, want 1", projection.completions)
	}
}
