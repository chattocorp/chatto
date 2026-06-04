package core

import (
	"slices"
	"testing"

	"hmans.de/chatto/internal/events"
)

func TestNarrowProjectionSubjectFilters(t *testing.T) {
	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{
			name: "reactions",
			got:  NewReactionProjection().Subjects(),
			want: []string{
				events.RoomEventTypeFilter(events.EventReactionAdded),
				events.RoomEventTypeFilter(events.EventReactionRemoved),
			},
		},
		{
			name: "content keys",
			got:  NewContentKeyProjection().Subjects(),
			want: []string{
				events.UserEventTypeFilter(events.EventUserDEKGenerated),
				events.UserEventTypeFilter(events.EventUserKeyShredded),
			},
		},
		{
			name: "config user cleanup",
			got:  NewConfigProjection().Subjects(),
			want: []string{
				events.UserEventTypeFilter(events.EventUserServerPreferencesChanged),
				events.UserEventTypeFilter(events.EventUserAccountDeleted),
			},
		},
		{
			name: "config server settings",
			got:  NewConfigProjection().Subjects(),
			want: []string{
				events.ConfigEventTypeFilter(events.EventServerNameChanged),
				events.ConfigEventTypeFilter(events.EventServerDescriptionChanged),
				events.ConfigEventTypeFilter(events.EventUserRoomNotificationLevelSet),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, want := range tc.want {
				if !slices.Contains(tc.got, want) {
					t.Fatalf("Subjects() = %v, missing %q", tc.got, want)
				}
			}
		})
	}

	for name, subjects := range map[string][]string{
		"reactions":    NewReactionProjection().Subjects(),
		"content keys": NewContentKeyProjection().Subjects(),
		"config":       NewConfigProjection().Subjects(),
	} {
		t.Run(name+" no firehose", func(t *testing.T) {
			for _, broad := range []string{events.RoomSubjectFilter(), events.UserSubjectFilter(), events.ConfigSubjectFilter()} {
				if slices.Contains(subjects, broad) {
					t.Fatalf("Subjects() = %v, should not include broad filter %q", subjects, broad)
				}
			}
		})
	}
}
