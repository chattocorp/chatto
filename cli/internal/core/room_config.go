package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
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

// RoomConfigLayer contains sparse values contributed by one scope.
type RoomConfigLayer struct {
	AuthorEditWindow *time.Duration
}

// RoomConfigUpdateMask selects which layer fields an update may change.
type RoomConfigUpdateMask struct {
	AuthorEditWindow bool
}

// RoomConfig contains fully resolved behavior governing one room.
type RoomConfig struct {
	AuthorEditWindow time.Duration
}

// RoomConfigSource identifies the scope that supplied an effective value.
type RoomConfigSource struct {
	Kind           RoomConfigScopeKind
	ID             string
	ProductDefault bool
}

// RoomConfigSources corresponds field-for-field with RoomConfig.
type RoomConfigSources struct {
	AuthorEditWindow RoomConfigSource
}

// RoomConfigState combines stored and resolved state for administration.
type RoomConfigState struct {
	Scope     RoomConfigScope
	Layer     RoomConfigLayer
	Effective RoomConfig
	Sources   RoomConfigSources
}

func roomConfigScopeKeyFor(scope RoomConfigScope) roomConfigScopeKey {
	return roomConfigScopeKey{kind: scope.Kind, id: scope.ID}
}

func roomConfigScopeProto(scope RoomConfigScope) *corev1.RoomConfigScope {
	switch scope.Kind {
	case RoomConfigScopeServer:
		return &corev1.RoomConfigScope{Scope: &corev1.RoomConfigScope_Server{Server: true}}
	case RoomConfigScopeRoomGroup:
		return &corev1.RoomConfigScope{Scope: &corev1.RoomConfigScope_RoomGroupId{RoomGroupId: scope.ID}}
	case RoomConfigScopeRoom:
		return &corev1.RoomConfigScope{Scope: &corev1.RoomConfigScope_RoomId{RoomId: scope.ID}}
	default:
		return &corev1.RoomConfigScope{}
	}
}

func roomConfigLayerProto(layer RoomConfigLayer) *corev1.RoomConfigLayer {
	result := &corev1.RoomConfigLayer{}
	if layer.AuthorEditWindow != nil {
		result.AuthorEditWindow = durationpb.New(*layer.AuthorEditWindow)
	}
	return result
}

func roomConfigChangedEvent(actorID string, scope RoomConfigScope, layer RoomConfigLayer, paths ...string) *corev1.Event {
	return newEvent(actorID, &corev1.Event{Event: &corev1.Event_RoomConfigChanged{RoomConfigChanged: &corev1.RoomConfigChangedEvent{
		Scope: roomConfigScopeProto(scope), Changes: roomConfigLayerProto(layer),
		ChangedFields: &fieldmaskpb.FieldMask{Paths: paths},
	}}})
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

func (c *ChattoCore) roomConfigLayer(scope RoomConfigScope) RoomConfigLayer {
	state := c.configModel.config.Projection().roomConfigLayer(roomConfigScopeKeyFor(scope))
	return RoomConfigLayer{AuthorEditWindow: state.authorEditWindow}
}

func resolveAuthorEditWindow(candidates ...struct {
	scope RoomConfigScope
	value *time.Duration
}) (time.Duration, RoomConfigSource) {
	for _, candidate := range candidates {
		if candidate.value != nil {
			return *candidate.value, RoomConfigSource{Kind: candidate.scope.Kind, ID: candidate.scope.ID}
		}
	}
	return DefaultAuthorEditWindow, RoomConfigSource{ProductDefault: true}
}

func (c *ChattoCore) effectiveRoomConfigForScope(scope RoomConfigScope) (RoomConfig, RoomConfigSources) {
	server := RoomConfigScope{Kind: RoomConfigScopeServer}
	candidates := []struct {
		scope RoomConfigScope
		value *time.Duration
	}{}
	if scope.Kind != RoomConfigScopeServer {
		value := c.roomConfigLayer(scope).AuthorEditWindow
		candidates = append(candidates, struct {
			scope RoomConfigScope
			value *time.Duration
		}{scope, value})
	}
	serverValue := c.roomConfigLayer(server).AuthorEditWindow
	candidates = append(candidates, struct {
		scope RoomConfigScope
		value *time.Duration
	}{server, serverValue})
	window, source := resolveAuthorEditWindow(candidates...)
	return RoomConfig{AuthorEditWindow: window}, RoomConfigSources{AuthorEditWindow: source}
}

// EffectiveServerRoomConfig resolves the server baseline and product defaults.
func (c *ChattoCore) EffectiveServerRoomConfig() (RoomConfig, RoomConfigSources) {
	return c.effectiveRoomConfigForScope(RoomConfigScope{Kind: RoomConfigScopeServer})
}

// EffectiveRoomConfig resolves room, current group, server, and product
// defaults. Direct messages intentionally skip room and group tiers.
func (c *ChattoCore) EffectiveRoomConfig(room *corev1.Room) (RoomConfig, RoomConfigSources) {
	server := RoomConfigScope{Kind: RoomConfigScopeServer}
	candidates := []struct {
		scope RoomConfigScope
		value *time.Duration
	}{}
	if room != nil && KindOfRoom(room) == KindChannel {
		roomScope := RoomConfigScope{Kind: RoomConfigScopeRoom, ID: room.GetId()}
		roomValue := c.roomConfigLayer(roomScope).AuthorEditWindow
		candidates = append(candidates, struct {
			scope RoomConfigScope
			value *time.Duration
		}{roomScope, roomValue})
		if groupID := c.roomModel.roomGroupForRoom(room.GetId()); groupID != "" {
			groupScope := RoomConfigScope{Kind: RoomConfigScopeRoomGroup, ID: groupID}
			groupValue := c.roomConfigLayer(groupScope).AuthorEditWindow
			candidates = append(candidates, struct {
				scope RoomConfigScope
				value *time.Duration
			}{groupScope, groupValue})
		}
	}
	serverValue := c.roomConfigLayer(server).AuthorEditWindow
	candidates = append(candidates, struct {
		scope RoomConfigScope
		value *time.Duration
	}{server, serverValue})
	window, source := resolveAuthorEditWindow(candidates...)
	return RoomConfig{AuthorEditWindow: window}, RoomConfigSources{AuthorEditWindow: source}
}

func (c *ChattoCore) roomConfigState(ctx context.Context, scope RoomConfigScope) RoomConfigState {
	effective, sources := c.effectiveRoomConfigForScope(scope)
	if scope.Kind == RoomConfigScopeRoom {
		if room, err := c.FindRoomByID(ctx, scope.ID); err == nil {
			effective, sources = c.EffectiveRoomConfig(room)
		}
	}
	return RoomConfigState{Scope: scope, Layer: c.roomConfigLayer(scope), Effective: effective, Sources: sources}
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

func validateRoomConfigUpdate(layer RoomConfigLayer, mask RoomConfigUpdateMask) error {
	if !mask.AuthorEditWindow {
		return invalidArgument("at least one room configuration field must be selected")
	}
	if layer.AuthorEditWindow == nil {
		return nil
	}
	if *layer.AuthorEditWindow < 0 || *layer.AuthorEditWindow > MaxAuthorEditWindow {
		return invalidArgument(fmt.Sprintf("author edit window must be between 0 and %s", MaxAuthorEditWindow))
	}
	return nil
}

func roomConfigUpdatePaths(mask RoomConfigUpdateMask) []string {
	paths := make([]string, 0, 1)
	if mask.AuthorEditWindow {
		paths = append(paths, "author_edit_window")
	}
	return paths
}

// allRoomConfigLayerPaths is the single list used when a resource lifecycle
// event removes an entire layer. Add every new RoomConfigLayer field here.
func allRoomConfigLayerPaths() []string {
	return []string{"author_edit_window"}
}

func roomConfigLayerMatchesUpdate(current, requested RoomConfigLayer, mask RoomConfigUpdateMask) bool {
	return !mask.AuthorEditWindow || equalOptionalDuration(current.AuthorEditWindow, requested.AuthorEditWindow)
}

// UpdateRoomConfig changes selected values in one room-configuration layer.
func (c *ChattoCore) UpdateRoomConfig(ctx context.Context, actorID string, scope RoomConfigScope, layer RoomConfigLayer, mask RoomConfigUpdateMask) (RoomConfigState, error) {
	if err := validateRoomConfigUpdate(layer, mask); err != nil {
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
		agg, filter, expectedSeq, err := c.configModel.prepareSubject(ctx, subject)
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
		if roomConfigLayerMatchesUpdate(current, layer, mask) {
			return c.GetRoomConfig(ctx, actorID, scope)
		}
		event := roomConfigChangedEvent(actorID, scope, layer, roomConfigUpdatePaths(mask)...)
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
