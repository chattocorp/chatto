package core

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

var notificationDecisionSnapshotContractID = snapshotContractID("v1", &corev1.NotificationDecisionProjectionSnapshot{})

// NotificationDecisionProjection keeps the compact event-time state needed
// to derive notification recipients and policy while enforcing persistent
// privacy boundaries.
type NotificationDecisionProjection struct {
	mu sync.RWMutex

	rooms  *RoomDirectoryProjection
	groups *RoomGroupLayoutProjection
	rbac   *RBACProjection
	config *ConfigProjection

	activeUsers   map[string]struct{}
	threadFollows map[string]notificationThreadFollow
	followers     map[string]map[string]struct{}
	replyCounts   map[string]uint64

	// The main projection may run ahead of the single-lane durable worker. A
	// second in-memory evaluator follows the worker instead: Apply journals each
	// relevant event above the confirmed worker floor, and the worker advances
	// the evaluator through deliveries in order. This keeps ordinary boundary
	// work proportional to new EVT facts, never to total server state.
	deltas              []notificationDecisionDelta
	boundaries          map[uint64]struct{}
	evaluatorSequence   uint64
	evaluator           *notificationDecisionSnapshot
	acknowledgedThrough atomic.Uint64
}

type notificationDecisionDelta struct {
	sequence uint64
	event    *corev1.Event
}

type notificationThreadFollow struct {
	userID            string
	roomID            string
	threadRootEventID string
	state             ThreadFollowState
}

func NewNotificationDecisionProjection() *NotificationDecisionProjection {
	p := &NotificationDecisionProjection{
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
	p.evaluator = newNotificationDecisionSnapshot()
	return p
}

func (*NotificationDecisionProjection) Subjects() []string {
	return notificationDecisionProjectionSubjects()
}

// ReplaySubjects uses one physical EVT filter. This projection's logical
// contract spans several sparse aggregate families; one broad scan avoids the
// JetStream multi-filter cost while Projector still rejects unrelated subjects
// before decoding or applying them.
func (*NotificationDecisionProjection) ReplaySubjects() []string {
	return []string{evtstream.EventSubjectFilter()}
}

func notificationDecisionProjectionSubjects() []string {
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
		evtstream.RoomEventTypeFilter(evtstream.EventReactionRemoved),
		evtstream.RoomEventTypeFilter(evtstream.EventMessageRetracted),
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
		evtstream.UserEventTypeFilter(evtstream.EventUserVerifiedEmailAdded),
	}
}

func (p *NotificationDecisionProjection) Apply(event *corev1.Event, seq uint64) error {
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
	if seq <= p.acknowledgedThrough.Load() {
		// Idle consumer progress can advance the acknowledged floor across
		// state-only facts while this projector is catching up. Apply any older
		// journaled deltas first; advancing the new fact directly would otherwise
		// skip them and make the worker-position evaluator order-dependent.
		if seq > 0 && p.evaluatorSequence < seq-1 {
			if err := p.advanceEvaluatorThrough(seq - 1); err != nil {
				return fmt.Errorf("advance acknowledged notification decision history before %d: %w", seq, err)
			}
		}
		if seq <= p.evaluatorSequence {
			return fmt.Errorf("notification decision event %d does not advance evaluator at %d", seq, p.evaluatorSequence)
		}
		if err := applyNotificationDecisionDeltas(p.evaluator, []notificationDecisionDelta{{sequence: seq, event: event}}); err != nil {
			return fmt.Errorf("advance acknowledged notification decision state through %d: %w", seq, err)
		}
		p.evaluatorSequence = seq
		firstRetained := sort.Search(len(p.deltas), func(i int) bool { return p.deltas[i].sequence > seq })
		if firstRetained > 0 {
			p.deltas = append([]notificationDecisionDelta(nil), p.deltas[firstRetained:]...)
		}
		return nil
	}
	p.deltas = append(p.deltas, notificationDecisionDelta{
		sequence: seq,
		event:    proto.Clone(event).(*corev1.Event),
	})
	if notificationDecisionBoundaryEvent(event) {
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

func (*NotificationDecisionProjection) SnapshotContractID() string {
	return notificationDecisionSnapshotContractID
}

func (p *NotificationDecisionProjection) Snapshot() ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return encodeNotificationDecisionState(p.rooms, p.groups, p.rbac, p.config, p.activeUsers, p.threadFollows, p.replyCounts)
}

func (p *NotificationDecisionProjection) Restore(data []byte) error {
	rooms, groups, rbac, config, activeUsers, threadFollows, followers, replyCounts, err := decodeNotificationDecisionState(data)
	if err != nil {
		return err
	}
	evaluatorRooms, evaluatorGroups, evaluatorRBAC, evaluatorConfig, evaluatorActiveUsers, evaluatorThreadFollows, evaluatorFollowers, evaluatorReplyCounts, err := decodeNotificationDecisionState(data)
	if err != nil {
		return fmt.Errorf("restore notification decision evaluator: %w", err)
	}
	p.mu.Lock()
	p.rooms, p.groups, p.rbac, p.config = rooms, groups, rbac, config
	p.activeUsers, p.threadFollows, p.followers, p.replyCounts = activeUsers, threadFollows, followers, replyCounts
	p.deltas = nil
	p.boundaries = make(map[uint64]struct{})
	p.evaluatorSequence = 0
	p.evaluator = &notificationDecisionSnapshot{
		rooms: evaluatorRooms, groups: evaluatorGroups, rbac: evaluatorRBAC, config: evaluatorConfig,
		activeUsers: evaluatorActiveUsers, threadFollows: evaluatorThreadFollows, followers: evaluatorFollowers, replyCounts: evaluatorReplyCounts,
	}
	p.mu.Unlock()
	return nil
}

func (p *NotificationDecisionProjection) CompleteStartupReplay() {
	p.mu.Lock()
	p.rbac.CompleteStartupReplay()
	p.evaluator.rbac.CompleteStartupReplay()
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
	case *corev1.Event_UserNotificationPolicyChanged,
		*corev1.Event_UserRoomGroupNotificationPolicyChanged:
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

func encodeNotificationDecisionState(
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
	snapshot := &corev1.NotificationDecisionProjectionSnapshot{
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

func decodeNotificationDecisionState(data []byte) (*RoomDirectoryProjection, *RoomGroupLayoutProjection, *RBACProjection, *ConfigProjection, map[string]struct{}, map[string]notificationThreadFollow, map[string]map[string]struct{}, map[string]uint64, error) {
	snapshot := &corev1.NotificationDecisionProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("unmarshal notification decision snapshot: %w", err)
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
func (p *NotificationDecisionProjection) SetAcknowledgedThrough(sequence uint64) {
	p.acknowledgedThrough.Store(sequence)
}

func (p *NotificationDecisionProjection) RestoreMaxCutoff() uint64 {
	return p.acknowledgedThrough.Load()
}

// AllowSnapshotPublication uses the same full durable-consumer floor as
// snapshot restore. Any filtered delivery—not only an implicit visibility
// boundary—can hold that floor behind the projector's current state.
func (p *NotificationDecisionProjection) AllowSnapshotPublication(cutoff uint64) bool {
	return cutoff <= p.acknowledgedThrough.Load()
}

func (p *NotificationDecisionProjection) Boundary(sequence uint64, at time.Time) (*notificationDecisionSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, retained := p.boundaries[sequence]
	if !retained || p.evaluator == nil || sequence < p.evaluatorSequence {
		return nil, fmt.Errorf("notification decision boundary %d is unavailable", sequence)
	}
	if err := p.advanceEvaluatorThrough(sequence); err != nil {
		return nil, fmt.Errorf("advance notification decision boundary %d: %w", sequence, err)
	}
	p.evaluator.at = at
	return p.evaluator, nil
}

// AdvanceThrough advances the lagging evaluator after a worker delivery has
// completed. Boundary deliveries have already advanced it; this method also
// accounts for policy, membership, and other state-only deliveries so they do
// not accumulate while notification traffic is idle. It intentionally runs
// before DoubleAck: redelivery at the same sequence is safe and idempotent.
func (p *NotificationDecisionProjection) AdvanceThrough(sequence uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.advanceEvaluatorThrough(sequence)
}

func (p *NotificationDecisionProjection) advanceEvaluatorThrough(sequence uint64) error {
	if p.evaluator == nil {
		return fmt.Errorf("notification decision evaluator is unavailable")
	}
	if sequence < p.evaluatorSequence {
		return fmt.Errorf("notification decision evaluator is at %d, cannot move back to %d", p.evaluatorSequence, sequence)
	}
	start := sort.Search(len(p.deltas), func(i int) bool { return p.deltas[i].sequence > p.evaluatorSequence })
	end := sort.Search(len(p.deltas), func(i int) bool { return p.deltas[i].sequence > sequence })
	if err := applyNotificationDecisionDeltas(p.evaluator, p.deltas[start:end]); err != nil {
		return err
	}
	p.evaluatorSequence = sequence
	return nil
}

func applyNotificationDecisionDeltas(snapshot *notificationDecisionSnapshot, deltas []notificationDecisionDelta) error {
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

// ReleaseThrough drops event-time boundary state only through facts whose
// durable acknowledgement has been confirmed by the shared consumer. The
// evaluator has already advanced in the worker handler before DoubleAck, so
// cleanup never mutates a snapshot that an active delivery is reading.
func (p *NotificationDecisionProjection) ReleaseThrough(sequence uint64) error {
	for current := p.acknowledgedThrough.Load(); sequence > current; current = p.acknowledgedThrough.Load() {
		if p.acknowledgedThrough.CompareAndSwap(current, sequence) {
			break
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for boundary := range p.boundaries {
		if boundary <= sequence {
			delete(p.boundaries, boundary)
		}
	}
	releaseThrough := min(sequence, p.evaluatorSequence)
	firstRetained := sort.Search(len(p.deltas), func(i int) bool { return p.deltas[i].sequence > releaseThrough })
	if firstRetained > 0 {
		p.deltas = append([]notificationDecisionDelta(nil), p.deltas[firstRetained:]...)
	}
	return nil
}

func (p *NotificationDecisionProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	roomEntries, roomBytes, roomMetrics := p.rooms.adminProjectionEstimate()
	groupEntries, groupBytes, groupMetrics := p.groups.adminProjectionEstimate()
	rbacEntries, rbacBytes, rbacMetrics := p.rbac.adminProjectionEstimate()
	metrics := append(roomMetrics, groupMetrics...)
	metrics = append(metrics, rbacMetrics...)
	policyEntries := notificationPolicyEntryCount(p.config)
	decisionEntries := int64(len(p.activeUsers)+len(p.threadFollows)+len(p.replyCounts)) + policyEntries
	evaluatorRoomEntries, evaluatorRoomBytes, _ := p.evaluator.rooms.adminProjectionEstimate()
	evaluatorGroupEntries, evaluatorGroupBytes, _ := p.evaluator.groups.adminProjectionEstimate()
	evaluatorRBACEntries, evaluatorRBACBytes, _ := p.evaluator.rbac.adminProjectionEstimate()
	evaluatorDecisionEntries := int64(len(p.evaluator.activeUsers)+len(p.evaluator.threadFollows)+len(p.evaluator.replyCounts)) + notificationPolicyEntryCount(p.evaluator.config)
	evaluatorEntries := evaluatorRoomEntries + evaluatorGroupEntries + evaluatorRBACEntries + evaluatorDecisionEntries
	evaluatorBytes := evaluatorRoomBytes + evaluatorGroupBytes + evaluatorRBACBytes + evaluatorDecisionEntries*projectionMapEntryOverhead
	var deltaBytes int64
	for _, delta := range p.deltas {
		deltaBytes += int64(proto.Size(delta.event))
	}
	metrics = append(metrics,
		ProjectionAdminMetric{Name: "decision_state", Value: decisionEntries, Bytes: decisionEntries * projectionMapEntryOverhead},
		ProjectionAdminMetric{Name: "worker_position_decision_state", Value: evaluatorEntries, Bytes: evaluatorBytes},
		ProjectionAdminMetric{Name: "pending_decision_boundaries", Value: int64(len(p.boundaries))},
		ProjectionAdminMetric{Name: "decision_boundary_deltas", Value: int64(len(p.deltas)), Bytes: deltaBytes},
	)
	return roomEntries + groupEntries + rbacEntries + decisionEntries + evaluatorEntries + int64(len(p.boundaries)+len(p.deltas)), roomBytes + groupBytes + rbacBytes + decisionEntries*projectionMapEntryOverhead + evaluatorBytes + deltaBytes, metrics
}

func notificationPolicyEntryCount(config *ConfigProjection) int64 {
	var entries int64
	config.RLock()
	defer config.RUnlock()
	for _, user := range config.users {
		entries += notificationDeliveryModeFieldCount(user.serverModes)
		for _, group := range user.roomGroupModesByGroup {
			entries += notificationDeliveryModeFieldCount(group)
		}
		for _, room := range user.roomModesByRoom {
			entries += notificationDeliveryModeFieldCount(room)
		}
	}
	return entries
}

func notificationDeliveryModeFieldCount(modes *corev1.NotificationDeliveryModes) int64 {
	if modes == nil {
		return 0
	}
	var count int64
	modes.ProtoReflect().Range(func(protoreflect.FieldDescriptor, protoreflect.Value) bool {
		count++
		return true
	})
	return count
}

// cappedNotificationDecisionSnapshotSource prevents projection restore from
// advancing beyond the shared worker's acknowledged floor.
type cappedNotificationDecisionSnapshotSource struct {
	source     events.ProjectionSnapshotSource
	projection *NotificationDecisionProjection
}

func (s cappedNotificationDecisionSnapshotSource) LoadProjectionSnapshot(ctx context.Context, request events.ProjectionSnapshotLoadRequest) (events.ProjectionSnapshot, error) {
	if cutoff := s.projection.RestoreMaxCutoff(); cutoff < request.MaxCutoff {
		request.MaxCutoff = cutoff
	}
	return s.source.LoadProjectionSnapshot(ctx, request)
}

type notificationDecisionSnapshot struct {
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

func newNotificationDecisionSnapshot() *notificationDecisionSnapshot {
	return &notificationDecisionSnapshot{
		rooms:         NewRoomDirectoryProjection(),
		groups:        NewRoomGroupLayoutProjection(),
		rbac:          NewRBACProjection(),
		config:        NewConfigProjection(),
		activeUsers:   make(map[string]struct{}),
		threadFollows: make(map[string]notificationThreadFollow),
		followers:     make(map[string]map[string]struct{}),
		replyCounts:   make(map[string]uint64),
	}
}

func (s *notificationDecisionSnapshot) roomKind(roomID string) (RoomKind, bool) {
	room, exists := s.rooms.Catalog.Get(roomID)
	if !exists {
		return KindChannel, false
	}
	return KindOfRoom(room), true
}

func (s *notificationDecisionSnapshot) roomMemberIDs(roomID string) []string {
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

func (s *notificationDecisionSnapshot) threadFollowerIDs(roomID, threadRootEventID string) []string {
	users := s.followers[threadFollowKeyPart(roomID, threadRootEventID)]
	result := make([]string, 0, len(users))
	for userID := range users {
		result = append(result, userID)
	}
	sort.Strings(result)
	return result
}

func (s *notificationDecisionSnapshot) threadFollowState(userID, roomID, threadRootEventID string) ThreadFollowState {
	return s.threadFollows[userID+"\x00"+threadFollowKeyPart(roomID, threadRootEventID)].state
}

func (s *notificationDecisionSnapshot) effectiveNotificationMode(userID, roomID string, signal *corev1.NotificationSignal) corev1.NotificationDeliveryMode {
	s.config.RLock()
	defer s.config.RUnlock()
	if user := s.config.users[userID]; user != nil {
		if mode := notificationModeForSignal(user.roomModesByRoom[roomID], signal); mode != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
			return mode
		}
		if groupID := s.groups.Groups.GroupForRoom(roomID); groupID != "" {
			if mode := notificationModeForSignal(user.roomGroupModesByGroup[groupID], signal); mode != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
				return mode
			}
		}
		if mode := notificationModeForSignal(user.serverModes, signal); mode != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
			return mode
		}
	}
	return notificationModeForSignal(effectiveNotificationDeliveryModes(nil, nil), signal)
}

func (s *notificationDecisionSnapshot) membershipExists(userID, roomID string) bool {
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

func (s *notificationDecisionSnapshot) roomJoinAllowed(userID, roomID, groupID string) bool {
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
