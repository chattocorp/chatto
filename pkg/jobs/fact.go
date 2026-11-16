package jobs

import (
	"encoding/json"
	"time"
)

// fact is one fact in the history of a job. The job stream holds facts as JSON
// on {prefix}.{type}.{id}.{kind}. The state of a job is the fold of its facts
// in stream order.
//
// The JSON form is a persisted contract of every application that uses this
// module. Add fields only. Never change the meaning of a field or of a kind,
// and never reuse a name. A reader ignores fields that it does not know.
type fact struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	// Kind names the fact: one of the fact* constants. It is also the last
	// token of the subject.
	Kind string `json:"fact"`
	// OccurredAt is when this fact occurred.
	OccurredAt time.Time `json:"occurred_at"`
	// CreatedAt is when the job was first accepted. Every fact repeats it, so
	// a fact stays readable after earlier facts are gone.
	CreatedAt time.Time `json:"created_at"`
	// Spec is what to run and the policy for it. The queued, retried, and
	// failed facts carry it: workers consume the first two, and a manual retry
	// reads it from the last.
	Spec *factSpec `json:"spec,omitempty"`
	// Attempts is the number of the attempt in a started or attempt_failed
	// fact, and the number of attempts that the job used in a succeeded or
	// failed fact.
	Attempts uint32 `json:"attempts,omitempty"`
	// MaxAttempts is the attempt budget in a succeeded fact, which has no Spec.
	MaxAttempts uint32 `json:"max_attempts,omitempty"`
	// Failure is the safe failure category in an attempt_failed or failed
	// fact, never handler error text.
	Failure string `json:"failure,omitempty"`
	// NextAttemptAt is when a job with a failed attempt runs again.
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
}

// factSpec is the stored form of a Spec. The payload is an application
// reference, never content or credentials.
type factSpec struct {
	Payload []byte `json:"payload,omitempty"`
	// MaxAttempts is the automatic attempt budget. UnlimitedAttempts means no
	// limit.
	MaxAttempts      uint32 `json:"max_attempts"`
	RetryDelayMillis int64  `json:"retry_delay_ms"`
	// ExpiresAt is an optional deadline. A manual retry never extends it.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// decodeFact reads a stored fact. Every reader uses it, so that all of them
// ignore fields that a later version added.
func decodeFact(data []byte) (*fact, error) {
	f := new(fact)
	if err := json.Unmarshal(data, f); err != nil {
		return nil, err
	}
	return f, nil
}

// encode returns the stored form of the fact. Times are written in UTC, so
// that a stored fact does not depend on the time zone of the server.
func (f *fact) encode() ([]byte, error) {
	stored := *f
	stored.OccurredAt, stored.CreatedAt = f.OccurredAt.UTC(), f.CreatedAt.UTC()
	if f.NextAttemptAt != nil {
		next := f.NextAttemptAt.UTC()
		stored.NextAttemptAt = &next
	}
	if f.Spec != nil && f.Spec.ExpiresAt != nil {
		spec := *f.Spec
		expires := f.Spec.ExpiresAt.UTC()
		spec.ExpiresAt = &expires
		stored.Spec = &spec
	}
	return json.Marshal(stored)
}

// specOf returns the specification of a queued, retried, or failed fact, or
// nil.
func specOf(f *fact) *factSpec {
	switch f.Kind {
	case factQueued, factRetried, factFailed:
		return f.Spec
	}
	return nil
}

// effectSource is the payload of an effect job: a reference to one retained
// source message. Each attempt loads the message again; the job never copies
// its data. The digest binds the reference to that exact message, so a
// recreated stream that reuses the sequence cannot satisfy it.
type effectSource struct {
	Subject    string `json:"subject"`
	Sequence   uint64 `json:"sequence"`
	DataSHA256 []byte `json:"data_sha256"`
}
