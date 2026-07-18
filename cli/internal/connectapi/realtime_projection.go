package connectapi

import (
	"context"
	"errors"
	"fmt"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/parallel"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	realtimeProjectionTimelineLimit = 50
	realtimeProjectionUserPageSize  = 500
)

// RealtimeProjectionSnapshot is the current public, caller-authorized state
// emitted as the compacted prefix of a fresh realtime projection stream.
// Plaintext exists only in this per-request result; owning projections retain
// encrypted source fields and hydrate them at this boundary.
type RealtimeProjectionSnapshot struct {
	Server        *apiv1.ServerPublicProfile
	ServerState   *RealtimeProjectionServerState
	Viewer        *apiv1.GetViewerResponse
	Users         []*apiv1.DirectoryMember
	Rooms         []*RealtimeProjectionRoom
	RoomGroups    []*apiv1.RoomGroup
	Timelines     []*RealtimeProjectionRoomTimeline
	Notifications *RealtimeProjectionNotifications
	ActiveCalls   []*apiv1.ActiveCall
}

// RealtimeProjectionServerState is authenticated server state carried by the
// projection protocol in addition to the public discovery profile.
type RealtimeProjectionServerState struct {
	MOTD    string
	Runtime *apiv1.ServerRuntimeConfig
}

// RealtimeProjectionRoom is lightweight room state retained for every visible
// room. Membership is represented by IDs because the same snapshot already
// carries every public directory user exactly once; timelines are hydrated
// independently when first viewed.
type RealtimeProjectionRoom struct {
	Room                    *apiv1.RoomWithViewerState
	MemberUserIDs           []string
	ViewerNotificationCount uint32
}

// RealtimeProjectionRoomTimeline identifies one compacted recent room window.
type RealtimeProjectionRoomTimeline struct {
	RoomID       string
	Page         *apiv1.RoomTimelinePage
	EventCursors map[string]string
}

// RealtimeProjectionNotifications is the finite current notification page and
// complete per-room counts reconciled on bootstrap and every socket resume.
type RealtimeProjectionNotifications struct {
	Page       *apiv1.ListNotificationsResponse
	RoomCounts []*apiv1.RoomNotificationCount
}

// RealtimeProjectionRoomViewerState is one latest-value room read/permission
// row reconciled on every subscription.
type RealtimeProjectionRoomViewerState struct {
	RoomID      string
	ViewerState *apiv1.RoomViewerState
}

// RealtimeProjectionThreadViewerState is one followed thread's current
// viewer-specific state. Absence from the complete replacement means false.
type RealtimeProjectionThreadViewerState struct {
	RoomID            string
	ThreadRootEventID string
	ViewerState       *apiv1.ThreadViewerState
}

// BuildRealtimeProjectionPresences returns complete latest-value presence for
// the server directory. Presence is transient, so realtime subscriptions use
// this to reconcile state that cannot be recovered from EVT replay.
func (a *API) BuildRealtimeProjectionPresences(ctx context.Context) (map[string]apiv1.PresenceStatus, error) {
	statuses := make(map[string]apiv1.PresenceStatus)
	for offset := 0; ; offset += realtimeProjectionUserPageSize {
		members, total, err := a.core.GetServerMembers(ctx, "", realtimeProjectionUserPageSize, offset)
		if err != nil {
			return nil, err
		}
		userIDs := make([]string, 0, len(members))
		for _, member := range members {
			if member.UserID != "" {
				userIDs = append(userIDs, member.UserID)
			}
		}
		presences, err := a.core.GetUserPresences(ctx, userIDs)
		if err != nil {
			return nil, err
		}
		for _, userID := range userIDs {
			statuses[userID] = corePresenceStatusToAPI(presences[userID])
		}
		if offset+len(members) >= total || len(members) == 0 {
			return statuses, nil
		}
	}
}

// BuildRealtimeProjectionRoomViewerStates returns current per-room read and
// permission state. Read markers live outside EVT, so durable replay alone
// cannot reconstruct changes made by another client during a disconnect.
func (a *API) BuildRealtimeProjectionRoomViewerStates(ctx context.Context, userID string) ([]*RealtimeProjectionRoomViewerState, error) {
	rooms, err := a.core.RoomDirectoryReads().ListRooms(ctx, userID, core.RoomDirectoryListOptions{
		IncludeChannels: true,
		IncludeDMs:      true,
		IncludeEmptyDMs: true,
	})
	if err != nil {
		return nil, err
	}
	states := make([]*RealtimeProjectionRoomViewerState, 0, len(rooms))
	for _, room := range rooms {
		apiRoom := apiRoomWithViewerState(room)
		if apiRoom.GetRoom().GetId() == "" {
			continue
		}
		states = append(states, &RealtimeProjectionRoomViewerState{
			RoomID:      apiRoom.GetRoom().GetId(),
			ViewerState: apiRoom.GetViewerState(),
		})
	}
	return states, nil
}

// BuildRealtimeProjectionThreadViewerStates returns the complete followed
// thread set, including RUNTIME_STATE-backed unread markers.
func (a *API) BuildRealtimeProjectionThreadViewerStates(ctx context.Context, userID string) ([]*RealtimeProjectionThreadViewerState, error) {
	threads, err := a.core.ThreadFollows().ListFollowedThreadViewerStates(ctx, userID)
	if err != nil {
		return nil, err
	}
	states := make([]*RealtimeProjectionThreadViewerState, 0, len(threads))
	for _, thread := range threads {
		if thread == nil || thread.RoomID == "" || thread.ThreadRootEventID == "" {
			continue
		}
		following := true
		hasUnread := thread.HasUnread
		states = append(states, &RealtimeProjectionThreadViewerState{
			RoomID:            thread.RoomID,
			ThreadRootEventID: thread.ThreadRootEventID,
			ViewerState: &apiv1.ThreadViewerState{
				IsFollowing: &following,
				HasUnread:   &hasUnread,
			},
		})
	}
	return states, nil
}

// BuildRealtimeProjectionSnapshot assembles the finite current state used by a
// fresh projection stream. It deliberately shares the same public assemblers
// as ConnectRPC so deletion, crypto-shredding, attachment tickets, and viewer
// authorization have one implementation.
func (a *API) BuildRealtimeProjectionSnapshot(ctx context.Context, userID string, timelineRoomIDs []string) (*RealtimeProjectionSnapshot, error) {
	ctx = core.WithDEKRequestCache(ctx)
	retainedRoomIDs := make(map[string]struct{}, len(timelineRoomIDs))
	for _, roomID := range timelineRoomIDs {
		retainedRoomIDs[roomID] = struct{}{}
	}

	server, err := a.serverProfile(ctx, serverProfileOptions{})
	if err != nil {
		return nil, fmt.Errorf("assemble realtime server profile: %w", err)
	}
	serverState, err := a.BuildRealtimeProjectionServerState(ctx)
	if err != nil {
		return nil, fmt.Errorf("assemble realtime authenticated server state: %w", err)
	}
	viewer, err := a.buildViewer(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assemble realtime viewer: %w", err)
	}
	users, err := a.realtimeProjectionUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("assemble realtime users: %w", err)
	}
	rooms, err := a.core.RoomDirectoryReads().ListRooms(ctx, userID, core.RoomDirectoryListOptions{
		IncludeChannels: true,
		IncludeDMs:      true,
		IncludeEmptyDMs: true,
	})
	if err != nil {
		return nil, fmt.Errorf("assemble realtime rooms: %w", err)
	}
	notifications, err := a.BuildRealtimeProjectionNotifications(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assemble realtime notifications: %w", err)
	}
	activeCalls, err := a.BuildRealtimeProjectionActiveCalls(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assemble realtime active calls: %w", err)
	}
	notificationCounts := make(map[string]uint32, len(notifications.RoomCounts))
	for _, count := range notifications.RoomCounts {
		notificationCounts[count.GetRoomId()] = uint32(max(count.GetTotalCount(), 0))
	}
	apiRooms := make([]*RealtimeProjectionRoom, 0, len(rooms))
	memberRooms := make(map[string]*core.DirectoryRoom, len(rooms))
	for _, room := range rooms {
		_, includeMembership := retainedRoomIDs[room.Room.GetId()]
		apiRoom, err := a.realtimeProjectionRoom(ctx, userID, room, notificationCounts[room.Room.GetId()], includeMembership)
		if err != nil {
			return nil, fmt.Errorf("assemble realtime room %q: %w", room.Room.GetId(), err)
		}
		apiRooms = append(apiRooms, apiRoom)
		if room != nil && room.ViewerState.IsMember {
			memberRooms[room.Room.GetId()] = room
		}
	}

	groups, err := a.core.RoomDirectoryReads().ListRoomGroups(ctx, userID, core.RoomDirectoryGroupOptions{})
	if err != nil {
		return nil, fmt.Errorf("assemble realtime room groups: %w", err)
	}
	apiGroups := make([]*apiv1.RoomGroup, 0, len(groups))
	for _, group := range groups {
		apiGroups = append(apiGroups, apiRoomGroup(group))
	}

	requestedRooms := make([]*core.DirectoryRoom, 0, len(timelineRoomIDs))
	seenTimelineRooms := make(map[string]struct{}, len(timelineRoomIDs))
	for _, roomID := range timelineRoomIDs {
		if _, seen := seenTimelineRooms[roomID]; seen {
			continue
		}
		seenTimelineRooms[roomID] = struct{}{}
		if room := memberRooms[roomID]; room != nil {
			requestedRooms = append(requestedRooms, room)
		}
	}
	timelines, err := parallel.MapNonNil(ctx, maxConnectAPIHydrationConcurrency, requestedRooms, func(ctx context.Context, _ int, room *core.DirectoryRoom) (*RealtimeProjectionRoomTimeline, error) {
		return a.BuildRealtimeProjectionRoomTimeline(ctx, userID, room.Room.GetId())
	})
	if err != nil {
		return nil, fmt.Errorf("assemble realtime timelines: %w", err)
	}

	return &RealtimeProjectionSnapshot{
		Server:        server,
		ServerState:   serverState,
		Viewer:        viewer,
		Users:         users,
		Rooms:         apiRooms,
		RoomGroups:    apiGroups,
		Timelines:     timelines,
		Notifications: notifications,
		ActiveCalls:   activeCalls,
	}, nil
}

// BuildRealtimeProjectionActiveCalls returns the complete current call state
// visible to one viewer.
func (a *API) BuildRealtimeProjectionActiveCalls(ctx context.Context, userID string) ([]*apiv1.ActiveCall, error) {
	if !a.config.LiveKit.IsConfigured() {
		return nil, nil
	}
	roomIDs, err := a.core.GetActiveCallRoomIDs(ctx, core.LegacySpaceIDForRoomKind(core.KindChannel))
	if err != nil {
		return nil, err
	}
	service := &voiceCallService{api: a}
	calls := make([]*apiv1.ActiveCall, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		call, err := service.activeCall(ctx, userID, roomID)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) || errors.Is(err, core.ErrPermissionDenied) || errors.Is(err, core.ErrNotRoomMember) {
				continue
			}
			return nil, err
		}
		calls = append(calls, call)
	}
	return calls, nil
}

// BuildRealtimeProjectionRoomTimeline returns the current recent window for a
// joined room. It is used by both compacted bootstrap and live membership
// acquisition so a room enters the client projection with identical state.
func (a *API) BuildRealtimeProjectionRoomTimeline(ctx context.Context, userID, roomID string) (*RealtimeProjectionRoomTimeline, error) {
	result, err := a.core.RoomTimelineReads().GetRoomEvents(ctx, core.RoomTimelineEventsInput{
		ActorID: userID,
		RoomID:  roomID,
		Limit:   realtimeProjectionTimelineLimit,
	})
	if err != nil {
		return nil, err
	}
	page := result.Page
	apiPage, err := newRoomTimelineAssembler(a).buildPage(ctx, userID, result.Kind, page.Events, page.HasOlder, page.HasNewer)
	if err != nil {
		return nil, err
	}
	apiPage.StartCursor, err = a.formatRoomTimelineCursor(userID, roomID, "", page.StartCursorSeq)
	if err != nil {
		return nil, err
	}
	apiPage.EndCursor, err = a.formatRoomTimelineCursor(userID, roomID, "", page.EndCursorSeq)
	if err != nil {
		return nil, err
	}
	eventCursors := make(map[string]string, len(page.Events))
	for _, event := range page.Events {
		if event != nil && event.Event != nil {
			cursor, err := a.formatRoomTimelineCursor(userID, roomID, "", event.Sequence)
			if err != nil {
				return nil, err
			}
			eventCursors[event.Event.Id] = cursor
		}
	}
	return &RealtimeProjectionRoomTimeline{RoomID: roomID, Page: apiPage, EventCursors: eventCursors}, nil
}

// BuildRealtimeProjectionServerState returns current authenticated server
// presentation and runtime settings for snapshot and live convergence.
func (a *API) BuildRealtimeProjectionServerState(ctx context.Context) (*RealtimeProjectionServerState, error) {
	service := &serverService{api: a}
	motd, err := service.serverMotd(ctx)
	if err != nil {
		return nil, err
	}
	return &RealtimeProjectionServerState{MOTD: motd, Runtime: service.serverRuntimeConfig()}, nil
}

func (a *API) realtimeProjectionUsers(ctx context.Context) ([]*apiv1.DirectoryMember, error) {
	var out []*apiv1.DirectoryMember
	for offset := 0; ; offset += realtimeProjectionUserPageSize {
		members, total, err := a.core.GetServerMembers(ctx, "", realtimeProjectionUserPageSize, offset)
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			user, err := a.core.GetUser(ctx, member.UserID)
			if err != nil {
				if errors.Is(err, core.ErrNotFound) {
					continue
				}
				return nil, err
			}
			apiMember, err := directoryMember(ctx, a, user, member.Roles)
			if err != nil {
				return nil, err
			}
			out = append(out, apiMember)
		}
		if offset+len(members) >= total || len(members) == 0 {
			return out, nil
		}
	}
}

// BuildRealtimeProjectionRoom returns current viewer-authorized room state.
// Inaccessible and deleted rooms return the same domain errors as ConnectRPC.
func (a *API) BuildRealtimeProjectionRoom(ctx context.Context, userID, roomID string) (*RealtimeProjectionRoom, error) {
	return a.buildRealtimeProjectionRoom(ctx, userID, roomID, true)
}

// BuildRealtimeProjectionRoomSummary returns sidebar/rendering state without
// eagerly materialising channel membership. DM participant IDs remain eager
// because they define the conversation identity shown in navigation.
func (a *API) BuildRealtimeProjectionRoomSummary(ctx context.Context, userID, roomID string) (*RealtimeProjectionRoom, error) {
	return a.buildRealtimeProjectionRoom(ctx, userID, roomID, false)
}

func (a *API) buildRealtimeProjectionRoom(ctx context.Context, userID, roomID string, includeChannelMembership bool) (*RealtimeProjectionRoom, error) {
	room, err := a.core.RoomDirectoryReads().GetRoom(ctx, userID, roomID)
	if err != nil {
		return nil, err
	}
	counts, err := a.realtimeProjectionNotificationCounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	return a.realtimeProjectionRoom(ctx, userID, room, counts[roomID], includeChannelMembership)
}

func (a *API) realtimeProjectionRoom(ctx context.Context, userID string, room *core.DirectoryRoom, notificationCount uint32, includeChannelMembership bool) (*RealtimeProjectionRoom, error) {
	if room == nil || room.Room == nil {
		return nil, core.ErrNotFound
	}
	// Directory-visible rooms are part of the server projection even before
	// the viewer joins. Their member list is not authorized at that point and
	// is not needed until the room becomes a joined-room projection.
	if !room.ViewerState.IsMember {
		return &RealtimeProjectionRoom{Room: apiRoomWithViewerState(room), ViewerNotificationCount: notificationCount}, nil
	}
	if room.Room.GetKind() != corev1.RoomKind_ROOM_KIND_DM && !includeChannelMembership {
		return &RealtimeProjectionRoom{Room: apiRoomWithViewerState(room), ViewerNotificationCount: notificationCount}, nil
	}
	members, err := a.core.ListRoomMemberReferencesForList(ctx, userID, room.Room.GetId())
	if err != nil {
		return nil, err
	}
	memberIDs := make([]string, 0, len(members))
	for _, member := range members {
		if member.GetId() != "" {
			memberIDs = append(memberIDs, member.GetId())
		}
	}
	return &RealtimeProjectionRoom{Room: apiRoomWithViewerState(room), MemberUserIDs: memberIDs, ViewerNotificationCount: notificationCount}, nil
}

func (a *API) realtimeProjectionNotificationCounts(ctx context.Context, userID string) (map[string]uint32, error) {
	notifications, err := a.core.GetNotifications(ctx, userID)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]uint32)
	for _, notification := range notifications {
		if roomID := notificationTargetRoomID(notification); roomID != "" {
			counts[roomID]++
		}
	}
	return counts, nil
}

// BuildRealtimeProjectionRoomViewerState returns current viewer-specific room
// state without hydrating room membership.
func (a *API) BuildRealtimeProjectionRoomViewerState(ctx context.Context, userID, roomID string) (*apiv1.RoomViewerState, error) {
	room, err := a.core.RoomDirectoryReads().GetRoom(ctx, userID, roomID)
	if err != nil {
		return nil, err
	}
	apiRoom := apiRoomWithViewerState(room)
	return apiRoom.GetViewerState(), nil
}

// BuildRealtimeProjectionNotifications returns the viewer's newest pending
// notification page plus complete room counts. It is intentionally emitted on
// every resume because RUNTIME_STATE notification mutations have no EVT cursor.
func (a *API) BuildRealtimeProjectionNotifications(ctx context.Context, userID string) (*RealtimeProjectionNotifications, error) {
	notifications, err := a.core.GetNotifications(ctx, userID)
	if err != nil {
		return nil, err
	}
	page, err := newNotificationAssembler(a).pageFromList(ctx, notifications, &apiv1.PageRequest{Limit: defaultNotificationLimit})
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int32)
	for _, notification := range notifications {
		if roomID := notificationTargetRoomID(notification); roomID != "" {
			counts[roomID]++
		}
	}
	roomCounts := make([]*apiv1.RoomNotificationCount, 0, len(counts))
	for roomID, count := range counts {
		roomCounts = append(roomCounts, &apiv1.RoomNotificationCount{RoomId: roomID, TotalCount: count})
	}
	return &RealtimeProjectionNotifications{Page: page, RoomCounts: roomCounts}, nil
}

// BuildRealtimeProjectionUser returns the current public directory row. PII is
// decrypted only while building this caller-specific response.
func (a *API) BuildRealtimeProjectionUser(ctx context.Context, userID string) (*apiv1.DirectoryMember, error) {
	user, err := a.core.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	roles, err := a.core.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	return directoryMember(ctx, a, user, append([]string{core.RoleEveryone}, roles...))
}

// BuildRealtimeProjectionRoomGroups returns the complete current visible layout.
func (a *API) BuildRealtimeProjectionRoomGroups(ctx context.Context, userID string) ([]*apiv1.RoomGroup, error) {
	groups, err := a.core.RoomDirectoryReads().ListRoomGroups(ctx, userID, core.RoomDirectoryGroupOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]*apiv1.RoomGroup, 0, len(groups))
	for _, group := range groups {
		out = append(out, apiRoomGroup(group))
	}
	return out, nil
}

// BuildRealtimeProjectionServer returns the current public server profile.
func (a *API) BuildRealtimeProjectionServer(ctx context.Context) (*apiv1.ServerPublicProfile, error) {
	return a.serverProfile(ctx, serverProfileOptions{})
}

// BuildRealtimeProjectionViewer returns the current authenticated viewer resource.
func (a *API) BuildRealtimeProjectionViewer(ctx context.Context, userID string) (*apiv1.GetViewerResponse, error) {
	return a.buildViewer(ctx, userID)
}

// BuildRealtimeProjectionTimelineEvent hydrates one current renderable room
// event. Message edits, reactions, and deletions should pass the original
// message event ID so replay never retransmits an obsolete body.
func (a *API) BuildRealtimeProjectionTimelineEvent(ctx context.Context, userID, roomID, eventID string) (*apiv1.RoomTimelineEvent, *apiv1.RoomTimelineIncludes, string, error) {
	result, err := a.core.RoomTimelineReads().GetTimelineEvent(ctx, userID, roomID, eventID)
	if err != nil {
		return nil, nil, "", err
	}
	event, includes, err := newRoomTimelineAssembler(a).hydrateEvent(ctx, userID, result.Kind, result.Event)
	if err != nil {
		return nil, nil, "", err
	}
	seq, err := a.core.GetEventSequence(ctx, result.Kind, roomID, eventID)
	if err != nil {
		return nil, nil, "", err
	}
	cursor, err := a.formatRoomTimelineCursor(userID, roomID, "", seq)
	if err != nil {
		return nil, nil, "", err
	}
	return event, includes, cursor, nil
}

// BuildRealtimeProjectionSourceTimelineEvent hydrates a source EVT fact that
// is itself visible in the room timeline, such as room lifecycle/membership.
func (a *API) BuildRealtimeProjectionSourceTimelineEvent(ctx context.Context, userID, roomID string, event *corev1.Event) (*apiv1.RoomTimelineEvent, *apiv1.RoomTimelineIncludes, string, error) {
	room, err := a.core.RoomDirectoryReads().GetRoom(ctx, userID, roomID)
	if err != nil {
		return nil, nil, "", err
	}
	kind := core.KindOfRoom(room.Room)
	timelineEvent, includes, err := newRoomTimelineAssembler(a).hydrateEvent(ctx, userID, kind, event)
	if err != nil {
		return nil, nil, "", err
	}
	seq, err := a.core.GetEventSequence(ctx, kind, roomID, event.GetId())
	if err != nil {
		return nil, nil, "", err
	}
	cursor, err := a.formatRoomTimelineCursor(userID, roomID, "", seq)
	if err != nil {
		return nil, nil, "", err
	}
	return timelineEvent, includes, cursor, nil
}
