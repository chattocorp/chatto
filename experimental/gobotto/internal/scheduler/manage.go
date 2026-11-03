package scheduler

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// prepareDefinition applies the same normalization and bounds to creation and
// edits. Pause does not use it: stopping must not require a future occurrence.
func prepareDefinition(d *Definition, now time.Time) (time.Time, error) {
	d.Title = strings.TrimSpace(d.Title)
	d.Task = strings.TrimSpace(d.Task)
	if d.Title == "" || d.Task == "" || len([]rune(d.Title)) > 120 || len([]rune(d.Task)) > 4000 {
		return time.Time{}, errors.New("Supply a nonempty title and task within their limits")
	}
	spec, parsed, err := parseSchedule(d.Schedule)
	if err != nil {
		return time.Time{}, err
	}
	next := parsed.Next(now.UTC())
	if next.IsZero() {
		return time.Time{}, errors.New("Schedule has no upcoming occurrence")
	}
	d.Schedule = spec
	return next, nil
}

func destination(kind, room, thread string) Destination {
	d := Destination{Kind: kind, Room: room}
	if kind == "thread" {
		d.Thread = thread
	}
	return d
}

// mutate requires the caller to hold m.mu. Every changed definition revokes the
// old revision; paused claims remain marked active until their worker finishes.
func (m *Manager) mutate(op string, a toolArguments, fields map[string]json.RawMessage, room, thread, admin string) (string, error) {
	id := a.ID
	e, ok := m.state.Entries[id]
	if !ok {
		return feedback("Schedule not found; use schedule_list")
	}
	if a.Revision != e.Revision {
		return feedback("Stale revision; use schedule_list again")
	}
	d, err := m.read(id, e)
	if err != nil {
		return feedback("Invalid definition; delete and recreate it")
	}
	if e.RunningRevision != 0 && op != "pause" {
		return feedback("A run is active; pause to revoke pending delivery, then wait for it to finish before updating or resuming")
	}
	if op == "update" && len(fields) == 2 {
		return feedback("Supply at least one field to update")
	}
	if op == "pause" && !d.Enabled || op == "resume" && d.Enabled && e.Status != "revoked" {
		return receipt(d, e)
	}
	if op == "update" {
		if _, ok := fields["title"]; ok {
			d.Title = a.Title
		}
		if _, ok := fields["task"]; ok {
			d.Task = a.Task
		}
		if _, ok := fields["schedule"]; ok {
			d.Schedule = a.Schedule
		}
		if _, ok := fields["destination"]; ok {
			if a.Destination != "room" && a.Destination != "thread" {
				return feedback("Choose destination room or thread")
			}
			d.Destination = destination(a.Destination, room, thread)
		}
	}
	if op == "pause" {
		d.Enabled = false
	}
	if op == "resume" {
		d.Enabled = true
	}
	if op != "pause" {
		next, err := prepareDefinition(&d, m.now())
		if err != nil {
			return feedback(err.Error())
		}
		if op == "resume" || fields["schedule"] != nil {
			e.Next = next
		}
	}
	d.Revision++
	d.AuthorizedBy = admin
	e.Status = "ready"
	if !d.Enabled {
		e.Status = "paused"
	}
	if err := m.writeDefinition(d, e); err != nil {
		return "", err
	}
	return receipt(d, m.state.Entries[id])
}

func scheduleRow(d Definition, e Entry) map[string]any {
	row := map[string]any{"id": d.ID, "title": d.Title, "task": d.Task, "schedule": d.Schedule, "destination": d.Destination, "created_from": d.CreatedFrom, "created_by": d.Owner, "authorized_by": d.AuthorizedBy, "revision": d.Revision, "enabled": d.Enabled, "status": e.Status, "active_run": e.RunningRevision != 0}
	if d.Enabled && e.Status != "revoked" && e.Status != "invalid" {
		row["next_run"] = e.Next
	}
	return row
}
func receipt(d Definition, e Entry) (string, error) {
	row := scheduleRow(d, e)
	row["state"] = row["status"]
	row["status"] = "complete"
	row["guidance"] = "Confirm only the resulting state. Completed or in-flight external effects cannot be undone."
	return encode(row)
}
func encode(v any) (string, error) { b, err := json.Marshal(v); return string(b), err }
