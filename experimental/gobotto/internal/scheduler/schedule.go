package scheduler

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	_ "time/tzdata" // Keep IANA timezone schedules available in minimal deployments.

	"github.com/robfig/cron/v3"
)

var scheduleParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// parseSchedule normalizes whitespace and makes the default timezone explicit.
// The library owns calendar/DST semantics. Host guards prevent sub-minute work,
// silently rounded interval values, and the library's bare-TZ-prefix panic.
func parseSchedule(spec string) (string, cron.Schedule, error) {
	if len(spec) > 200 {
		return "", nil, errors.New("schedule exceeds 200 bytes")
	}
	fields := strings.Fields(spec)
	if len(fields) == 0 {
		return "", nil, errors.New("schedule is required")
	}
	if strings.HasPrefix(fields[0], "TZ=") || strings.HasPrefix(fields[0], "CRON_TZ=") {
		zone := strings.SplitN(fields[0], "=", 2)[1]
		if len(fields) < 2 || zone == "" || zone == "Local" {
			return "", nil, errors.New("supply an explicit IANA timezone and a cron expression")
		}
		if _, err := time.LoadLocation(zone); err != nil {
			return "", nil, errors.New("unknown schedule timezone")
		}
		if fields[1] == "@every" {
			return "", nil, errors.New("@every uses elapsed time and must not include a timezone")
		}
		fields[0] = "CRON_TZ=" + zone
	} else if fields[0] != "@every" {
		fields = append([]string{"CRON_TZ=UTC"}, fields...)
	}
	if fields[0] == "@every" {
		if len(fields) != 2 {
			return "", nil, errors.New("use @every followed by one duration")
		}
		duration, err := time.ParseDuration(fields[1])
		if err != nil || duration < time.Minute || duration > 30*24*time.Hour || duration%time.Second != 0 {
			return "", nil, errors.New("@every requires whole seconds from one minute to 30 days")
		}
		fields[1] = duration.String()
	}
	spec = strings.Join(fields, " ")
	schedule, err := scheduleParser.Parse(spec)
	if err != nil {
		return "", nil, errors.New("invalid schedule: use five-field cron, a named descriptor, or @every duration")
	}
	return spec, schedule, nil
}

func (d Definition) next(after time.Time) time.Time {
	_, schedule, err := parseSchedule(d.Schedule)
	if err != nil {
		return time.Time{}
	}
	return schedule.Next(after.UTC())
}

// scheduleBrainDefinition defines the code-owned recurring task collection.
func scheduleBrainDefinition() []byte {
	properties := map[string]any{}
	for _, name := range []string{"id", "title", "created_by", "authorized_by"} {
		properties[name] = map[string]any{"type": "string"}
	}
	properties["schedule"] = map[string]any{"type": "string", "maxLength": 200}
	text := map[string]any{"type": "string", "minLength": 1}
	properties["destination"] = map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"kind": map[string]any{"enum": []string{"room", "thread"}}, "room_id": text, "thread_id": text}, "required": []string{"kind", "room_id"}, "oneOf": []any{map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "room"}}, "not": map[string]any{"required": []string{"thread_id"}}}, map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "thread"}}, "required": []string{"thread_id"}}}}
	properties["created_from"] = map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"room_id": text, "thread_id": text}, "required": []string{"room_id", "thread_id"}}
	properties["revision"] = map[string]any{"type": "integer", "minimum": 1}
	properties["enabled"] = map[string]any{"type": "boolean"}
	schema := map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": []string{"id", "title", "schedule", "destination", "created_from", "created_by", "authorized_by", "revision", "enabled"}}
	header, _ := json.Marshal(map[string]any{"title": "Schedules", "description": "Owner-authorized recurring tasks managed by Gobotto", "schema": schema})
	return []byte("---\n" + string(header) + "\n---\nUse schedule_create, schedule_list, schedule_update, schedule_pause, schedule_resume, and schedule_delete. Configured admins manage schedules globally. Supply the current revision for mutations. Generic memory tools cannot write here. The private ledger authorizes structured definition revisions; manual changes do not authorize execution.\n")
}
