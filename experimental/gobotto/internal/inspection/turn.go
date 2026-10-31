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
}

// Input preserves a triggering message, separately from older thread context.
type Input struct {
	Text      string
	Addressed bool
}

// Turn is a version-independent inspection payload embedded in session snapshots.
// State is running, delivered, failed, uncertain, cancelled, or interrupted.
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
