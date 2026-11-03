package scheduler

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
)

func call(t *testing.T, tools []agent.Tool, name, args string) string {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			out, err := tool.Execute(context.Background(), json.RawMessage(args))
			if err != nil {
				t.Fatal(err)
			}
			return out
		}
	}
	t.Fatal("tool not found", name)
	return ""
}

func create(t *testing.T, m *Manager) string {
	t.Helper()
	out := call(t, m.Tools("room", "root", "owner", true), "schedule_create", `{"title":"Deals","task":"Report free games","schedule":"@every 1m","destination":"thread"}`)
	var v struct{ ID string }
	if json.Unmarshal([]byte(out), &v) != nil || v.ID == "" {
		t.Fatal(out)
	}
	return v.ID
}

func TestScheduleClaimRestartAndNoReplay(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir, "server/bot")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	m.now = func() time.Time { return now }
	id := create(t, m)
	if create(t, m) != id || len(m.state.Entries) != 1 {
		t.Fatal("duplicate create")
	}
	if d, err := m.Claim(now, []string{"owner"}); err != nil || d != nil {
		t.Fatal("ran early", err)
	}
	now = now.Add(time.Minute)
	d, err := m.Claim(now, []string{"owner"})
	if err != nil || d == nil || d.ID != id || !m.Authorized(*d) {
		t.Fatal("due run missing", err)
	}
	if again, err := m.Claim(now.Add(time.Hour), []string{"owner"}); err != nil || again != nil {
		t.Fatal("overlap", err)
	}
	m.Close()
	m, err = Open(dir, "server/bot")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if m.state.Entries[id].Status != "interrupted" || m.Authorized(*d) {
		t.Fatal("replayed interrupted claim")
	}
	if again, err := m.Claim(time.Now(), []string{"owner"}); err != nil || again != nil {
		t.Fatal("startup catch-up", err)
	}
	if _, err := Open(dir, "server/bot"); err == nil {
		t.Fatal("second process acquired brain")
	}
}

func TestScheduleRevocationAndDefinitionIntegrity(t *testing.T) {
	for _, action := range []string{"delete", "owner removed", "tamper"} {
		t.Run(action, func(t *testing.T) {
			m, err := Open(t.TempDir(), "server/bot")
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			now := time.Now()
			m.now = func() time.Time { return now }
			id := create(t, m)
			now = now.Add(time.Minute)
			owners := []string{"owner"}
			if action == "owner removed" {
				owners = nil
			}
			if action == "tamper" {
				if err := os.WriteFile(filepath.Join(m.dir, id+".md"), []byte("changed task"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			d, err := m.Claim(now, owners)
			if err != nil {
				t.Fatal(err)
			}
			if action != "delete" {
				if d != nil {
					t.Fatal("unauthorized run")
				}
				return
			}
			if d == nil {
				t.Fatal("missing claim")
			}
			out := call(t, m.Tools("other-room", "other-thread", "another-admin", true), "schedule_delete", `{"id":"`+id+`","revision":1}`)
			if !strings.Contains(out, "complete") {
				t.Fatal("global admin deletion failed", out)
			}
			if m.Authorized(*d) {
				t.Fatal("deleted pending run remains authorized")
			}
			if err := m.Finish(*d, "complete"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestScheduleToolsAndDurableFailure(t *testing.T) {
	m, err := Open(t.TempDir(), "server/bot")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if len(m.Tools("room", "root", "owner", false)) != 0 || len(m.Tools("room", "root", "", true)) != 0 {
		t.Fatal("management without authority")
	}
	tools := m.Tools("room", "root", "owner", true)
	for _, raw := range []string{`null`, `{}`, `{"title":"X","task":"Y","destination":"thread","interval_seconds":60}`, `{"title":"X","task":"Y","destination":"thread","schedule":"@every 30s"}`, `{"title":"X","task":"Y","destination":"thread","schedule":"0 0 31 2 *"}`, `{"title":"X","task":"Y","destination":"thread","schedule":"@daily","room":"elsewhere"}`, `{"title":"X","task":"Y","destination":"thread","schedule":"@daily"} {}`} {
		if !strings.Contains(call(t, tools, "schedule_create", raw), "rejected") {
			t.Fatal("invalid create", raw)
		}
	}
	id := create(t, m)
	if !strings.Contains(call(t, m.Tools("other-room", "other", "other-admin", true), "schedule_list", `{}`), id) {
		t.Fatal("global admin listing omitted schedule")
	}
	// Make the ledger destination unwritable without relying on chmod/root behavior.
	if err := os.Remove(filepath.Join(m.dir, ".runtime.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(m.dir, ".runtime.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if d, err := m.Claim(time.Now().Add(time.Hour), []string{"owner"}); err == nil || d != nil {
		t.Fatal("executed without durable claim")
	}
	if d, err := m.Claim(time.Now().Add(time.Hour), []string{"owner"}); err == nil || d != nil {
		t.Fatal("continued after storage uncertainty")
	}
}

func TestScheduleScopeAndMissedRuns(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir, "first")
	if err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time { return time.Now().Add(-24 * time.Hour) }
	id := create(t, m)
	m.Close()
	if other, err := Open(dir, "second"); err == nil {
		other.Close()
		t.Fatal("cross-account schedule reuse")
	}
	m, err = Open(dir, "first")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if !m.state.Entries[id].Next.After(time.Now()) {
		t.Fatal("missed run not skipped")
	}
	if d, err := m.Claim(time.Now(), []string{"owner"}); err != nil || d != nil {
		t.Fatal("catch-up run", err)
	}
}

func TestScheduleDocumentsAreMeansReadable(t *testing.T) {
	binary, err := exec.LookPath("means")
	if err != nil {
		t.Skip("means not installed")
	}
	m, err := Open(t.TempDir(), "server/bot")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	id := create(t, m)
	out, err := exec.Command(binary, "--path", m.dir, "--json", "list").Output()
	var result struct {
		Documents   []struct{ ID string }
		Diagnostics []any
	}
	if err != nil || json.Unmarshal(out, &result) != nil || len(result.Diagnostics) != 0 || len(result.Documents) != 1 || result.Documents[0].ID != id {
		t.Fatal("schedule brain is not means-readable", err, string(out))
	}
}

func TestScheduleDestinationsAreExplicitAndBound(t *testing.T) {
	m, err := Open(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	tools := m.Tools("room", "root", "owner", true)
	for _, destination := range []string{"", "other-room", "room", "thread"} {
		args, _ := json.Marshal(map[string]string{"title": "Task", "task": "Report", "schedule": "@every 1m", "destination": destination})
		out := call(t, tools, "schedule_create", string(args))
		if destination == "" || destination == "other-room" {
			if !strings.Contains(out, "rejected") {
				t.Fatal(out)
			}
			continue
		}
		var receipt struct {
			ID          string
			Destination Destination
		}
		if json.Unmarshal([]byte(out), &receipt) != nil || receipt.Destination.Kind != destination {
			t.Fatal(out)
		}
		d, err := m.read(receipt.ID, m.state.Entries[receipt.ID])
		if err != nil || d.Destination.Kind != destination || d.Destination.Room != "room" || d.CreatedFrom.Thread != "root" || d.Owner != "owner" {
			t.Fatal("unbound destination", err)
		}
		if !strings.Contains(call(t, tools, "schedule_list", `{}`), `"kind":"`+destination+`"`) {
			t.Fatal("list omitted destination")
		}
	}
	if len(m.state.Entries) != 2 {
		t.Fatal("distinct destinations deduplicated")
	}
}

func TestSchemaRefreshDoesNotAuthorizeMissingDestination(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	id := create(t, m)
	d, err := m.read(id, m.state.Entries[id])
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(d)
	var fields map[string]any
	_ = json.Unmarshal(data, &fields)
	delete(fields, "destination")
	data, _ = json.Marshal(map[string]any{"id": id, "title": d.Title, "data": fields})
	document := []byte("---\n" + string(data) + "\n---\n")
	if err := atomicWrite(filepath.Join(dir, id+".md"), document); err != nil {
		t.Fatal(err)
	}
	e := m.state.Entries[id]
	e.Hash = digest(document)
	m.state.Entries[id] = e
	if err := m.save(m.state.Entries); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(dir, "MEANS.md"), []byte("obsolete schema")); err != nil {
		t.Fatal(err)
	}
	m.Close()
	m, err = Open(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if m.state.Entries[id].Status != "invalid" || m.state.Entries[id].Hash != e.Hash {
		t.Fatal("old task gained authorization")
	}
	got, err := os.ReadFile(filepath.Join(dir, id+".md"))
	if err != nil || string(got) != string(document) {
		t.Fatal("task was rewritten", err)
	}
	got, err = os.ReadFile(filepath.Join(dir, "MEANS.md"))
	if err != nil || string(got) != string(scheduleBrainDefinition()) {
		t.Fatal("schema was not refreshed", err)
	}
	if d, err := m.Claim(time.Now().Add(time.Hour), []string{"owner"}); err != nil || d != nil {
		t.Fatal("missing destination ran", err)
	}
}
