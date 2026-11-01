package subagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRetainedReportDisclosesVerificationLimitsOnEverySurface(t *testing.T) {
	for _, status := range []string{"running", "partial", "complete"} {
		t.Run(status, func(t *testing.T) {
			// Old snapshots have no verification metadata. A model's confident
			// opening must not hide either host limits or a later caveat.
			report := "Confirmed! " + strings.Repeat("界", 2100) + " Runtime reproduction still needed."
			data, err := json.Marshal(map[string]any{"ID": "old", "Status": status, "Report": report})
			if err != nil {
				t.Fatal(err)
			}
			var run Run
			if err := json.Unmarshal(data, &run); err != nil {
				t.Fatal(err)
			}
			runs := map[string]*Run{run.ID: &run}
			b := NewBackground(context.Background(), nil, runs, nil, nil)
			defer b.Close()
			check := func(value any, truncated bool) {
				t.Helper()
				encoded, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				var got struct {
					Report       string         `json:"report"`
					Truncated    bool           `json:"report_truncated"`
					Verification map[string]any `json:"verification"`
				}
				if err := json.Unmarshal(encoded, &got); err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{"runtime_checked", "tests_run", "git_history_checked"} {
					if got.Verification[key] != false {
						t.Fatalf("%s not explicitly false: %s", key, encoded)
					}
				}
				if got.Verification["completion_meaning"] == nil || got.Truncated != truncated {
					t.Fatalf("missing completion limit or wrong truncation: %s", encoded)
				}
				if !truncated && got.Report != report {
					t.Fatal("full report lost its caveat")
				}
			}
			check(b.Context()[0], true)
			tool := (Binding{Runs: runs}).statusTool()
			for _, raw := range []string{`{}`, `{"run_id":"old"}`} {
				out, err := tool.Execute(context.Background(), json.RawMessage(raw))
				if err != nil {
					t.Fatal(err)
				}
				var decoded map[string]any
				if err := json.Unmarshal([]byte(out), &decoded); err != nil {
					t.Fatal(err)
				}
				if raw == `{}` {
					check(decoded["runs"].([]any)[0], true)
				} else {
					check(decoded, false)
				}
			}
			if run.Report != report || run.Status != status {
				t.Fatal("disclosure changed the saved report")
			}
		})
	}
}
