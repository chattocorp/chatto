// Package inspection defines private, structured turn records. These records
// belong in session storage and the inspector, never in operational logs.
package inspection

import (
	"encoding/json"
	"time"
)

// Usage contains reported billing totals; cached input is part of input.
type Usage struct {
	Input, Cached, Output         int64
	Cost                          float64
	Known, CostKnown, CostMissing bool
	// CacheKnown is false for missing telemetry and legacy records with ambiguous zeros.
	CacheKnown bool
	// Reasoning is included in Output; false means absent or legacy telemetry.
	Reasoning      int64
	ReasoningKnown bool
}

// Step records one externally observable operation, without model reasoning.
type Step struct {
	Kind, Name, State, Detail string
	Started                   time.Time
	Duration                  time.Duration
	Usage                     Usage
	// Provider and RequestID support private inspection of cache misses.
	Provider, RequestID string
	// Request and Response are bounded live-only snapshots, never persisted or
	// added to model context. The session already stores its model transcript.
	// Request contains the provider payload with image data/reasoning omitted.
	Request  string `json:"-"`
	Response string `json:"-"`
}

// Input preserves a triggering message, separately from older thread context.
type Input struct {
	Text      string
	Addressed bool
}

// Turn is a version-independent inspection payload embedded in session snapshots.
// State is running, delivered, silent, waiting, failed, uncertain, cancelled, or interrupted.
type Turn struct {
	Number                                 int
	Started, Finished                      time.Time
	Wait                                   time.Duration
	State, Model, Response, Emoji, Failure string
	Inputs                                 []Input
	Steps                                  []Step
}

// Clone isolates UI readers from the worker that continues to update a turn.
func (t Turn) Clone() Turn {
	t.Inputs = append([]Input(nil), t.Inputs...)
	t.Steps = append([]Step(nil), t.Steps...)
	return t
}

// Retain bounds stored inspection history by count and encoded size. The latest
// turn always remains available; each message is bounded at capture time.
func Retain(turns []Turn) []Turn {
	// Bound live payloads separately from durable event history. Drop the oldest
	// captures first without deleting their timing, usage, or tool receipts.
	turns = append([]Turn(nil), turns...)
	bytes := 0
	for _, turn := range turns {
		for _, step := range turn.Steps {
			bytes += len(step.Request) + len(step.Response)
		}
	}
	for i := 0; i < len(turns)-1 && bytes > 4<<20; i++ {
		turns[i] = turns[i].Clone()
		for j := range turns[i].Steps {
			s := &turns[i].Steps[j]
			bytes -= len(s.Request) + len(s.Response)
			s.Request = ""
			s.Response = ""
		}
	}
	if len(turns) > 50 {
		turns = turns[len(turns)-50:]
	}
	for len(turns) > 1 {
		data, _ := json.Marshal(turns)
		if len(data) <= 2<<20 {
			break
		}
		turns = turns[1:]
	}
	return turns
}

// Log contains only fixed operational text, not conversation content.
type Log struct {
	At   time.Time
	Text string
}
