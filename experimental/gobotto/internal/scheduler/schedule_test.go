package scheduler

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCalendarAndIntervalSchedules(t *testing.T) {
	for _, tc := range []struct{ spec, after, want string }{
		{"0 9 * * 1-5", "2026-11-06T10:00:00Z", "2026-11-09T09:00:00Z"},
		{"CRON_TZ=Europe/Berlin 0 9 * * 1-5", "2026-11-06T10:00:00Z", "2026-11-09T08:00:00Z"},
		{"@daily", "2026-11-02T12:00:00Z", "2026-11-03T00:00:00Z"},
		{"@every 90m", "2026-11-02T12:00:00Z", "2026-11-02T13:30:00Z"},
		// Berlin skips 02:30 in spring and repeats it in autumn.
		{"CRON_TZ=Europe/Berlin 30 2 * * *", "2026-03-28T02:00:00Z", "2026-03-30T00:30:00Z"},
		{"CRON_TZ=Europe/Berlin 30 2 * * *", "2026-10-25T00:30:00Z", "2026-10-25T01:30:00Z"},
	} {
		t.Run(tc.spec+tc.after, func(t *testing.T) {
			_, schedule, err := parseSchedule(tc.spec)
			if err != nil {
				t.Fatal(err)
			}
			after, _ := time.Parse(time.RFC3339, tc.after)
			if got := schedule.Next(after).UTC().Format(time.RFC3339); got != tc.want {
				t.Fatalf("next = %s, want %s", got, tc.want)
			}
		})
	}
	for _, spec := range []string{"", "CRON_TZ=Europe/Berlin", "TZ=", "TZ=Local @daily", "TZ=Unknown/Zone @daily", "CRON_TZ=UTC @every 1m", "* * * * * *", "@every 59s", "@every 60.5s", "@every 721h", "@reboot"} {
		if _, _, err := parseSchedule(spec); err == nil {
			t.Errorf("accepted %q", spec)
		}
	}
}

func TestCalendarFinishSkipsElapsedOccurrences(t *testing.T) {
	m, err := Open(t.TempDir(), "server/bot")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	out := call(t, m.Tools("room", "root", "owner", true), "schedule_create", `{"title":"Daily","task":"Report","schedule":"0 9 * * *","destination":"thread"}`)
	var receipt struct{ ID string }
	if json.Unmarshal([]byte(out), &receipt) != nil || receipt.ID == "" {
		t.Fatal(out)
	}
	now = now.Add(time.Hour)
	d, err := m.Claim(now, []string{"owner"})
	if err != nil || d == nil {
		t.Fatal("missing claim", err)
	}
	now = now.Add(49 * time.Hour)
	if err := m.Finish(*d, "complete"); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	if got := m.state.Entries[d.ID].Next; !got.Equal(want) {
		t.Fatalf("next = %v, want %v", got, want)
	}
	if d, err := m.Claim(now, []string{"owner"}); err != nil || d != nil {
		t.Fatal("catch-up after slow run", err)
	}
}
