package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
)

// toolArguments is decoded once; fields separately records which update values
// were supplied so omitted values do not overwrite the saved definition.
type toolArguments struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Task        string `json:"task"`
	Schedule    string `json:"schedule"`
	Destination string `json:"destination"`
	Revision    int64  `json:"revision"`
}

func feedback(message string) (string, error) {
	b, _ := json.Marshal(map[string]any{"status": "rejected", "mutated": false, "error": message})
	return string(b), nil
}

// Tools exposes global management to an entirely configured-admin-authored input.
// New destination choices bind to this input's room/thread. Scheduled runs and
// non-admin inputs must pass ownerOnly=false.
func (m *Manager) Tools(room, thread, owner string, ownerOnly bool) []agent.Tool {
	if m == nil || !ownerOnly || room == "" || thread == "" || owner == "" {
		return nil
	}
	var tools []agent.Tool
	for _, op := range []string{"create", "list", "delete", "update", "pause", "resume"} {
		properties := map[string]any{}
		required := []string{}
		description := "List ALL schedules on this bot across rooms and threads, including definitions, destinations, revisions and execution state. Any configured admin may manage any schedule. Use before changing a schedule; ask which one if ambiguous."
		if op == "create" || op == "update" {
			properties = map[string]any{"title": map[string]any{"type": "string", "maxLength": 120}, "task": map[string]any{"type": "string", "maxLength": 4000}, "schedule": map[string]any{"type": "string", "maxLength": 200, "description": "Five-field cron (UTC by default), optionally prefixed with CRON_TZ=Europe/Berlin; named descriptors such as @daily; or @every 1h30m. Minimum interval one minute."}}
			properties["destination"] = map[string]any{"type": "string", "enum": []string{"room", "thread"}, "description": "room posts a new top-level message in the current room; thread replies in the current thread. Match the owner's requested destination; ask if ambiguous. IDs are host-supplied."}
			required = []string{"title", "task", "schedule", "destination"}
			description = "On an explicit owner request, schedule a recurring task in the current room or thread. Use cron with an explicit timezone for local calendar times or @every for elapsed intervals. First execution is the next occurrence after creation. No catch-up or overlap. Future runs have no admin authority. Confirm the returned destination, schedule and next_run (UTC). Do not repeat uncertain creates; list first."
		}
		if op != "create" && op != "list" {
			properties["id"] = map[string]any{"type": "string"}
			properties["revision"] = map[string]any{"type": "integer", "minimum": 0, "description": "Current revision from schedule_list. Zero is only for deletion of obsolete invalid definitions."}
			required = []string{"id", "revision"}
			description = "On an explicit admin request, " + op + " a schedule anywhere on this bot. Supply the id and current revision from schedule_list; stale revisions are rejected. Confirm success only from a complete receipt. Never claim a schedule stopped without successful pause/delete. Active external effects may finish. Updates preserve enabled state and creation context. Supply only fields to change; omit destination to retain it. An explicit destination moves delivery to THIS conversation's room or thread; never infer a move because management occurs elsewhere."
		}
		operation := op
		tools = append(tools, agent.Tool{Name: "schedule_" + op, Description: description, Parameters: map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}, Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if len(raw) > 24<<10 {
				return feedback("Schedule arguments are too large")
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(raw, &fields) != nil || fields == nil {
				return feedback("Supply a JSON object")
			}
			for key := range fields {
				if _, ok := properties[key]; !ok {
					return feedback("Unexpected schedule argument")
				}
				if bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
					return feedback("Null schedule arguments are not supported")
				}
			}
			for _, key := range required {
				if len(fields[key]) == 0 {
					return feedback("Missing schedule argument")
				}
			}
			var a toolArguments
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&a) != nil || decoder.Decode(new(any)) != io.EOF {
				return feedback("Invalid schedule arguments")
			}
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.failed {
				return "", errStorage
			}
			switch operation {
			case "update", "pause", "resume":
				return m.mutate(operation, a, fields, room, thread, owner)
			case "create":
				if a.Destination != "room" && a.Destination != "thread" {
					return feedback("Choose destination room or thread")
				}
				d := Definition{Title: a.Title, Task: a.Task, Schedule: a.Schedule, Destination: destination(a.Destination, room, thread), CreatedFrom: Origin{room, thread}, Owner: owner, AuthorizedBy: owner, Enabled: true, Revision: 1}
				next, err := prepareDefinition(&d, m.now())
				if err != nil {
					return feedback(err.Error())
				}
				// Deduplicate exact creates, including a repeated tool call after a
				// completed receipt. Explicit identical jobs can use distinct titles.
				for id, e := range m.state.Entries {
					existing, err := m.read(id, e)
					if err == nil && existing.Owner == owner && existing.Title == d.Title && existing.Task == d.Task && existing.Schedule == d.Schedule && existing.Destination == d.Destination {
						if !existing.Enabled || e.Status == "revoked" || e.Status == "invalid" {
							return feedback("An identical schedule is disabled; delete it before recreating it")
						}
						return receipt(existing, e)
					}
				}
				if len(m.state.Entries) >= maxSchedules {
					return feedback("Schedule limit reached; delete an existing schedule first")
				}
				d.ID = newID()
				for {
					if _, exists := m.state.Entries[d.ID]; !exists {
						break
					}
					d.ID = newID()
				}
				e := Entry{Next: next, Status: "ready"}
				if err := m.writeDefinition(d, e); err != nil {
					return "", err
				}
				return receipt(d, m.state.Entries[d.ID])
			case "delete":
				if !validID(a.ID) {
					return feedback("Invalid schedule ID")
				}
				e, ok := m.state.Entries[a.ID]
				if !ok {
					return feedback("Schedule not found; use schedule_list")
				}
				if a.Revision != e.Revision {
					return feedback("Stale revision; use schedule_list again")
				}
				entries := clone(m.state.Entries)
				delete(entries, a.ID)
				if err := m.save(entries); err != nil {
					return "", err
				}
				if err := os.Remove(filepath.Join(m.dir, a.ID+".md")); err != nil && !os.IsNotExist(err) {
					m.failed = true
					return "", errStorage
				}
				return encode(map[string]any{"status": "complete", "id": a.ID, "revision": a.Revision, "deleted": true, "active_run": e.RunningRevision != 0, "guidance": "Future runs and pending delivery are revoked; completed or in-flight external effects may remain."})
			default:
				rows := []map[string]any{}
				invalid := 0
				for id, e := range m.state.Entries {
					d, err := m.read(id, e)
					if err != nil {
						invalid++
						rows = append(rows, map[string]any{"id": id, "revision": e.Revision, "status": "invalid", "guidance": "Disabled; an admin can delete this definition using its revision."})
						continue
					}
					rows = append(rows, scheduleRow(d, e))
				}
				sort.Slice(rows, func(i, j int) bool { return rows[i]["id"].(string) < rows[j]["id"].(string) })
				status := "complete"
				if invalid > 0 {
					status = "partial"
				}
				return encode(map[string]any{"status": status, "schedules": rows, "invalid_definitions": invalid, "guidance": "Invalid definitions cannot execute. An admin can delete and recreate them."})
			}
		}})
	}
	return tools
}
