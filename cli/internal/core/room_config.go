package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"hmans.de/chatto/internal/evtstream"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	roomConfigPathAuthorEditWindow = "author_edit_window"

	// DefaultAuthorEditWindow is the product default used when no room
	// configuration layer supplies a value.
	DefaultAuthorEditWindow = 3 * time.Hour
	// MessageEditWindow is retained as the compatibility name for the product
	// default. Enforcement resolves the current room configuration instead.
	MessageEditWindow = DefaultAuthorEditWindow
	// MaxAuthorEditWindow bounds operator-controlled edit windows.
	MaxAuthorEditWindow = 30 * 24 * time.Hour
)

type RoomConfigScopeKind int

const (
	RoomConfigScopeServer RoomConfigScopeKind = iota + 1
	RoomConfigScopeRoomGroup
	RoomConfigScopeRoom
)

// RoomConfigScope identifies one room-configuration inheritance tier.
type RoomConfigScope struct {
	Kind RoomConfigScopeKind
	ID   string
}

// RoomConfig contains typed room settings. Fields are sparse in a stored layer
// and fully populated in a resolved configuration.
type RoomConfig struct {
	AuthorEditWindow *time.Duration
}

// RoomConfigState combines stored and resolved state for administration.
type RoomConfigState struct {
	Layer     RoomConfig
	Effective RoomConfig
}

func roomConfigScopeKeyFor(scope RoomConfigScope) roomConfigScopeKey {
	return roomConfigScopeKey{kind: scope.Kind, id: scope.ID}
}

func roomConfigProto(config RoomConfig) *apiv1.RoomConfig {
	result := &apiv1.RoomConfig{}
	if config.AuthorEditWindow != nil {
		result.AuthorEditWindow = durationpb.New(*config.AuthorEditWindow)
	}
	return result
}

func roomConfigFromProto(config *apiv1.RoomConfig) RoomConfig {
	result := RoomConfig{}
	if config != nil && config.AuthorEditWindow != nil {
		value := config.GetAuthorEditWindow().AsDuration()
		result.AuthorEditWindow = &value
	}
	return result
}

func roomConfigUpdateProto(scope RoomConfigScope, config RoomConfig, paths ...string) *corev1.RoomConfigUpdatedEvent {
	updated := &corev1.RoomConfigUpdatedEvent{
		Config:     roomConfigProto(config),
		UpdateMask: &fieldmaskpb.FieldMask{Paths: paths},
	}
	switch scope.Kind {
	case RoomConfigScopeServer:
		updated.Scope = &corev1.RoomConfigUpdatedEvent_Server{Server: true}
	case RoomConfigScopeRoomGroup:
		updated.Scope = &corev1.RoomConfigUpdatedEvent_RoomGroupId{RoomGroupId: scope.ID}
	case RoomConfigScopeRoom:
		updated.Scope = &corev1.RoomConfigUpdatedEvent_RoomId{RoomId: scope.ID}
	}
	return updated
}

func roomConfigUpdatedEvent(actorID string, scope RoomConfigScope, config RoomConfig, paths ...string) *corev1.Event {
	updated := roomConfigUpdateProto(scope, config, paths...)
	return newEvent(actorID, &corev1.Event{Event: &corev1.Event_RoomConfigUpdated{RoomConfigUpdated: updated}})
}

// RoomConfigScopeFromUpdate returns the typed scope carried by a persisted
// room-configuration update.
func RoomConfigScopeFromUpdate(updated *corev1.RoomConfigUpdatedEvent) (RoomConfigScope, bool) {
	if updated == nil {
		return RoomConfigScope{}, false
	}
	switch value := updated.GetScope().(type) {
	case *corev1.RoomConfigUpdatedEvent_Server:
		return RoomConfigScope{Kind: RoomConfigScopeServer}, value.Server
	case *corev1.RoomConfigUpdatedEvent_RoomGroupId:
		return RoomConfigScope{Kind: RoomConfigScopeRoomGroup, ID: value.RoomGroupId}, value.RoomGroupId != ""
	case *corev1.RoomConfigUpdatedEvent_RoomId:
		return RoomConfigScope{Kind: RoomConfigScopeRoom, ID: value.RoomId}, value.RoomId != ""
	default:
		return RoomConfigScope{}, false
	}
}

func (c *ChattoCore) validateRoomConfigScope(ctx context.Context, scope RoomConfigScope) error {
	switch scope.Kind {
	case RoomConfigScopeServer:
		if scope.ID != "" {
			return invalidArgument("server room-configuration scope must not have an ID")
		}
	case RoomConfigScopeRoomGroup:
		if scope.ID == "" {
			return invalidArgument("room-group room-configuration scope requires an ID")
		}
		if _, err := c.GetRoomGroup(ctx, scope.ID); err != nil {
			return err
		}
	case RoomConfigScopeRoom:
		if scope.ID == "" {
			return invalidArgument("room configuration scope requires an ID")
		}
		room, err := c.FindRoomByID(ctx, scope.ID)
		if err != nil {
			return err
		}
		if KindOfRoom(room) != KindChannel {
			return invalidArgument("direct messages do not support room configuration layers")
		}
	default:
		return invalidArgument("room configuration scope is required")
	}
	return nil
}

func (c *ChattoCore) authorizeRoomConfigScope(ctx context.Context, actorID string, scope RoomConfigScope) error {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return err
	}
	switch scope.Kind {
	case RoomConfigScopeServer:
		ok, err := c.CanManageServer(ctx, actorID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrPermissionDenied
		}
	case RoomConfigScopeRoomGroup:
		return c.requireCanManageRoomGroup(ctx, actorID, scope.ID)
	case RoomConfigScopeRoom:
		ok, err := c.PermResolver().HasRoomPermission(ctx, actorID, KindChannel, scope.ID, PermRoomManage)
		if err != nil {
			return err
		}
		if !ok {
			return ErrPermissionDenied
		}
	default:
		return invalidArgument("room configuration scope is required")
	}
	return nil
}

func (c *ChattoCore) roomConfigLayer(scope RoomConfigScope) RoomConfig {
	state := c.configModel.config.Projection().roomConfigLayer(roomConfigScopeKeyFor(scope))
	return RoomConfig{AuthorEditWindow: state.authorEditWindow}
}

func resolveAuthorEditWindow(candidates ...*time.Duration) time.Duration {
	for _, candidate := range candidates {
		if candidate != nil {
			return *candidate
		}
	}
	return DefaultAuthorEditWindow
}

func (c *ChattoCore) effectiveRoomConfigForScope(scope RoomConfigScope) RoomConfig {
	server := RoomConfigScope{Kind: RoomConfigScopeServer}
	candidates := make([]*time.Duration, 0, 2)
	if scope.Kind != RoomConfigScopeServer {
		candidates = append(candidates, c.roomConfigLayer(scope).AuthorEditWindow)
	}
	candidates = append(candidates, c.roomConfigLayer(server).AuthorEditWindow)
	value := resolveAuthorEditWindow(candidates...)
	return RoomConfig{AuthorEditWindow: &value}
}

// EffectiveServerRoomConfig resolves the server baseline and product defaults.
func (c *ChattoCore) EffectiveServerRoomConfig() RoomConfig {
	return c.effectiveRoomConfigForScope(RoomConfigScope{Kind: RoomConfigScopeServer})
}

// EffectiveRoomConfig resolves room, current group, server, and product
// defaults. Direct messages intentionally skip room and group tiers.
func (c *ChattoCore) EffectiveRoomConfig(room *corev1.Room) RoomConfig {
	server := RoomConfigScope{Kind: RoomConfigScopeServer}
	candidates := make([]*time.Duration, 0, 3)
	if room != nil && KindOfRoom(room) == KindChannel {
		roomScope := RoomConfigScope{Kind: RoomConfigScopeRoom, ID: room.GetId()}
		candidates = append(candidates, c.roomConfigLayer(roomScope).AuthorEditWindow)
		if groupID := c.roomModel.roomGroupForRoom(room.GetId()); groupID != "" {
			groupScope := RoomConfigScope{Kind: RoomConfigScopeRoomGroup, ID: groupID}
			candidates = append(candidates, c.roomConfigLayer(groupScope).AuthorEditWindow)
		}
	}
	candidates = append(candidates, c.roomConfigLayer(server).AuthorEditWindow)
	value := resolveAuthorEditWindow(candidates...)
	return RoomConfig{AuthorEditWindow: &value}
}

func (c *ChattoCore) roomConfigState(ctx context.Context, scope RoomConfigScope) RoomConfigState {
	effective := c.effectiveRoomConfigForScope(scope)
	if scope.Kind == RoomConfigScopeRoom {
		if room, err := c.FindRoomByID(ctx, scope.ID); err == nil {
			effective = c.EffectiveRoomConfig(room)
		}
	}
	return RoomConfigState{Layer: c.roomConfigLayer(scope), Effective: effective}
}

// GetRoomConfig returns the stored layer and effective room configuration at a scope.
func (c *ChattoCore) GetRoomConfig(ctx context.Context, actorID string, scope RoomConfigScope) (RoomConfigState, error) {
	if err := c.validateRoomConfigScope(ctx, scope); err != nil {
		return RoomConfigState{}, err
	}
	if err := c.authorizeRoomConfigScope(ctx, actorID, scope); err != nil {
		return RoomConfigState{}, err
	}
	return c.roomConfigState(ctx, scope), nil
}

func validateRoomConfigUpdate(config RoomConfig, mask *fieldmaskpb.FieldMask) error {
	if mask == nil || len(mask.GetPaths()) == 0 {
		return invalidArgument("at least one room configuration field must be selected")
	}
	if !mask.IsValid(&apiv1.RoomConfig{}) {
		return invalidArgument("room configuration update mask contains an unknown field")
	}
	for _, path := range mask.GetPaths() {
		switch path {
		case roomConfigPathAuthorEditWindow:
			if config.AuthorEditWindow != nil && (*config.AuthorEditWindow < 0 || *config.AuthorEditWindow > MaxAuthorEditWindow) {
				return invalidArgument(fmt.Sprintf("author edit window must be between 0 and %s", MaxAuthorEditWindow))
			}
		}
	}
	return nil
}

func allRoomConfigPaths() []string {
	fields := (&apiv1.RoomConfig{}).ProtoReflect().Descriptor().Fields()
	paths := make([]string, 0, fields.Len())
	for index := 0; index < fields.Len(); index++ {
		paths = append(paths, string(fields.Get(index).Name()))
	}
	return paths
}

// RoomConfigUpdateAffectsPublicClients reports whether a persisted room
// configuration patch changes a field exposed to ordinary clients. Keep this
// explicit allow-list aligned with the ordinary-client room view. The
// canonical RoomConfig may also contain administrative or private fields;
// their values must remain omitted from public views and their changes must
// not produce public realtime traffic.
func RoomConfigUpdateAffectsPublicClients(updated *corev1.RoomConfigUpdatedEvent) bool {
	for _, path := range updated.GetUpdateMask().GetPaths() {
		switch path {
		case roomConfigPathAuthorEditWindow:
			return true
		}
	}
	return false
}

func roomConfigMatchesUpdate(current, requested RoomConfig, mask *fieldmaskpb.FieldMask) bool {
	for _, path := range mask.GetPaths() {
		switch path {
		case roomConfigPathAuthorEditWindow:
			if !equalOptionalDuration(current.AuthorEditWindow, requested.AuthorEditWindow) {
				return false
			}
		}
	}
	return true
}

// UpdateRoomConfig changes selected values in one room-configuration layer.
func (c *ChattoCore) UpdateRoomConfig(ctx context.Context, actorID string, scope RoomConfigScope, config RoomConfig, mask *fieldmaskpb.FieldMask) (RoomConfigState, error) {
	if err := validateRoomConfigUpdate(config, mask); err != nil {
		return RoomConfigState{}, err
	}
	subject := ConfigSubjectServer
	if scope.Kind != RoomConfigScopeServer {
		subject = scope.ID
	}
	for attempt := 0; attempt < maxConfigUpdateRetries; attempt++ {
		// Scope deletion advances this same config aggregate. Revalidate on
		// every OCC attempt against the current authoritative lifecycle tail so
		// a delete that wins the race prevents a later configuration fact from
		// recreating unreachable projected state on a lagging replica.
		if err := c.waitForRoomConfigScopeCurrent(ctx, scope); err != nil {
			return RoomConfigState{}, err
		}
		if err := c.validateRoomConfigScope(ctx, scope); err != nil {
			return RoomConfigState{}, err
		}
		authorizationSeq, err := c.authorizationFenceSeq(ctx)
		if err != nil {
			return RoomConfigState{}, err
		}
		agg, filter, expectedSeq, err := c.configModel.prepareAggregate(ctx, evtstream.RoomConfigAggregate(subject))
		if err != nil {
			return RoomConfigState{}, err
		}
		if err := c.waitForRoomConfigAuthorization(ctx, actorID); err != nil {
			return RoomConfigState{}, err
		}
		if err := c.authorizeRoomConfigScope(ctx, actorID, scope); err != nil {
			return RoomConfigState{}, err
		}
		current := c.roomConfigLayer(scope)
		if roomConfigMatchesUpdate(current, config, mask) {
			return c.GetRoomConfig(ctx, actorID, scope)
		}
		event := roomConfigUpdatedEvent(actorID, scope, config, mask.GetPaths()...)
		entries := []evtstream.BatchEntry{{Subject: agg.SubjectFor(event), Event: event, ExpectedSeq: expectedSeq, FilterSubject: filter, HasOCC: true}}
		seqs, err := c.appendAuthorizationFencedBatch(ctx, actorID, entries, authorizationSeq)
		if err == nil {
			if err := c.configModel.waitFor(ctx, events.SubjectPosition(entries[0].Subject, seqs[0])); err != nil {
				return RoomConfigState{}, err
			}
			if c.afterRoomConfigCommit != nil {
				c.afterRoomConfigCommit()
			}
			// Authorization was tied to the committed fence above. A later
			// revocation must not turn this successful durable mutation into an
			// apparent failure response.
			return c.roomConfigState(ctx, scope), nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return RoomConfigState{}, err
		}
		select {
		case <-ctx.Done():
			return RoomConfigState{}, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return RoomConfigState{}, ErrConfigConflict
}

func (c *ChattoCore) waitForRoomConfigScopeCurrent(ctx context.Context, scope RoomConfigScope) error {
	var (
		position events.StreamPosition
		err      error
	)
	switch scope.Kind {
	case RoomConfigScopeRoom:
		position, err = c.EventPublisher.LastSubjectPosition(ctx, evtstream.RoomAggregate(scope.ID).AllEventsFilter())
		if err == nil && !position.IsZero() {
			err = c.roomModel.waitForDirectory(ctx, position)
		}
	case RoomConfigScopeRoomGroup:
		position, err = c.EventPublisher.LastSubjectPosition(ctx, evtstream.GroupAggregate(scope.ID).AllEventsFilter())
		if err == nil && !position.IsZero() {
			err = c.roomModel.waitForGroupLayout(ctx, position)
		}
	case RoomConfigScopeServer:
		return nil
	default:
		return invalidArgument("room configuration scope is required")
	}
	if err != nil {
		return fmt.Errorf("wait for room configuration scope lifecycle: %w", err)
	}
	return nil
}

func (c *ChattoCore) waitForRoomConfigAuthorization(ctx context.Context, actorID string) error {
	groupPosition, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.GroupSubjectFilter())
	if err != nil {
		return fmt.Errorf("read room-group authorization tail: %w", err)
	}
	rbacPosition, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.RBACSubjectFilter())
	if err != nil {
		return fmt.Errorf("read RBAC authorization tail: %w", err)
	}
	userPosition, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.UserAggregate(actorID).AllEventsFilter())
	if err != nil {
		return fmt.Errorf("read actor authorization tail: %w", err)
	}
	if err := c.roomModel.waitForGroupLayout(ctx, groupPosition); err != nil {
		return fmt.Errorf("wait for room-group authorization projection: %w", err)
	}
	if err := c.rbacModel.waitFor(ctx, rbacPosition); err != nil {
		return fmt.Errorf("wait for RBAC authorization projection: %w", err)
	}
	if err := c.userModel.waitForUsers(ctx, userPosition); err != nil {
		return fmt.Errorf("wait for actor authorization projection: %w", err)
	}
	return nil
}

func equalOptionalDuration(a, b *time.Duration) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
