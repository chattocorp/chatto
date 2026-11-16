package jobs

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// storedFacts are facts in the JSON form that a released version wrote. The
// form is a persisted contract: job streams of running servers hold facts like
// these. When the format gains a field, add samples. Never change or remove a
// sample: every later version must still read it.
func storedFacts() []struct {
	name string
	json string
	fact fact
} {
	created := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	later := created.Add(time.Minute)
	deadline := created.Add(time.Hour)
	spec := &factSpec{Payload: []byte("ref"), MaxAttempts: 3, RetryDelayMillis: 30000, ExpiresAt: &deadline}
	unlimited := &factSpec{MaxAttempts: UnlimitedAttempts, RetryDelayMillis: 1000}
	return []struct {
		name string
		json string
		fact fact
	}{
		{
			"queued",
			`{"id":"j","type":"work","fact":"queued","occurred_at":"2026-11-16T12:00:00Z","created_at":"2026-11-16T12:00:00Z","spec":{"payload":"cmVm","max_attempts":3,"retry_delay_ms":30000,"expires_at":"2026-11-16T13:00:00Z"}}`,
			fact{ID: "j", Type: "work", Kind: factQueued, OccurredAt: created, CreatedAt: created, Spec: spec},
		},
		{
			"queued without payload, deadline, or attempt limit",
			`{"id":"j","type":"work","fact":"queued","occurred_at":"2026-11-16T12:00:00Z","created_at":"2026-11-16T12:00:00Z","spec":{"max_attempts":4294967295,"retry_delay_ms":1000}}`,
			fact{ID: "j", Type: "work", Kind: factQueued, OccurredAt: created, CreatedAt: created, Spec: unlimited},
		},
		{
			"started",
			`{"id":"j","type":"work","fact":"started","occurred_at":"2026-11-16T12:01:00Z","created_at":"2026-11-16T12:00:00Z","attempts":1}`,
			fact{ID: "j", Type: "work", Kind: factStarted, OccurredAt: later, CreatedAt: created, Attempts: 1},
		},
		{
			"attempt_failed",
			`{"id":"j","type":"work","fact":"attempt_failed","occurred_at":"2026-11-16T12:01:00Z","created_at":"2026-11-16T12:00:00Z","attempts":1,"failure":"http_error","next_attempt_at":"2026-11-16T13:00:00Z"}`,
			fact{ID: "j", Type: "work", Kind: factAttemptFailed, OccurredAt: later, CreatedAt: created, Attempts: 1, Failure: "http_error", NextAttemptAt: &deadline},
		},
		{
			"succeeded",
			`{"id":"j","type":"work","fact":"succeeded","occurred_at":"2026-11-16T12:01:00Z","created_at":"2026-11-16T12:00:00Z","attempts":2,"max_attempts":3}`,
			fact{ID: "j", Type: "work", Kind: factSucceeded, OccurredAt: later, CreatedAt: created, Attempts: 2, MaxAttempts: 3},
		},
		{
			"failed",
			`{"id":"j","type":"work","fact":"failed","occurred_at":"2026-11-16T12:01:00Z","created_at":"2026-11-16T12:00:00Z","spec":{"payload":"cmVm","max_attempts":3,"retry_delay_ms":30000,"expires_at":"2026-11-16T13:00:00Z"},"attempts":3,"failure":"attempt_failed"}`,
			fact{ID: "j", Type: "work", Kind: factFailed, OccurredAt: later, CreatedAt: created, Spec: spec, Attempts: 3, Failure: "attempt_failed"},
		},
		{
			"failed before the first attempt",
			`{"id":"j","type":"work","fact":"failed","occurred_at":"2026-11-16T12:01:00Z","created_at":"2026-11-16T12:00:00Z","spec":{"payload":"cmVm","max_attempts":3,"retry_delay_ms":30000,"expires_at":"2026-11-16T13:00:00Z"},"failure":"expired"}`,
			fact{ID: "j", Type: "work", Kind: factFailed, OccurredAt: later, CreatedAt: created, Spec: spec, Failure: "expired"},
		},
		{
			"retried",
			`{"id":"j","type":"work","fact":"retried","occurred_at":"2026-11-16T12:01:00Z","created_at":"2026-11-16T12:00:00Z","spec":{"payload":"cmVm","max_attempts":3,"retry_delay_ms":30000,"expires_at":"2026-11-16T13:00:00Z"}}`,
			fact{ID: "j", Type: "work", Kind: factRetried, OccurredAt: later, CreatedAt: created, Spec: spec},
		},
		{
			"deleted",
			`{"id":"j","type":"work","fact":"deleted","occurred_at":"2026-11-16T12:01:00Z","created_at":"2026-11-16T12:00:00Z"}`,
			fact{ID: "j", Type: "work", Kind: factDeleted, OccurredAt: later, CreatedAt: created},
		},
	}
}

// This version reads every stored fact, and it writes the same form. A
// failure here means that a change broke the persisted contract.
func TestFactJSONFormIsPinned(t *testing.T) {
	for _, tt := range storedFacts() {
		var read fact
		noError(t, json.Unmarshal([]byte(tt.json), &read))
		if !reflect.DeepEqual(read, tt.fact) {
			t.Errorf("%s: read %+v, want %+v", tt.name, read, tt.fact)
		}
		if written := must(json.Marshal(tt.fact)); string(written) != tt.json {
			t.Errorf("%s:\n wrote %s\n want  %s", tt.name, written, tt.json)
		}
	}
	const storedSource = `{"subject":"evt.a","sequence":7,"data_sha256":"AQI="}`
	source := effectSource{Subject: "evt.a", Sequence: 7, DataSHA256: []byte{1, 2}}
	var read effectSource
	noError(t, json.Unmarshal([]byte(storedSource), &read))
	if !reflect.DeepEqual(read, source) {
		t.Errorf("effect source: read %+v", read)
	}
	if written := must(json.Marshal(source)); string(written) != storedSource {
		t.Errorf("effect source: wrote %s", written)
	}
}

// The view folds the stored facts of one job, in the order that a job that
// failed, was retried, and then succeeded writes them.
func TestViewFoldsStoredFacts(t *testing.T) {
	stored := map[string]string{}
	for _, tt := range storedFacts() {
		stored[tt.name] = tt.json
	}
	v := &view{jobs: map[string]*entry{}}
	for seq, name := range []string{"queued", "started", "attempt_failed", "failed", "retried", "succeeded"} {
		f := new(fact)
		noError(t, json.Unmarshal([]byte(stored[name]), f))
		noError(t, v.Apply(f, uint64(seq+1)))
	}
	e, ok := v.get("j")
	if !ok || e.job.Status != Succeeded || e.job.Attempts != 2 || e.job.MaxAttempts != 3 || e.job.Failure != "" || e.job.Revision != 6 || e.job.RetryDelay != 30*time.Second || e.job.ExpiresAt.IsZero() {
		t.Fatalf("job = %+v", e)
	}
}

// A later version can add fields and kinds. This version must read such facts
// without an error: it ignores the fields, and the view ignores the kind.
func TestFactReadsFieldsAndKindsOfLaterVersions(t *testing.T) {
	var f fact
	noError(t, json.Unmarshal([]byte(`{"id":"j","type":"work","fact":"paused","occurred_at":"2026-11-16T12:00:00Z","created_at":"2026-11-16T12:00:00Z","priority":5,"spec":{"max_attempts":1,"retry_delay_ms":1,"queue":"fast"}}`), &f))
	if f.ID != "j" || f.Kind != "paused" || specOf(&f) != nil {
		t.Fatalf("fact = %+v", f)
	}
	v := &view{jobs: map[string]*entry{}}
	noError(t, v.Apply(&fact{ID: "j", Type: "work", Kind: factQueued, Spec: &factSpec{MaxAttempts: 1}}, 1))
	noError(t, v.Apply(&f, 2))
	if e, _ := v.get("j"); e.job.Status != Queued || e.job.Revision != 2 {
		t.Fatalf("job after an unknown kind = %+v", e)
	}
}
