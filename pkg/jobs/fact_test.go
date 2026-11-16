package jobs

import (
	"encoding/json"
	"testing"
	"time"
)

// The JSON form of a fact is a persisted contract. This test pins it: a change
// here changes what stored facts mean.
func TestFactJSONFormIsPinned(t *testing.T) {
	created := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	deadline := created.Add(time.Hour)
	next := created.Add(time.Minute)
	spec := &factSpec{Payload: []byte("ref"), MaxAttempts: 3, RetryDelayMillis: 30000, ExpiresAt: &deadline}
	for name, tt := range map[string]struct {
		fact fact
		want string
	}{
		"queued": {
			fact{ID: "j", Type: "work", Kind: factQueued, OccurredAt: created, CreatedAt: created, Spec: spec},
			`{"id":"j","type":"work","fact":"queued","occurred_at":"2026-11-16T12:00:00Z","created_at":"2026-11-16T12:00:00Z","spec":{"payload":"cmVm","max_attempts":3,"retry_delay_ms":30000,"expires_at":"2026-11-16T13:00:00Z"}}`,
		},
		"attempt_failed": {
			fact{ID: "j", Type: "work", Kind: factAttemptFailed, OccurredAt: next, CreatedAt: created, Attempts: 1, Failure: "http_error", NextAttemptAt: &deadline},
			`{"id":"j","type":"work","fact":"attempt_failed","occurred_at":"2026-11-16T12:01:00Z","created_at":"2026-11-16T12:00:00Z","attempts":1,"failure":"http_error","next_attempt_at":"2026-11-16T13:00:00Z"}`,
		},
		"succeeded": {
			fact{ID: "j", Type: "work", Kind: factSucceeded, OccurredAt: next, CreatedAt: created, Attempts: 2, MaxAttempts: 3},
			`{"id":"j","type":"work","fact":"succeeded","occurred_at":"2026-11-16T12:01:00Z","created_at":"2026-11-16T12:00:00Z","attempts":2,"max_attempts":3}`,
		},
		"deleted": {
			fact{ID: "j", Type: "work", Kind: factDeleted, OccurredAt: next, CreatedAt: created},
			`{"id":"j","type":"work","fact":"deleted","occurred_at":"2026-11-16T12:01:00Z","created_at":"2026-11-16T12:00:00Z"}`,
		},
	} {
		data := must(json.Marshal(tt.fact))
		if string(data) != tt.want {
			t.Errorf("%s:\n got %s\nwant %s", name, data, tt.want)
		}
	}
	source := must(json.Marshal(effectSource{Subject: "evt.a", Sequence: 7, DataSHA256: []byte{1, 2}}))
	if want := `{"subject":"evt.a","sequence":7,"data_sha256":"AQI="}`; string(source) != want {
		t.Errorf("effect source:\n got %s\nwant %s", source, want)
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
