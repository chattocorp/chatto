package core

import (
	"slices"
	"testing"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/pkg/events"
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
			want: []string{evtstream.RoomSubjectFilter()},
		},
		{
			name: "notification visibility uses focused authorization state facts",
			got:  NewNotificationDecisionProjection().Subjects(),
			want: notificationDecisionProjectionSubjects(),
		},
		{
			name: "room membership uses room aggregate namespace",
			got:  NewRoomMembershipProjection().Subjects(),
			want: []string{evtstream.RoomSubjectFilter()},
		},
		{
			name: "room catalog uses room aggregate namespace",
			got:  NewRoomCatalogProjection().Subjects(),
			want: []string{evtstream.RoomSubjectFilter()},
		},
		{
			name: "call state uses room aggregate namespace",
			got:  NewCallStateProjection().Subjects(),
			want: []string{evtstream.RoomSubjectFilter()},
		},
		{
			name: "room group layout uses group namespace plus layout namespace",
			got:  NewRoomGroupLayoutProjection().Subjects(),
			want: []string{evtstream.GroupSubjectFilter(), evtstream.LayoutSubjectFilter()},
		},
		{
			name: "room groups use group aggregate namespace",
			got:  NewRoomGroupProjection().Subjects(),
			want: []string{evtstream.GroupSubjectFilter()},
		},
		{
			name: "config uses config aggregate namespace plus user extras",
			got:  NewConfigProjection().Subjects(),
			want: []string{
				evtstream.ConfigSubjectFilter(),
				evtstream.UserEventTypeFilter(evtstream.EventUserServerPreferencesChanged),
				evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted),
			},
		},
		{
			name: "reactions use room aggregate namespace",
			got:  NewReactionProjection().Subjects(),
			want: []string{evtstream.RoomSubjectFilter()},
		},
		{
			name: "room timeline uses room aggregate namespace plus key shredding",
			got:  NewRoomTimelineProjection().Subjects(),
			want: []string{
				evtstream.RoomSubjectFilter(),
				evtstream.UserEventTypeFilter(evtstream.EventUserKeyShreddingRequested),
				evtstream.UserEventTypeFilter(evtstream.EventUserKeyShredded),
			},
		},
		{
			name: "threads use focused room event families plus key shredding",
			got:  NewThreadProjection().Subjects(),
			want: []string{
				evtstream.RoomEventTypeFilter(evtstream.EventRoomCreated),
				evtstream.RoomEventTypeFilter(evtstream.EventRoomDeleted),
				evtstream.RoomEventTypeFilter(evtstream.EventThreadCreated),
				evtstream.RoomEventTypeFilter(evtstream.EventThreadFollowed),
				evtstream.RoomEventTypeFilter(evtstream.EventThreadUnfollowed),
				evtstream.RoomEventTypeFilter(evtstream.EventMessagePosted),
				evtstream.RoomEventTypeFilter(evtstream.EventMessageEdited),
				evtstream.RoomEventTypeFilter(evtstream.EventMessageRetracted),
				evtstream.UserEventTypeFilter(evtstream.EventUserKeyShreddingRequested),
				evtstream.UserEventTypeFilter(evtstream.EventUserKeyShredded),
			},
		},
		{
			name: "assets use lifecycle lanes plus message bodies that claim assets",
			got:  NewAssetProjection().Subjects(),
			want: []string{
				evtstream.AssetSubjectFilter(),
				evtstream.RoomEventTypeFilter(evtstream.EventAssetCreated),
				evtstream.RoomEventTypeFilter(evtstream.EventAssetProcessingStarted),
				evtstream.RoomEventTypeFilter(evtstream.EventAssetProcessingSucceeded),
				evtstream.RoomEventTypeFilter(evtstream.EventAssetProcessingFailed),
				evtstream.RoomEventTypeFilter(evtstream.EventAssetDeleted),
				evtstream.RoomEventTypeFilter(evtstream.EventMessageBody),
			},
		},
		{
			name: "content keys remain focused",
			got:  NewContentKeyProjection().Subjects(),
			want: []string{
				evtstream.UserEventTypeFilter(evtstream.EventUserDEKGenerated),
				evtstream.UserEventTypeFilter(evtstream.EventUserKeyShreddingRequested),
				evtstream.UserEventTypeFilter(evtstream.EventUserKeyShredded),
			},
		},
		{
			name: "user auth remains focused",
			got:  newUserAuthProjection().Subjects(),
			want: []string{
				evtstream.UserEventTypeFilter(evtstream.EventUserAccountCreated),
				evtstream.UserEventTypeFilter(evtstream.EventUserPasswordHashChanged),
				evtstream.UserEventTypeFilter(evtstream.EventUserOIDCSubjectLinked),
				evtstream.UserEventTypeFilter(evtstream.EventUserExternalIdentityLinked),
				evtstream.UserEventTypeFilter(evtstream.EventUserExternalIdentityUnlinked),
				evtstream.UserEventTypeFilter(evtstream.EventOAuthConsentGranted),
				evtstream.UserEventTypeFilter(evtstream.EventOAuthScopedConsentGranted),
				evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted),
				evtstream.UserEventTypeFilter(evtstream.EventUserKeyShreddingRequested),
				evtstream.UserEventTypeFilter(evtstream.EventUserKeyShredded),
				evtstream.UserEventTypeFilter(evtstream.EventBotAPIKeyCreated),
				evtstream.UserEventTypeFilter(evtstream.EventBotAPIKeyRotated),
				evtstream.UserEventTypeFilter(evtstream.EventBotAPIKeyAdded),
				evtstream.UserEventTypeFilter(evtstream.EventBotAPIKeyRevoked),
				evtstream.UserEventTypeFilter(evtstream.EventBotOwnerReassigned),
				evtstream.UserEventTypeFilter(evtstream.EventBotIncomingWebhookCreated),
				evtstream.UserEventTypeFilter(evtstream.EventBotIncomingWebhookRotated),
				evtstream.UserEventTypeFilter(evtstream.EventBotIncomingWebhookRevoked),
			},
		},
		{
			name: "mentionables uses stream-wide namespace",
			got:  NewMentionablesProjection(nil, nil).Subjects(),
			want: []string{evtstream.EventSubjectFilter()},
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
		"user auth":    newUserAuthProjection().Subjects(),
	} {
		t.Run(name, func(t *testing.T) {
			for _, broad := range []string{evtstream.RoomSubjectFilter(), evtstream.UserSubjectFilter(), evtstream.ConfigSubjectFilter()} {
				if slices.Contains(subjects, broad) {
					t.Fatalf("Subjects() = %v, should not include broad filter %q", subjects, broad)
				}
			}
		})
	}
}

func TestMultiLaneProjectionsUseSinglePhysicalReplayFilter(t *testing.T) {
	for name, projection := range map[string]events.ReplaySubjectProjection{
		"room timeline": NewRoomTimelineProjection(),
		"threads":       NewThreadProjection(),
		"assets":        NewAssetProjection(),
	} {
		t.Run(name, func(t *testing.T) {
			got := projection.ReplaySubjects()
			want := []string{evtstream.EventSubjectFilter()}
			if !slices.Equal(got, want) {
				t.Fatalf("ReplaySubjects() = %v, want %v", got, want)
			}
		})
	}
}
