package core

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

var notificationVisibilitySnapshotContractID = snapshotContractID("v1", &corev1.NotificationVisibilityProjectionSnapshot{})

// NotificationVisibilityProjection keeps the compact event-time state needed
// to derive notification recipients and policy while enforcing persistent
// privacy boundaries. The historical Go name mirrors its snapshot key; runtime
// diagnostics present it as the Notification Decisions projection.
type NotificationVisibilityProjection struct {
	mu sync.RWMutex

	rooms  *RoomDirectoryProjection
	groups *RoomGroupLayoutProjection
	rbac   *RBACProjection
	config *ConfigProjection

	activeUsers   map[string]struct{}
	threadFollows map[string]notificationThreadFollow
	followers     map[string]map[string]struct{}
	replyCounts   map[string]uint64

	// A pending run keeps one full checkpoint at its earliest boundary and a
	// compact event journal after it. This makes projector-ahead replay cost
	// O(state + events), rather than copying all visibility state once per
	// administrative fact.
	checkpointSequence uint64
	checkpoint         []byte
	deltas             []notificationVisibilityDelta
	boundaries         map[uint64]struct{}
	// Boundary calls are serialized by the single-lane durable worker. Keep a
	// decoded cursor so processing P pending facts replays each compact delta at
	// most once instead of repeatedly decoding the full checkpoint.
	evaluatorSequence   uint64
	evaluator           *notificationVisibilitySnapshot
	acknowledgedThrough atomic.Uint64
}

type notificationVisibilityDelta struct {
	sequence uint64
	event    *corev1.Event
}

type notificationThreadFollow struct {
	userID            string
	roomID            string
	threadRootEventID string
	state             ThreadFollowState
}

func NewNotificationVisibilityProjection() *NotificationVisibilityProjection {
	return &NotificationVisibilityProjection{
		rooms:         NewRoomDirectoryProjection(),
		groups:        NewRoomGroupLayoutProjection(),
		rbac:          NewRBACProjection(),
		config:        NewConfigProjection(),
		activeUsers:   make(map[string]struct{}),
		threadFollows: make(map[string]notificationThreadFollow),
		followers:     make(map[string]map[string]struct{}),
		replyCounts:   make(map[string]uint64),
		boundaries:    make(map[uint64]struct{}),
	}
}

func (*NotificationVisibilityProjection) Subjects() []string {
	return notificationVisibilityProjectionSubjects()
}

// ReplaySubjects uses one physical EVT filter. This projection's logical
// contract spans several sparse aggregate families; one broad scan avoids the
// JetStream multi-filter cost while Projector still rejects unrelated subjects
// before decoding or applying them.
func (*NotificationVisibilityProjection) ReplaySubjects() []string {
	return []string{evtstream.EventSubjectFilter()}
}

func notificationVisibilityProjectionSubjects() []string {
	return []string{
		evtstream.RoomEventTypeFilter(evtstream.EventRoomCreated),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomUniversalChanged),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomDeleted),
		evtstream.RoomEventTypeFilter(evtstream.EventUserJoinedRoom),
		evtstream.RoomEventTypeFilter(evtstream.EventUserLeftRoom),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomMemberBanned),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomMemberUnbanned),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomMemberRemoved),
		evtstream.RoomEventTypeFilter(evtstream.EventMessagePosted),
		evtstream.RoomEventTypeFilter(evtstream.EventReactionAdded),
		evtstream.RoomEventTypeFilter(evtstream.EventThreadFollowed),
		evtstream.RoomEventTypeFilter(evtstream.EventThreadUnfollowed),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomGroupCreated),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomGroupDeleted),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomAddedToGroup),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomRemovedFromGroup),
		evtstream.RBACSubjectFilter(),
		evtstream.ConfigSubjectFilter(),
		evtstream.UserEventTypeFilter(evtstream.EventUserAccountCreated),
		evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted),
	}
}

func (p *NotificationVisibilityProjection) Apply(event *corev1.Event, seq uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.rooms.Apply(event, seq); err != nil {
		return err
	}
	if err := p.groups.Apply(event, seq); err != nil {
		return err
	}
	if err := p.rbac.Apply(event, seq); err != nil {
		return err
	}
	if err := applyNotificationDecisionState(p.config, p.activeUsers, p.threadFollows, p.followers, p.replyCounts, event, seq); err != nil {
		return err
	}
	boundary := seq > p.acknowledgedThrough.Load() && notificationDecisionBoundaryEvent(event)
	if len(p.checkpoint) == 0 {
		if !boundary {
			return nil
		}
		payload, err := encodeNotificationVisibilityState(p.rooms, p.groups, p.rbac, p.config, p.activeUsers, p.threadFollows, p.replyCounts)
		if err != nil {
			return fmt.Errorf("capture notification visibility checkpoint %d: %w", seq, err)
		}
		p.checkpointSequence = seq
		p.checkpoint = payload
		p.boundaries[seq] = struct{}{}
		return nil
	}
	if seq > p.checkpointSequence {
		p.deltas = append(p.deltas, notificationVisibilityDelta{
			sequence: seq,
			event:    proto.Clone(event).(*corev1.Event),
		})
	}
	if boundary {
		p.boundaries[seq] = struct{}{}
	}
	return nil
}

func notificationDecisionBoundaryEvent(event *corev1.Event) bool {
	if event == nil {
		return false
	}
	switch event.GetEvent().(type) {
	case *corev1.Event_MessagePosted, *corev1.Event_ReactionAdded:
		return true
	default:
		return notificationVisibilityBoundaryEvent(event)
	}
}

func notificationVisibilityBoundaryEvent(event *corev1.Event) bool {
	if event == nil {
		return false
	}
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_RoomMemberBanned,
		*corev1.Event_RoomUniversalChanged,
		*corev1.Event_RoomAddedToGroup,
		*corev1.Event_RoomRemovedFromGroup,
		*corev1.Event_RoomGroupDeleted,
		*corev1.Event_RbacRoleAssigned,
		*corev1.Event_RbacRoleRevoked,
		*corev1.Event_RbacRoleDeleted:
		return true
	case *corev1.Event_RbacPermissionGranted:
		return payload.RbacPermissionGranted.GetPermission() == string(PermRoomJoin)
	case *corev1.Event_RbacPermissionDenied:
		return payload.RbacPermissionDenied.GetPermission() == string(PermRoomJoin)
	case *corev1.Event_RbacPermissionCleared:
		return payload.RbacPermissionCleared.GetPermission() == string(PermRoomJoin)
	default:
		return false
	}
}

func (*NotificationVisibilityProjection) SnapshotContractID() string {
	return notificationVisibilitySnapshotContractID
}

func (p *NotificationVisibilityProjection) Snapshot() ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return encodeNotificationVisibilityState(p.rooms, p.groups, p.rbac, p.config, p.activeUsers, p.threadFollows, p.replyCounts)
}

func (p *NotificationVisibilityProjection) Restore(data []byte) error {
	rooms, groups, rbac, config, activeUsers, threadFollows, followers, replyCounts, err := decodeNotificationVisibilityState(data)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.rooms, p.groups, p.rbac, p.config = rooms, groups, rbac, config
	p.activeUsers, p.threadFollows, p.followers, p.replyCounts = activeUsers, threadFollows, followers, replyCounts
	p.checkpointSequence = 0
	p.checkpoint = nil
	p.deltas = nil
	p.boundaries = make(map[uint64]struct{})
	p.evaluatorSequence = 0
	p.evaluator = nil
	p.mu.Unlock()
	return nil
}

func (p *NotificationVisibilityProjection) CompleteStartupReplay() {
	p.mu.Lock()
	p.rbac.CompleteStartupReplay()
	p.mu.Unlock()
}

func applyNotificationDecisionState(
	config *ConfigProjection,
	activeUsers map[string]struct{},
	threadFollows map[string]notificationThreadFollow,
	followers map[string]map[string]struct{},
	replyCounts map[string]uint64,
	event *corev1.Event,
	seq uint64,
) error {
	if event == nil {
		return nil
	}
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_UserNotificationPreferenceChanged:
		if err := config.Apply(event, seq); err != nil {
			return err
		}
	case *corev1.Event_UserAccountCreated:
		if userID := payload.UserAccountCreated.GetUserId(); userID != "" {
			activeUsers[userID] = struct{}{}
		}
	case *corev1.Event_UserAccountDeleted:
		if err := config.Apply(event, seq); err != nil {
			return err
		}
		delete(activeUsers, payload.UserAccountDeleted.GetUserId())
	case *corev1.Event_ThreadFollowed:
		follow := payload.ThreadFollowed
		setNotificationThreadFollow(threadFollows, followers, follow.GetUserId(), follow.GetRoomId(), follow.GetThreadRootEventId(), ThreadFollowStateFollowing)
	case *corev1.Event_ThreadUnfollowed:
		follow := payload.ThreadUnfollowed
		setNotificationThreadFollow(threadFollows, followers, follow.GetUserId(), follow.GetRoomId(), follow.GetThreadRootEventId(), ThreadFollowStateUnfollowed)
	case *corev1.Event_MessagePosted:
		if threadRootEventID := payload.MessagePosted.GetInThread(); threadRootEventID != "" {
			replyCounts[threadRootEventID]++
		}
	}
	return nil
}

func setNotificationThreadFollow(
	threadFollows map[string]notificationThreadFollow,
	followers map[string]map[string]struct{},
	userID, roomID, threadRootEventID string,
	state ThreadFollowState,
) {
	if userID == "" || roomID == "" || threadRootEventID == "" {
		return
	}
	threadKey := threadFollowKeyPart(roomID, threadRootEventID)
	key := userID + "\x00" + threadKey
	previous := threadFollows[key]
	if previous.state == ThreadFollowStateFollowing {
		delete(followers[threadKey], userID)
		if len(followers[threadKey]) == 0 {
			delete(followers, threadKey)
		}
	}
	threadFollows[key] = notificationThreadFollow{userID: userID, roomID: roomID, threadRootEventID: threadRootEventID, state: state}
	if state == ThreadFollowStateFollowing {
		if followers[threadKey] == nil {
			followers[threadKey] = make(map[string]struct{})
		}
		followers[threadKey][userID] = struct{}{}
	}
}

func encodeNotificationVisibilityState(
	rooms *RoomDirectoryProjection,
	groups *RoomGroupLayoutProjection,
	rbac *RBACProjection,
	config *ConfigProjection,
	activeUsers map[string]struct{},
	threadFollows map[string]notificationThreadFollow,
	replyCounts map[string]uint64,
) ([]byte, error) {
	roomData, err := rooms.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("snapshot room visibility: %w", err)
	}
	groupData, err := groups.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("snapshot room-group visibility: %w", err)
	}
	rbacData, err := rbac.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("snapshot RBAC visibility: %w", err)
	}
	configData, err := config.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("snapshot notification policy: %w", err)
	}
	snapshot := &corev1.NotificationVisibilityProjectionSnapshot{
		RoomDirectory:   &corev1.RoomDirectoryProjectionSnapshot{},
		RoomGroupLayout: &corev1.RoomGroupLayoutProjectionSnapshot{},
		Rbac:            &corev1.RBACProjectionSnapshot{},
		Config:          &corev1.ConfigProjectionSnapshot{},
	}
	if err := proto.Unmarshal(roomData, snapshot.RoomDirectory); err != nil {
		return nil, fmt.Errorf("decode room visibility snapshot: %w", err)
	}
	if err := proto.Unmarshal(groupData, snapshot.RoomGroupLayout); err != nil {
		return nil, fmt.Errorf("decode room-group visibility snapshot: %w", err)
	}
	if err := proto.Unmarshal(rbacData, snapshot.Rbac); err != nil {
		return nil, fmt.Errorf("decode RBAC visibility snapshot: %w", err)
	}
	if err := proto.Unmarshal(configData, snapshot.Config); err != nil {
		return nil, fmt.Errorf("decode notification policy snapshot: %w", err)
	}
	for userID := range activeUsers {
		snapshot.ActiveUserIds = append(snapshot.ActiveUserIds, userID)
	}
	sort.Strings(snapshot.ActiveUserIds)
	for _, key := range sortedMapKeys(threadFollows) {
		follow := threadFollows[key]
		snapshot.ThreadFollows = append(snapshot.ThreadFollows, &corev1.ThreadFollowSnapshot{
			UserId: follow.userID, RoomId: follow.roomID, ThreadRootEventId: follow.threadRootEventID, State: string(follow.state),
		})
	}
	for _, threadRootEventID := range sortedMapKeys(replyCounts) {
		snapshot.Threads = append(snapshot.Threads, &corev1.NotificationThreadStateSnapshot{
			ThreadRootEventId: threadRootEventID, ReplyCount: replyCounts[threadRootEventID],
		})
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func decodeNotificationVisibilityState(data []byte) (*RoomDirectoryProjection, *RoomGroupLayoutProjection, *RBACProjection, *ConfigProjection, map[string]struct{}, map[string]notificationThreadFollow, map[string]map[string]struct{}, map[string]uint64, error) {
	snapshot := &corev1.NotificationVisibilityProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("unmarshal notification visibility snapshot: %w", err)
		}
	}
	rooms := NewRoomDirectoryProjection()
	groups := NewRoomGroupLayoutProjection()
	rbac := NewRBACProjection()
	config := NewConfigProjection()
	marshalRestore := func(value proto.Message, restore func([]byte) error) error {
		payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(value)
		if err != nil {
			return err
		}
		return restore(payload)
	}
	if err := marshalRestore(snapshot.GetRoomDirectory(), rooms.Restore); err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("restore room visibility: %w", err)
	}
	if err := marshalRestore(snapshot.GetRoomGroupLayout(), groups.Restore); err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("restore room-group visibility: %w", err)
	}
	if err := marshalRestore(snapshot.GetRbac(), rbac.Restore); err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("restore RBAC visibility: %w", err)
	}
	if err := marshalRestore(snapshot.GetConfig(), config.Restore); err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("restore notification policy: %w", err)
	}
	activeUsers := make(map[string]struct{}, len(snapshot.GetActiveUserIds()))
	for _, userID := range snapshot.GetActiveUserIds() {
		if userID == "" {
			return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("notification decision snapshot has empty active user ID")
		}
		activeUsers[userID] = struct{}{}
	}
	threadFollows := make(map[string]notificationThreadFollow, len(snapshot.GetThreadFollows()))
	followers := make(map[string]map[string]struct{})
	for _, row := range snapshot.GetThreadFollows() {
		state := ThreadFollowState(row.GetState())
		if row.GetUserId() == "" || row.GetRoomId() == "" || row.GetThreadRootEventId() == "" || (state != ThreadFollowStateFollowing && state != ThreadFollowStateUnfollowed) {
			return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("notification decision snapshot has invalid thread follow")
		}
		setNotificationThreadFollow(threadFollows, followers, row.GetUserId(), row.GetRoomId(), row.GetThreadRootEventId(), state)
	}
	replyCounts := make(map[string]uint64, len(snapshot.GetThreads()))
	for _, row := range snapshot.GetThreads() {
		if row.GetThreadRootEventId() == "" {
			return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("notification decision snapshot has empty thread root event ID")
		}
		replyCounts[row.GetThreadRootEventId()] = row.GetReplyCount()
	}
	return rooms, groups, rbac, config, activeUsers, threadFollows, followers, replyCounts, nil
}

// SetAcknowledgedThrough seeds the notification consumer's confirmed floor
// before snapshot restore. Pending deliveries are replayed instead of being
// hidden behind a newer projection snapshot; ReleaseThrough advances the same
// floor after startup so unsafe generations cannot be published either.
func (p *NotificationVisibilityProjection) SetAcknowledgedThrough(sequence uint64) {
	p.acknowledgedThrough.Store(sequence)
}

func (p *NotificationVisibilityProjection) RestoreMaxCutoff() uint64 {
	return p.acknowledgedThrough.Load()
}

// AllowSnapshotPublication uses the same full durable-consumer floor as
// snapshot restore. Any filtered delivery—not only an implicit visibility
// boundary—can hold that floor behind the projector's current state.
func (p *NotificationVisibilityProjection) AllowSnapshotPublication(cutoff uint64) bool {
	return cutoff <= p.acknowledgedThrough.Load()
}

func (p *NotificationVisibilityProjection) Boundary(sequence uint64, at time.Time) (*notificationVisibilitySnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, retained := p.boundaries[sequence]
	if !retained || len(p.checkpoint) == 0 || sequence < p.checkpointSequence {
		return nil, fmt.Errorf("notification visibility boundary %d is unavailable", sequence)
	}
	if p.evaluator == nil || sequence < p.evaluatorSequence {
		rooms, groups, rbac, config, activeUsers, threadFollows, followers, replyCounts, err := decodeNotificationVisibilityState(p.checkpoint)
		if err != nil {
			return nil, fmt.Errorf("restore notification visibility boundary %d: %w", sequence, err)
		}
		p.evaluator = &notificationVisibilitySnapshot{
			rooms: rooms, groups: groups, rbac: rbac, config: config,
			activeUsers: activeUsers, threadFollows: threadFollows, followers: followers, replyCounts: replyCounts,
		}
		p.evaluatorSequence = p.checkpointSequence
	}
	start := 0
	for start < len(p.deltas) && p.deltas[start].sequence <= p.evaluatorSequence {
		start++
	}
	end := start
	for end < len(p.deltas) && p.deltas[end].sequence <= sequence {
		end++
	}
	if err := applyNotificationVisibilityDeltas(p.evaluator, p.deltas[start:end]); err != nil {
		return nil, fmt.Errorf("replay notification visibility boundary %d: %w", sequence, err)
	}
	p.evaluatorSequence = sequence
	p.evaluator.at = at
	return p.evaluator, nil
}

func applyNotificationVisibilityDeltas(snapshot *notificationVisibilitySnapshot, deltas []notificationVisibilityDelta) error {
	for _, delta := range deltas {
		if err := snapshot.rooms.Apply(delta.event, delta.sequence); err != nil {
			return err
		}
		if err := snapshot.groups.Apply(delta.event, delta.sequence); err != nil {
			return err
		}
		if err := snapshot.rbac.Apply(delta.event, delta.sequence); err != nil {
			return err
		}
		if err := applyNotificationDecisionState(snapshot.config, snapshot.activeUsers, snapshot.threadFollows, snapshot.followers, snapshot.replyCounts, delta.event, delta.sequence); err != nil {
			return err
		}
	}
	return nil
}

// ReleaseThrough drops compact boundary state only through facts whose durable
// acknowledgement has been confirmed by the shared consumer. The journal is
// released as one run when its final pending boundary is acknowledged; keeping
// the single checkpoint avoids re-serializing full state per acknowledgement.
func (p *NotificationVisibilityProjection) ReleaseThrough(sequence uint64) error {
	for current := p.acknowledgedThrough.Load(); sequence > current; current = p.acknowledgedThrough.Load() {
		if p.acknowledgedThrough.CompareAndSwap(current, sequence) {
			break
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.checkpoint) == 0 || sequence < p.checkpointSequence {
		return nil
	}
	for boundary := range p.boundaries {
		if boundary <= sequence {
			delete(p.boundaries, boundary)
		}
	}
	if len(p.boundaries) == 0 {
		p.checkpointSequence = 0
		p.checkpoint = nil
		p.deltas = nil
		p.evaluatorSequence = 0
		p.evaluator = nil
		return nil
	}
	return nil
}

func (p *NotificationVisibilityProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	roomEntries, roomBytes, roomMetrics := p.rooms.adminProjectionEstimate()
	groupEntries, groupBytes, groupMetrics := p.groups.adminProjectionEstimate()
	rbacEntries, rbacBytes, rbacMetrics := p.rbac.adminProjectionEstimate()
	metrics := append(roomMetrics, groupMetrics...)
	metrics = append(metrics, rbacMetrics...)
	var policyEntries int64
	p.config.RLock()
	for _, user := range p.config.users {
		policyEntries += int64(len(user.serverIntensityByKind))
		for _, room := range user.roomIntensityByRoomAndKind {
			policyEntries += int64(len(room))
		}
	}
	p.config.RUnlock()
	decisionEntries := int64(len(p.activeUsers)+len(p.threadFollows)+len(p.replyCounts)) + policyEntries
	var deltaBytes int64
	for _, delta := range p.deltas {
		deltaBytes += int64(proto.Size(delta.event))
	}
	metrics = append(metrics,
		ProjectionAdminMetric{Name: "decision_state", Value: decisionEntries, Bytes: decisionEntries * projectionMapEntryOverhead},
		ProjectionAdminMetric{Name: "pending_decision_boundaries", Value: int64(len(p.boundaries))},
		ProjectionAdminMetric{Name: "decision_boundary_deltas", Value: int64(len(p.deltas)), Bytes: deltaBytes},
	)
	return roomEntries + groupEntries + rbacEntries + decisionEntries + int64(len(p.boundaries)+len(p.deltas)), roomBytes + groupBytes + rbacBytes + decisionEntries*projectionMapEntryOverhead + int64(len(p.checkpoint)) + deltaBytes, metrics
}

// cappedNotificationVisibilitySnapshotSource prevents projection restore from
// advancing beyond the shared worker's acknowledged floor.
type cappedNotificationVisibilitySnapshotSource struct {
	source     events.ProjectionSnapshotSource
	projection *NotificationVisibilityProjection
}

func (s cappedNotificationVisibilitySnapshotSource) LoadProjectionSnapshot(ctx context.Context, request events.ProjectionSnapshotLoadRequest) (events.ProjectionSnapshot, error) {
	if cutoff := s.projection.RestoreMaxCutoff(); cutoff < request.MaxCutoff {
		request.MaxCutoff = cutoff
	}
	return s.source.LoadProjectionSnapshot(ctx, request)
}

type notificationVisibilitySnapshot struct {
	rooms         *RoomDirectoryProjection
	groups        *RoomGroupLayoutProjection
	rbac          *RBACProjection
	config        *ConfigProjection
	activeUsers   map[string]struct{}
	threadFollows map[string]notificationThreadFollow
	followers     map[string]map[string]struct{}
	replyCounts   map[string]uint64
	at            time.Time
}

func (s *notificationVisibilitySnapshot) roomKind(roomID string) (RoomKind, bool) {
	room, exists := s.rooms.Catalog.Get(roomID)
	if !exists {
		return KindChannel, false
	}
	return KindOfRoom(room), true
}

func (s *notificationVisibilitySnapshot) roomMemberIDs(roomID string) []string {
	seen := make(map[string]struct{})
	for _, userID := range s.rooms.Membership.Members(roomID) {
		seen[userID] = struct{}{}
	}
	room, exists := s.rooms.Catalog.Get(roomID)
	if exists && room.GetKind() == corev1.RoomKind_ROOM_KIND_CHANNEL && room.GetUniversal() {
		for userID := range s.activeUsers {
			if s.membershipExists(userID, roomID) {
				seen[userID] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for userID := range seen {
		result = append(result, userID)
	}
	sort.Strings(result)
	return result
}

func (s *notificationVisibilitySnapshot) threadFollowerIDs(roomID, threadRootEventID string) []string {
	users := s.followers[threadFollowKeyPart(roomID, threadRootEventID)]
	result := make([]string, 0, len(users))
	for userID := range users {
		result = append(result, userID)
	}
	sort.Strings(result)
	return result
}

func (s *notificationVisibilitySnapshot) threadFollowState(userID, roomID, threadRootEventID string) ThreadFollowState {
	return s.threadFollows[userID+"\x00"+threadFollowKeyPart(roomID, threadRootEventID)].state
}

func (s *notificationVisibilitySnapshot) effectiveNotificationIntensity(userID, roomID string, kind corev1.NotificationPolicyKind) corev1.NotificationDeliveryIntensity {
	s.config.RLock()
	defer s.config.RUnlock()
	if user := s.config.users[userID]; user != nil {
		if intensity := user.roomIntensityByRoomAndKind[roomID][kind]; intensity != corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED {
			return intensity
		}
		if intensity := user.serverIntensityByKind[kind]; intensity != corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED {
			return intensity
		}
	}
	return defaultNotificationIntensity(kind)
}

func (s *notificationVisibilitySnapshot) membershipExists(userID, roomID string) bool {
	if s.rooms.Membership.IsMember(roomID, userID) {
		return true
	}
	room, exists := s.rooms.Catalog.Get(roomID)
	if !exists || room.GetKind() != corev1.RoomKind_ROOM_KIND_CHANNEL || !room.GetUniversal() {
		return false
	}
	if s.rooms.Bans.IsActive(roomID, userID, s.at) {
		return false
	}
	return s.roomJoinAllowed(userID, roomID, s.groups.Groups.GroupForRoom(roomID))
}

func (s *notificationVisibilitySnapshot) roomJoinAllowed(userID, roomID, groupID string) bool {
	if s.rbac.HasRole(userID, RoleOwner) {
		return true
	}
	scopes := []permissionScopeTarget{{scope: ScopeRoom, level: LevelRoom, id: roomID}}
	if groupID != "" {
		scopes = append(scopes, permissionScopeTarget{scope: ScopeGroup, level: LevelGroup, id: groupID})
	}
	scopes = append(scopes, permissionScopeTarget{scope: ScopeServer, level: LevelServer})

	nearest := func(subject string) (TraceEntry, bool) {
		for _, target := range scopes {
			decision := s.rbac.GetDecision(target.scope, target.id, subject, PermRoomJoin)
			if decision != DecisionNone {
				return TraceEntry{Level: target.level, RoleName: subject, Decision: decision, ObjectID: target.objectID()}, true
			}
		}
		return TraceEntry{}, false
	}

	var decisions applicablePermissionDecisions
	for _, subject := range append([]string{userID}, s.rbac.GetUserRoles(userID)...) {
		if entry, ok := nearest(subject); ok {
			decisions.named = append(decisions.named, entry)
		}
	}
	if entry, ok := nearest(RoleEveryone); ok {
		decisions.everyone = &entry
	}
	decision, _, _ := resolveApplicablePermissionDecisions(decisions)
	return decision == DecisionAllow
}
