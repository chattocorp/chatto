package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func change(t *testing.T, m *Manager, id string, rev int64, op, extra string) string {
	t.Helper()
	return call(t, m.Tools("elsewhere", "other-thread", "second-admin", true), "schedule_"+op, fmt.Sprintf(`{"id":%q,"revision":%d%s}`, id, rev, extra))
}
func mustDefinition(t *testing.T, m *Manager, id string) Definition {
	t.Helper()
	d, err := m.read(id, m.state.Entries[id])
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestGlobalRevisionedManagement(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	m.now = func() time.Time { return now }
	id := create(t, m)
	origin := mustDefinition(t, m, id).CreatedFrom
	out := change(t, m, id, 1, "update", `,"title":"Renamed","schedule":"@every 2m"`)
	if !strings.Contains(out, `"status":"complete"`) {
		t.Fatal(out)
	}
	d := mustDefinition(t, m, id)
	if d.Revision != 2 || d.Title != "Renamed" || d.Destination.Room != "room" || d.CreatedFrom != origin || d.AuthorizedBy != "second-admin" || d.Owner != "owner" {
		t.Fatal("update changed provenance or routing", d)
	}
	for _, op := range []string{"update", "pause", "resume", "delete"} {
		extra := ""
		if op == "update" {
			extra = `,"title":"Stale"`
		}
		if out := change(t, m, id, 1, op, extra); !strings.Contains(out, "Stale revision") {
			t.Fatal("stale write accepted", out)
		}
	}
	if out := change(t, m, id, 2, "pause", ""); !strings.Contains(out, `"enabled":false`) {
		t.Fatal(out)
	}
	now = now.Add(24 * time.Hour)
	if d, err := m.Claim(now, []string{"owner", "second-admin"}); err != nil || d != nil {
		t.Fatal("paused run claimed", err)
	}
	if out := change(t, m, id, 3, "update", `,"task":"New task","destination":"room"`); !strings.Contains(out, "complete") {
		t.Fatal(out)
	}
	d = mustDefinition(t, m, id)
	if d.Enabled || d.Destination != (Destination{Kind: "room", Room: "elsewhere"}) || d.CreatedFrom != origin {
		t.Fatal("paused update or destination incorrect", d)
	}
	m.Close()
	m, err = Open(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.now = func() time.Time { return now }
	if d := mustDefinition(t, m, id); d.Enabled || d.Revision != 4 {
		t.Fatal("paused state lost")
	}
	if out := change(t, m, id, 4, "resume", ""); !strings.Contains(out, `"enabled":true`) {
		t.Fatal(out)
	}
	if !m.state.Entries[id].Next.After(now) {
		t.Fatal("resume caught up missed runs")
	}
	if d, err := m.Claim(now, []string{"second-admin"}); err != nil || d != nil {
		t.Fatal("early resumed claim", err)
	}
	if out := change(t, m, id, 5, "delete", ""); !strings.Contains(out, `"deleted":true`) {
		t.Fatal(out)
	}
	if len(m.state.Entries) != 0 {
		t.Fatal("not deleted")
	}
}

func TestPauseRevokesClaimAndStaleFinishCannotOverwriteRevision(t *testing.T) {
	m, err := Open(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	id := create(t, m)
	now := time.Now().Add(time.Minute)
	claim, err := m.Claim(now, []string{"owner"})
	if err != nil || claim == nil {
		t.Fatal(err)
	}
	if out := change(t, m, id, 1, "update", `,"title":"Changed"`); !strings.Contains(out, "rejected") {
		t.Fatal("updated active claim")
	}
	if out := change(t, m, id, 1, "pause", ""); !strings.Contains(out, `"active_run":true`) {
		t.Fatal(out)
	}
	if m.Authorized(*claim) {
		t.Fatal("paused revision still authorized")
	}
	if out := change(t, m, id, 2, "resume", ""); !strings.Contains(out, "rejected") {
		t.Fatal("resumed before old claim finished")
	}
	if err := m.Finish(*claim, "complete"); err != nil {
		t.Fatal(err)
	}
	if e := m.state.Entries[id]; e.Status != "paused" || e.Revision != 2 || e.RunningRevision != 0 {
		t.Fatal("stale finish overwrote pause", e)
	}
	if out := change(t, m, id, 2, "resume", ""); !strings.Contains(out, "complete") {
		t.Fatal(out)
	}
	nextClaim, err := m.Claim(m.state.Entries[id].Next, []string{"second-admin"})
	if err != nil || nextClaim == nil {
		t.Fatal("resumed claim missing", err)
	}
	if err := m.Finish(*claim, "failed"); err != nil {
		t.Fatal(err)
	}
	if !m.Authorized(*nextClaim) {
		t.Fatal("old finish revoked new run")
	}
}

func TestStructuredApprovalSurvivesFormattingButRejectsTaskEdits(t *testing.T) {
	m, err := Open(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	id := create(t, m)
	d := mustDefinition(t, m, id)
	write := func() {
		t.Helper()
		b, _ := json.MarshalIndent(map[string]any{"data": documentMetadata{Definition: d}, "title": d.Title, "id": id}, "", "  ")
		if err := os.WriteFile(filepath.Join(m.dir, id+".md"), []byte("---\n"+string(b)+"\n---\n\n"+d.Task+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	mustDefinition(t, m, id)
	d.Task = "Unapproved task"
	write()
	if _, err := m.read(id, m.state.Entries[id]); err == nil {
		t.Fatal("unapproved task accepted")
	}
	if out := change(t, m, id, 1, "delete", ""); !strings.Contains(out, `"deleted":true`) {
		t.Fatal("invalid definition could not be deleted", out)
	}
}

func TestUncertainUpdateCannotAuthorizeNewRevision(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	id := create(t, m)
	path := filepath.Join(dir, ".runtime.json")
	old, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	for _, tool := range m.Tools("elsewhere", "root", "second-admin", true) {
		if tool.Name == "schedule_update" {
			_, err = tool.Execute(context.Background(), json.RawMessage(fmt.Sprintf(`{"id":%q,"revision":1,"task":"New uncommitted task"}`, id)))
			if err == nil {
				t.Fatal("uncertain update succeeded")
			}
		}
	}
	if d, err := m.Claim(time.Now().Add(time.Hour), []string{"owner", "second-admin"}); err == nil || d != nil {
		t.Fatal("continued after uncertain write")
	}
	m.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, old, 0600); err != nil {
		t.Fatal(err)
	}
	m, err = Open(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if d, err := m.Claim(time.Now().Add(time.Hour), []string{"owner", "second-admin"}); err != nil || d != nil {
		t.Fatal("uncommitted revision ran after restart")
	}
	if m.state.Entries[id].Status != "invalid" {
		t.Fatal("partial update not disabled")
	}
}

func TestPauseDoesNotRequireFutureOccurrence(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	id := create(t, m)
	d := mustDefinition(t, m, id)
	// A parsed calendar expression may have no future occurrence. Stopping it
	// must remain possible even when calculating the next run returns zero.
	d.Schedule = "0 0 31 2 *"
	if err := m.writeDefinition(d, m.state.Entries[id]); err != nil {
		t.Fatal(err)
	}
	if claim, err := m.Claim(time.Now().Add(time.Hour), []string{"owner"}); err != nil || claim != nil {
		t.Fatal("schedule without a future occurrence was claimed", err)
	}
	if m.state.Entries[id].RunningRevision != 0 {
		t.Fatal("invalid schedule left a phantom active run")
	}
	if out := change(t, m, id, 1, "pause", ""); !strings.Contains(out, `"enabled":false`) {
		t.Fatal(out)
	}
	m.Close()
	m, err = Open(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	if e := m.state.Entries[id]; e.Status != "paused" {
		t.Fatal("restart recalculated a paused schedule", e)
	}
}

func TestDuplicateCreateAtCapacityReturnsExistingSchedule(t *testing.T) {
	m, err := Open(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	id := create(t, m)
	// Other registered entries can be invalid; they still occupy capacity.
	for len(m.state.Entries) < maxSchedules {
		m.state.Entries[newID()] = Entry{Status: "invalid"}
	}
	if got := create(t, m); got != id {
		t.Fatal("duplicate created at capacity", got)
	}
}

func TestClaimChoosesOldestDueSchedule(t *testing.T) {
	m, err := Open(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	now := time.Now()
	for i, id := range []string{"000000000001", "ffffffffffff"} {
		d := Definition{ID: id, Title: "Task", Task: "Report", Schedule: "@every 1m", Destination: destination("room", "room", "root"), CreatedFrom: Origin{"room", "root"}, Owner: "owner", AuthorizedBy: "owner", Enabled: true, Revision: 1}
		if err := m.writeDefinition(d, Entry{Next: now.Add(-time.Duration(i+1) * time.Minute), Status: "ready"}); err != nil {
			t.Fatal(err)
		}
	}
	d, err := m.Claim(now, []string{"owner"})
	if err != nil || d == nil || d.ID != "ffffffffffff" {
		t.Fatal("oldest due task did not win", d, err)
	}
}

func TestTaskStoredAsMarkdownBody(t *testing.T) {
	m, err := Open(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	id := create(t, m)
	task := "# Instructions\n\nPost a **joke**.\n\n---\n\nKeep this separator and emoji 👍."
	extra, _ := json.Marshal(task)
	if out := change(t, m, id, 1, "update", `,"task":`+string(extra)); !strings.Contains(out, "complete") {
		t.Fatal(out)
	}
	b, err := os.ReadFile(filepath.Join(m.dir, id+".md"))
	if err != nil {
		t.Fatal(err)
	}
	header, body, ok := strings.Cut(string(b)[4:], "\n---\n")
	if !ok || strings.Contains(header, `"task"`) || strings.TrimSpace(body) != task {
		t.Fatal("task not stored only as Markdown body")
	}
	if d := mustDefinition(t, m, id); d.Task != task {
		t.Fatal("body did not round-trip")
	}
}
