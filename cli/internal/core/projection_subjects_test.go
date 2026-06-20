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
			name: "call state uses room aggregate namespace",
			got:  NewCallStateProjection().Subjects(),
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
			name: "reactions use room aggregate namespace",
			got:  NewReactionProjection().Subjects(),
			want: []string{events.RoomSubjectFilter()},
		},
		{
			name: "threads use focused room event families plus key shredding",
			got:  NewThreadProjection().Subjects(),
			want: []string{
				events.RoomEventTypeFilter(events.EventThreadCreated),
				events.RoomEventTypeFilter(events.EventMessagePosted),
				events.RoomEventTypeFilter(events.EventMessageEdited),
				events.RoomEventTypeFilter(events.EventMessageRetracted),
				events.UserEventTypeFilter(events.EventUserKeyShredded),
			},
		},
		{
			name: "assets use canonical asset namespace plus legacy beta room asset lanes",
			got:  NewAssetProjection().Subjects(),
			want: []string{
				events.AssetSubjectFilter(),
				events.RoomEventTypeFilter(events.EventAssetCreated),
				events.RoomEventTypeFilter(events.EventAssetProcessingStarted),
				events.RoomEventTypeFilter(events.EventAssetProcessingSucceeded),
				events.RoomEventTypeFilter(events.EventAssetProcessingFailed),
				events.RoomEventTypeFilter(events.EventAssetDeleted),
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
		"content keys": NewContentKeyProjection().Subjects(),
		"threads":      NewThreadProjection().Subjects(),
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

func TestThreadProjectionReplaySubjectsUseSharedRoomReplay(t *testing.T) {
	got := NewThreadProjection().ReplaySubjects()
	want := []string{events.RoomSubjectFilter(), events.UserEventTypeFilter(events.EventUserKeyShredded)}
	if !slices.Equal(got, want) {
		t.Fatalf("ReplaySubjects() = %v, want %v", got, want)
	}
}
