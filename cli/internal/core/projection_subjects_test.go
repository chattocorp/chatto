package core

import (
	"slices"
	"testing"

	"hmans.de/chatto/internal/events"
)

func TestProjectionSubjectPolicy(t *testing.T) {
	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{
			name: "room directory uses room aggregate namespace",
			got:  NewRoomDirectoryProjection().Subjects(),
			want: []string{events.RoomSubjectFilter()},
		},
		{
			name: "room membership uses room aggregate namespace",
			got:  NewRoomMembershipProjection().Subjects(),
			want: []string{events.RoomSubjectFilter()},
		},
		{
			name: "room catalog uses room aggregate namespace",
			got:  NewRoomCatalogProjection().Subjects(),
			want: []string{events.RoomSubjectFilter()},
		},
		{
			name: "room group layout uses group namespace plus layout namespace",
			got:  NewRoomGroupLayoutProjection().Subjects(),
			want: []string{events.GroupSubjectFilter(), events.LayoutSubjectFilter()},
		},
		{
			name: "room groups use group aggregate namespace",
			got:  NewRoomGroupProjection().Subjects(),
			want: []string{events.GroupSubjectFilter()},
		},
		{
			name: "config uses config aggregate namespace plus user extras",
			got:  NewConfigProjection().Subjects(),
			want: []string{
				events.ConfigSubjectFilter(),
				events.UserEventTypeFilter(events.EventUserServerPreferencesChanged),
				events.UserEventTypeFilter(events.EventUserAccountDeleted),
			},
		},
		{
			name: "reactions remain focused",
			got:  NewReactionProjection().Subjects(),
			want: []string{
				events.RoomEventTypeFilter(events.EventReactionAdded),
				events.RoomEventTypeFilter(events.EventReactionRemoved),
			},
		},
		{
			name: "content keys remain focused",
			got:  NewContentKeyProjection().Subjects(),
			want: []string{
				events.UserEventTypeFilter(events.EventUserDEKGenerated),
				events.UserEventTypeFilter(events.EventUserKeyShredded),
			},
		},
		{
			name: "mentionables uses stream-wide namespace",
			got:  NewMentionablesProjection(nil, nil).Subjects(),
			want: []string{events.EventSubjectFilter()},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !slices.Equal(tc.got, tc.want) {
				t.Fatalf("Subjects() = %v, want %v", tc.got, tc.want)
			}
		})
	}
}

func TestFocusedProjectionsDoNotUseAggregateNamespaceFilters(t *testing.T) {
	for name, subjects := range map[string][]string{
		"reactions":    NewReactionProjection().Subjects(),
		"content keys": NewContentKeyProjection().Subjects(),
	} {
		t.Run(name, func(t *testing.T) {
			for _, broad := range []string{events.RoomSubjectFilter(), events.UserSubjectFilter(), events.ConfigSubjectFilter()} {
				if slices.Contains(subjects, broad) {
					t.Fatalf("Subjects() = %v, should not include broad filter %q", subjects, broad)
				}
			}
		})
	}
}
