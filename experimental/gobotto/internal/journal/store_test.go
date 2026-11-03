package journal

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopeIsolationPersistenceAndInvalidation(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "server/bot")
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
	s, err = Open(dir, "server/bot")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	d, _ := s.Read(scopes[0])
	if err := s.Summarize(scopes[0], d.Revision, d.Generation, "A bounded summary", []string{"A fact"}); err != nil {
		t.Fatal(err)
	}
	d, _ = s.Read(scopes[0])
	if d.Summary != "A bounded summary" || d.Entries[0].Inputs != nil || d.Entries[0].Note != "A fact" {
		t.Fatal("summary did not replace raw input")
	}
	if err := s.Forget("one"); err != nil {
		t.Fatal(err)
	}
	for _, scope := range scopes {
		d, _ := s.Read(scope)
		if (len(d.Entries) == 0) != (scope.Room == "one") {
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
	s, err := Open(t.TempDir(), "account")
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
	if d.Entries[0].Receipts[0] != e.Receipts[0] {
		t.Fatal("host receipt changed")
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
	if err := s.Summarize(scope, d.Revision, d.Generation, "A decision.", []string{"A brief fact."}); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(binary, "--path", s.dir, "--json", "list").Output()
	var result struct {
		Documents   []json.RawMessage
		Diagnostics []json.RawMessage
	}
	if err != nil || json.Unmarshal(out, &result) != nil || len(result.Documents) != 1 || len(result.Diagnostics) != 0 {
		t.Fatal("journal is not means-readable", err, string(out))
	}
}
