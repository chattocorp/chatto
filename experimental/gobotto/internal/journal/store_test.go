package journal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/internal/testmeans"
)

func TestScopeIsolationPersistenceAndInvalidation(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "server/bot", testmeans.Command)
	if err != nil {
		t.Fatal(err)
	}
	scopes := []Scope{{Room: "one", Thread: "a"}, {Room: "one", Thread: "b"}, {Room: "two", Thread: "a"}, {Room: "one"}}
	for i, scope := range scopes {
		if err := s.Append(scope, Entry{Key: "same-turn", Inputs: []string{strings.Repeat("x", i+1)}, State: "delivered"}); err != nil {
			t.Fatal(err)
		}
	}
	for i, scope := range scopes {
		d, err := s.Read(scope)
		if err != nil || len(d.Entries) != 1 || d.Entries[0].Inputs[0] != strings.Repeat("x", i+1) {
			t.Fatal("scope mixed", err)
		}
		if err := s.Summarize(scope, d.Revision, d.Generation, "", []string{"A fact"}); err != nil {
			t.Fatal(err)
		}
	}
	if other, err := Open(dir, "server/bot"); err == nil {
		other.Close()
		t.Fatal("second process accepted")
	}
	s.Close()
	if other, err := Open(dir, "different-server/bot"); err == nil {
		other.Close()
		t.Fatal("account scope changed")
	}
	s, err = Open(dir, "server/bot", testmeans.Command)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	d, _ := s.Read(scopes[0])
	if d.Summary != "- A fact\n" || len(d.Entries) != 0 {
		t.Fatal("notebook did not survive restart")
	}
	if err := s.Forget("one"); err != nil {
		t.Fatal(err)
	}
	for _, scope := range scopes {
		d, _ := s.Read(scope)
		if (d.Summary == "") != (scope.Room == "one") {
			t.Fatal("wrong room invalidated")
		}
	}
	if err := s.AppendAt(scopes[0], Entry{Key: "stale"}, 0, 0); err == nil {
		t.Fatal("stale work restored invalidated context")
	}
	// Even a scope that had no document when invalidated must reject old work.
	if err := s.AppendAt(Scope{Room: "one", Thread: "new"}, Entry{Key: "stale"}, 0, 0); err == nil {
		t.Fatal("missing-document invalidation lost")
	}
}

func TestIdempotentAppendAndFailedCheckpoint(t *testing.T) {
	s, err := Open(t.TempDir(), "account", testmeans.Command)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	scope := Scope{Room: "r", Thread: "t"}
	e := Entry{Key: "turn", Inputs: []string{"request"}, Receipts: []Receipt{{Name: "write", Result: `{"status":"complete"}`}}}
	for range 2 {
		if err := s.Append(scope, e); err != nil {
			t.Fatal(err)
		}
	}
	d, _ := s.Read(scope)
	if len(d.Entries) != 1 {
		t.Fatal("duplicate appended")
	}
	if err := s.Summarize(scope, d.Revision+1, d.Generation, "stale", []string{"note"}); err == nil {
		t.Fatal("stale summary accepted")
	}
	if err := s.Summarize(scope, d.Revision, d.Generation, strings.Repeat("s", MaxSummary+1), []string{"note"}); err == nil {
		t.Fatal("oversized summary accepted")
	}
	if err := s.Summarize(scope, d.Revision, d.Generation, "summary", []string{"note"}); err != nil {
		t.Fatal(err)
	}
	d, _ = s.Read(scope)
	if len(d.Entries) != 0 || !d.Uncertain {
		t.Fatal("notebook duplicated receipts or lost uncertainty")
	}
	if err := s.Append(scope, e); err != nil {
		t.Fatal("completed append was not idempotent", err)
	}
	path := s.path(scope)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(scope, Entry{Key: "next"}); err == nil {
		t.Fatal("unwritable journal accepted")
	}
}

func TestJournalRejectsSymlinkAndMalformedDocument(t *testing.T) {
	s, err := Open(t.TempDir(), "account")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	scope := Scope{Room: "r"}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, s.path(scope)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(scope); err == nil {
		t.Fatal("symlink followed")
	}
	if err := os.Remove(s.path(scope)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.path(scope), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(scope); err == nil {
		t.Fatal("malformed journal silently replaced")
	}
}

func TestJournalIsMeansReadable(t *testing.T) {
	binary, err := exec.LookPath("means")
	if err != nil {
		t.Skip("means not installed")
	}
	s, err := Open(t.TempDir(), "account")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	scope := Scope{Room: "room", Thread: "thread"}
	if err := s.Append(scope, Entry{Key: "turn", State: "delivered", Inputs: []string{"request"}}); err != nil {
		t.Fatal(err)
	}
	d, _ := s.Read(scope)
	if err := s.Summarize(scope, d.Revision, d.Generation, "", []string{"A brief fact."}); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(scope, Entry{Key: "next", State: "delivered"}); err != nil {
		t.Fatal(err)
	}
	d, _ = s.Read(scope)
	if err := s.Summarize(scope, d.Revision, d.Generation, "", []string{"A second fact."}); err != nil {
		t.Fatal(err)
	}
	d, _ = s.Read(scope)
	if d.Summary != "- A brief fact.\n- A second fact.\n" {
		t.Fatal("append replaced or reformatted the notebook", d.Summary)
	}
	out, err := exec.Command(binary, "--path", s.dir, "--json", "list").Output()
	var result struct {
		Documents   []struct{ ID string }
		Diagnostics []json.RawMessage
	}
	if err != nil || json.Unmarshal(out, &result) != nil || len(result.Documents) != 1 || len(result.Diagnostics) != 0 {
		t.Fatal("journal is not means-readable", err, string(out))
	}
	if len(result.Documents[0].ID) != 12 {
		t.Fatal("means shows the storage digest instead of a short document ID", result.Documents[0].ID)
	}
	if _, err := exec.Command(binary, "--path", s.dir, "--json", "show", "--", result.Documents[0].ID).Output(); err != nil {
		t.Fatal("means cannot open the short document ID", err)
	}
}

func TestNotebookAppendRecoveryDoesNotDuplicateNotes(t *testing.T) {
	for _, applied := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-append", true: "after-append"}[applied], func(t *testing.T) {
			dir := t.TempDir()
			command := func(ctx context.Context, in []byte, args ...string) ([]byte, error) {
				if args[3] == "append" {
					if applied {
						if _, err := testmeans.Command(ctx, in, args...); err != nil {
							return nil, err
						}
					}
					return nil, errors.New("lost command acknowledgement")
				}
				return testmeans.Command(ctx, in, args...)
			}
			s, err := Open(dir, "account", command)
			if err != nil {
				t.Fatal(err)
			}
			scope := Scope{Room: "room"}
			e := Entry{Key: "turn", State: "delivered", Inputs: []string{"raw request"}}
			if err := s.Append(scope, e); err != nil {
				t.Fatal(err)
			}
			if err := s.Summarize(scope, 0, 0, "", []string{"A fact."}); err == nil {
				t.Fatal("uncertain command succeeded")
			}
			if _, err := s.Read(scope); err == nil {
				t.Fatal("uncertain store accepted further work")
			}
			s.Close()
			s, err = Open(dir, "account", testmeans.Command)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			d, err := s.Read(scope)
			if err != nil {
				t.Fatal(err)
			}
			if (d.Summary != "") != applied {
				t.Fatal("wrong recovered body")
			}
			if err := s.Append(scope, e); err != nil {
				t.Fatal(err)
			}
			d, _ = s.Read(scope)
			if !applied {
				if err := s.Summarize(scope, d.Revision, d.Generation, "", []string{"A fact."}); err != nil {
					t.Fatal(err)
				}
			}
			d, _ = s.Read(scope)
			if d.Summary != "- A fact.\n" || len(d.Entries) != 0 {
				t.Fatal("note duplicated or pending evidence persisted")
			}
		})
	}
}

func TestNotebookCheckpointStaysBoundedAndFillerAddsNothing(t *testing.T) {
	s, err := Open(t.TempDir(), "account", testmeans.Command)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	scope := Scope{Room: "room"}
	for i := 0; i < 50; i++ {
		e := Entry{Key: strings.Repeat("x", i+1), State: "delivered", Receipts: []Receipt{{Name: "react_to_ping", Result: "raw receipt"}}}
		if err := s.Append(scope, e); err != nil {
			t.Fatal(err)
		}
		d, _ := s.Read(scope)
		if err := s.Summarize(scope, d.Revision, d.Generation, "", []string{""}); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(s.path(scope))
	if err != nil {
		t.Fatal(err)
	}
	d, _ := s.Read(scope)
	if len(b) > 700 || d.Summary != "" || strings.Contains(string(b), "raw receipt") || strings.Contains(string(b), "Journal entries") {
		t.Fatal("filler grew the notebook or copied receipts")
	}
}

func TestLegacyNotebookChangesFormatOnlyOnWrite(t *testing.T) {
	s, err := Open(t.TempDir(), "account", testmeans.Command)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	scope := Scope{Room: "room", Thread: "thread"}
	header, _ := json.Marshal(map[string]any{"id": "legacy", "data": map[string]any{"scope": scope, "generation": 0, "revision": 2, "through": 1}})
	entries, _ := json.Marshal([]Entry{{Key: "old", Digest: "old-digest", Note: "Keep this decision.", State: "uncertain", Receipts: []Receipt{{Name: "write", Result: "raw receipt"}}}})
	original := []byte("---\n" + string(header) + "\n---\n# Current context\n\nDuplicated summary" + separator + string(entries) + "\n```\n")
	if err := os.WriteFile(s.path(scope), original, 0600); err != nil {
		t.Fatal(err)
	}
	d, err := s.Read(scope)
	if err != nil || d.Summary != "- Keep this decision.\n" || !d.Uncertain {
		t.Fatal("old notebook not understood", err)
	}
	b, _ := os.ReadFile(s.path(scope))
	if string(b) != string(original) {
		t.Fatal("read rewrote an old notebook")
	}
	if err := s.AppendAt(scope, Entry{Key: "new", State: "delivered"}, d.Revision, d.Generation); err != nil {
		t.Fatal(err)
	}
	if err := s.Summarize(scope, d.Revision, d.Generation, "", []string{"New fact."}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(s.path(scope))
	d, err = s.Read(scope)
	if err != nil || d.Summary != "- Keep this decision.\n- New fact.\n" || !d.Uncertain || strings.Contains(string(b), "raw receipt") || strings.Contains(string(b), "Journal entries") {
		t.Fatal("old format survived write or uncertainty was lost", err)
	}
}

func TestMeansLookupCannotMutateAnotherScopeWithTheSameBody(t *testing.T) {
	command := func(_ context.Context, _ []byte, args ...string) ([]byte, error) {
		if args[3] != "show" {
			t.Fatal("wrong-scope lookup reached mutation")
		}
		return []byte(`{"hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","body":"","frontmatter":{"data":{"scope":{"room":"other"},"generation":0}}}`), nil
	}
	s, err := Open(t.TempDir(), "account", command)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	scope := Scope{Room: "room"}
	if err := s.Append(scope, Entry{Key: "turn", State: "delivered"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Summarize(scope, 0, 0, "", []string{"Private note."}); err == nil {
		t.Fatal("wrong scope accepted")
	}
}

func TestReadReturnsDetachedPendingEvidence(t *testing.T) {
	s, err := Open(t.TempDir(), "account", testmeans.Command)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	scope := Scope{Room: "room"}
	e := Entry{Key: "turn", Inputs: []string{"original"}, Receipts: []Receipt{{Result: "original receipt"}}}
	if err := s.Append(scope, e); err != nil {
		t.Fatal(err)
	}
	e.Inputs[0] = "changed by caller"
	d, err := s.Read(scope)
	if err != nil {
		t.Fatal(err)
	}
	d.Entries[0].Inputs[0] = "changed"
	d.Entries[0].Receipts[0].Result = "changed"
	d, err = s.Read(scope)
	if err != nil || d.Entries[0].Inputs[0] != "original" || d.Entries[0].Receipts[0].Result != "original receipt" {
		t.Fatal("reader mutated pending store evidence", err)
	}
}

func TestSnapshotReplacementPreservesEarlierUncertainty(t *testing.T) {
	s, err := Open(t.TempDir(), "account", testmeans.Command)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	scope := Scope{Room: "room"}
	if err := s.StageSnapshot(scope, Entry{Key: "failed", State: "failed", Receipts: []Receipt{{Name: "write", Result: "unknown"}}}, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.StageSnapshot(scope, Entry{Key: "later", State: "delivered"}, 0, 0); err != nil {
		t.Fatal(err)
	}
	d, err := s.Read(scope)
	if err != nil || len(d.Entries) != 1 || d.Entries[0].Key != "later" {
		t.Fatal("snapshots accumulated", err)
	}
	if err := s.Summarize(scope, 0, 0, "", []string{}); err != nil {
		t.Fatal(err)
	}
	d, err = s.Read(scope)
	if err != nil || !d.Uncertain || d.LastKey != "later" {
		t.Fatal("filler checkpoint lost prior uncertainty", err)
	}
}
