package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	// DefaultAuthorEditWindow is the product default used when no runtime
	// policy override is stored.
	DefaultAuthorEditWindow = 3 * time.Hour
	// MessageEditWindow is retained as the compatibility name for the product
	// default. Enforcement resolves the runtime policy instead.
	MessageEditWindow = DefaultAuthorEditWindow
	// MaxAuthorEditWindow bounds operator-controlled edit windows.
	MaxAuthorEditWindow = 30 * 24 * time.Hour
)

type PolicyScopeKind int

const (
	PolicyScopeServer PolicyScopeKind = iota + 1
	PolicyScopeRoomGroup
	PolicyScopeRoom
)

// PolicyScope identifies one baseline runtime-policy administration tier.
type PolicyScope struct {
	Kind PolicyScopeKind
	ID   string
}

// PolicyOverrides contains sparse values stored at one scope.
type PolicyOverrides struct {
	AuthorEditWindowSeconds *int32
}

// PolicyUpdateMask selects which override fields an update may change.
type PolicyUpdateMask struct {
	AuthorEditWindow bool
}

// EffectivePolicies contains resolved runtime behavior for one resource.
type EffectivePolicies struct {
	AuthorEditWindowSeconds int32
}

// PolicySource identifies the scope that supplied an effective value.
type PolicySource struct {
	Kind           PolicyScopeKind
	ID             string
	ProductDefault bool
}

// PolicySources corresponds field-for-field with EffectivePolicies.
type PolicySources struct {
	AuthorEditWindow PolicySource
}

// PolicyConfiguration combines stored and resolved state for administration.
type PolicyConfiguration struct {
	Scope     PolicyScope
	Overrides PolicyOverrides
	Effective EffectivePolicies
	Sources   PolicySources
}

func baselinePolicyTarget(scope PolicyScope) runtimePolicyTargetKey {
	return runtimePolicyTargetKey{
		scopeKind: policyScopeProto(scope.Kind), scopeID: scope.ID,
		subjectKind: corev1.RuntimePolicySubjectKind_RUNTIME_POLICY_SUBJECT_KIND_BASELINE,
	}
}

func policyScopeProto(kind PolicyScopeKind) corev1.RuntimePolicyScopeKind {
	switch kind {
	case PolicyScopeServer:
		return corev1.RuntimePolicyScopeKind_RUNTIME_POLICY_SCOPE_KIND_SERVER
	case PolicyScopeRoomGroup:
		return corev1.RuntimePolicyScopeKind_RUNTIME_POLICY_SCOPE_KIND_ROOM_GROUP
	case PolicyScopeRoom:
		return corev1.RuntimePolicyScopeKind_RUNTIME_POLICY_SCOPE_KIND_ROOM
	default:
		return corev1.RuntimePolicyScopeKind_RUNTIME_POLICY_SCOPE_KIND_UNSPECIFIED
	}
}

func policyTargetProto(scope PolicyScope) *corev1.RuntimePolicyTarget {
	return &corev1.RuntimePolicyTarget{
		ScopeKind: policyScopeProto(scope.Kind), ScopeId: scope.ID,
		SubjectKind: corev1.RuntimePolicySubjectKind_RUNTIME_POLICY_SUBJECT_KIND_BASELINE,
	}
}

func (c *ChattoCore) validatePolicyScope(ctx context.Context, scope PolicyScope) error {
	switch scope.Kind {
	case PolicyScopeServer:
		if scope.ID != "" {
			return invalidArgument("server policy scope must not have an ID")
		}
	case PolicyScopeRoomGroup:
		if scope.ID == "" {
			return invalidArgument("room group policy scope requires an ID")
		}
		if _, err := c.GetRoomGroup(ctx, scope.ID); err != nil {
			return err
		}
	case PolicyScopeRoom:
		if scope.ID == "" {
			return invalidArgument("room policy scope requires an ID")
		}
		room, err := c.FindRoomByID(ctx, scope.ID)
		if err != nil {
			return err
		}
		if KindOfRoom(room) != KindChannel {
			return invalidArgument("direct messages do not support room policy overrides")
		}
	default:
		return invalidArgument("policy scope is required")
	}
	return nil
}

func (c *ChattoCore) authorizePolicyScope(ctx context.Context, actorID string, scope PolicyScope) error {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return err
	}
	switch scope.Kind {
	case PolicyScopeServer:
		ok, err := c.CanManageServer(ctx, actorID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrPermissionDenied
		}
	case PolicyScopeRoomGroup:
		return c.requireCanManageRoomGroup(ctx, actorID, scope.ID)
	case PolicyScopeRoom:
		ok, err := c.PermResolver().HasRoomPermission(ctx, actorID, KindChannel, scope.ID, PermRoomManage)
		if err != nil {
			return err
		}
		if !ok {
			return ErrPermissionDenied
		}
	default:
		return invalidArgument("policy scope is required")
	}
	return nil
}

func (c *ChattoCore) policyOverrides(scope PolicyScope) PolicyOverrides {
	state := c.configModel.config.Projection().policyState(baselinePolicyTarget(scope))
	return PolicyOverrides{AuthorEditWindowSeconds: state.authorEditWindowSeconds}
}

func resolveAuthorEditWindow(candidates ...struct {
	scope PolicyScope
	value *int32
}) (int32, PolicySource) {
	for _, candidate := range candidates {
		if candidate.value != nil {
			return *candidate.value, PolicySource{Kind: candidate.scope.Kind, ID: candidate.scope.ID}
		}
	}
	return int32(DefaultAuthorEditWindow / time.Second), PolicySource{ProductDefault: true}
}

func (c *ChattoCore) effectivePoliciesForScope(scope PolicyScope) (EffectivePolicies, PolicySources) {
	server := PolicyScope{Kind: PolicyScopeServer}
	candidates := []struct {
		scope PolicyScope
		value *int32
	}{}
	if scope.Kind != PolicyScopeServer {
		override := c.policyOverrides(scope).AuthorEditWindowSeconds
		candidates = append(candidates, struct {
			scope PolicyScope
			value *int32
		}{scope, override})
	}
	serverOverride := c.policyOverrides(server).AuthorEditWindowSeconds
	candidates = append(candidates, struct {
		scope PolicyScope
		value *int32
	}{server, serverOverride})
	seconds, source := resolveAuthorEditWindow(candidates...)
	return EffectivePolicies{AuthorEditWindowSeconds: seconds}, PolicySources{AuthorEditWindow: source}
}

// EffectiveServerPolicies resolves the server baseline and product defaults.
func (c *ChattoCore) EffectiveServerPolicies() (EffectivePolicies, PolicySources) {
	return c.effectivePoliciesForScope(PolicyScope{Kind: PolicyScopeServer})
}

// EffectiveRoomPolicies resolves room, current group, server, and product
// defaults. Direct messages intentionally skip room and group tiers.
func (c *ChattoCore) EffectiveRoomPolicies(room *corev1.Room) (EffectivePolicies, PolicySources) {
	server := PolicyScope{Kind: PolicyScopeServer}
	candidates := []struct {
		scope PolicyScope
		value *int32
	}{}
	if room != nil && KindOfRoom(room) == KindChannel {
		roomScope := PolicyScope{Kind: PolicyScopeRoom, ID: room.GetId()}
		roomOverride := c.policyOverrides(roomScope).AuthorEditWindowSeconds
		candidates = append(candidates, struct {
			scope PolicyScope
			value *int32
		}{roomScope, roomOverride})
		if groupID := c.roomModel.roomGroupForRoom(room.GetId()); groupID != "" {
			groupScope := PolicyScope{Kind: PolicyScopeRoomGroup, ID: groupID}
			groupOverride := c.policyOverrides(groupScope).AuthorEditWindowSeconds
			candidates = append(candidates, struct {
				scope PolicyScope
				value *int32
			}{groupScope, groupOverride})
		}
	}
	serverOverride := c.policyOverrides(server).AuthorEditWindowSeconds
	candidates = append(candidates, struct {
		scope PolicyScope
		value *int32
	}{server, serverOverride})
	seconds, source := resolveAuthorEditWindow(candidates...)
	return EffectivePolicies{AuthorEditWindowSeconds: seconds}, PolicySources{AuthorEditWindow: source}
}

// GetPolicyConfiguration returns stored and effective baseline policy state.
func (c *ChattoCore) GetPolicyConfiguration(ctx context.Context, actorID string, scope PolicyScope) (PolicyConfiguration, error) {
	if err := c.validatePolicyScope(ctx, scope); err != nil {
		return PolicyConfiguration{}, err
	}
	if err := c.authorizePolicyScope(ctx, actorID, scope); err != nil {
		return PolicyConfiguration{}, err
	}
	effective, sources := c.effectivePoliciesForScope(scope)
	if scope.Kind == PolicyScopeRoom {
		room, _ := c.FindRoomByID(ctx, scope.ID)
		effective, sources = c.EffectiveRoomPolicies(room)
	} else if scope.Kind == PolicyScopeRoomGroup {
		server := PolicyScope{Kind: PolicyScopeServer}
		groupOverride := c.policyOverrides(scope).AuthorEditWindowSeconds
		serverOverride := c.policyOverrides(server).AuthorEditWindowSeconds
		seconds, source := resolveAuthorEditWindow(
			struct {
				scope PolicyScope
				value *int32
			}{scope, groupOverride},
			struct {
				scope PolicyScope
				value *int32
			}{server, serverOverride},
		)
		effective = EffectivePolicies{AuthorEditWindowSeconds: seconds}
		sources = PolicySources{AuthorEditWindow: source}
	}
	return PolicyConfiguration{Scope: scope, Overrides: c.policyOverrides(scope), Effective: effective, Sources: sources}, nil
}

func validatePolicyUpdate(overrides PolicyOverrides, mask PolicyUpdateMask) error {
	if !mask.AuthorEditWindow {
		return invalidArgument("at least one policy field must be selected")
	}
	if overrides.AuthorEditWindowSeconds == nil {
		return nil
	}
	max := int32(MaxAuthorEditWindow / time.Second)
	if *overrides.AuthorEditWindowSeconds < 0 || *overrides.AuthorEditWindowSeconds > max {
		return invalidArgument(fmt.Sprintf("author edit window must be between 0 and %d seconds", max))
	}
	return nil
}

// UpdatePolicyConfiguration updates selected baseline overrides at one scope.
func (c *ChattoCore) UpdatePolicyConfiguration(ctx context.Context, actorID string, scope PolicyScope, overrides PolicyOverrides, mask PolicyUpdateMask) (PolicyConfiguration, error) {
	if err := validatePolicyUpdate(overrides, mask); err != nil {
		return PolicyConfiguration{}, err
	}
	subject := ConfigSubjectServer
	if scope.Kind != PolicyScopeServer {
		subject = scope.ID
	}
	for attempt := 0; attempt < maxConfigUpdateRetries; attempt++ {
		// Scope deletion advances this same config aggregate. Revalidate on
		// every OCC attempt against the current authoritative lifecycle tail so
		// a delete that wins the race prevents a later policy fact from
		// recreating unreachable projected state on a lagging replica.
		if err := c.waitForPolicyScopeCurrent(ctx, scope); err != nil {
			return PolicyConfiguration{}, err
		}
		if err := c.validatePolicyScope(ctx, scope); err != nil {
			return PolicyConfiguration{}, err
		}
		authorizationSeq, err := c.authorizationFenceSeq(ctx)
		if err != nil {
			return PolicyConfiguration{}, err
		}
		agg, filter, expectedSeq, err := c.configModel.prepareSubject(ctx, subject)
		if err != nil {
			return PolicyConfiguration{}, err
		}
		if err := c.waitForPolicyAuthorization(ctx, actorID); err != nil {
			return PolicyConfiguration{}, err
		}
		if err := c.authorizePolicyScope(ctx, actorID, scope); err != nil {
			return PolicyConfiguration{}, err
		}
		current := c.policyOverrides(scope).AuthorEditWindowSeconds
		if equalOptionalInt32(current, overrides.AuthorEditWindowSeconds) {
			return c.GetPolicyConfiguration(ctx, actorID, scope)
		}
		target := policyTargetProto(scope)
		var event *corev1.Event
		if overrides.AuthorEditWindowSeconds == nil {
			event = newEvent(actorID, &corev1.Event{Event: &corev1.Event_AuthorEditWindowCleared{AuthorEditWindowCleared: &corev1.AuthorEditWindowClearedEvent{Target: target}}})
		} else {
			event = newEvent(actorID, &corev1.Event{Event: &corev1.Event_AuthorEditWindowSet{AuthorEditWindowSet: &corev1.AuthorEditWindowSetEvent{Target: target, Seconds: *overrides.AuthorEditWindowSeconds}}})
		}
		entries := []evtstream.BatchEntry{{Subject: agg.SubjectFor(event), Event: event, ExpectedSeq: expectedSeq, FilterSubject: filter, HasOCC: true}}
		seqs, err := c.appendAuthorizationFencedBatch(ctx, actorID, entries, authorizationSeq)
		if err == nil {
			if err := c.configModel.waitFor(ctx, events.SubjectPosition(entries[0].Subject, seqs[0])); err != nil {
				return PolicyConfiguration{}, err
			}
			return c.GetPolicyConfiguration(ctx, actorID, scope)
		}
		if !errors.Is(err, events.ErrConflict) {
			return PolicyConfiguration{}, err
		}
		select {
		case <-ctx.Done():
			return PolicyConfiguration{}, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return PolicyConfiguration{}, ErrConfigConflict
}

func (c *ChattoCore) waitForPolicyScopeCurrent(ctx context.Context, scope PolicyScope) error {
	var (
		position events.StreamPosition
		err      error
	)
	switch scope.Kind {
	case PolicyScopeRoom:
		position, err = c.EventPublisher.LastSubjectPosition(ctx, evtstream.RoomAggregate(scope.ID).AllEventsFilter())
		if err == nil && !position.IsZero() {
			err = c.roomModel.waitForDirectory(ctx, position)
		}
	case PolicyScopeRoomGroup:
		position, err = c.EventPublisher.LastSubjectPosition(ctx, evtstream.GroupAggregate(scope.ID).AllEventsFilter())
		if err == nil && !position.IsZero() {
			err = c.roomModel.waitForGroupLayout(ctx, position)
		}
	case PolicyScopeServer:
		return nil
	default:
		return invalidArgument("policy scope is required")
	}
	if err != nil {
		return fmt.Errorf("wait for policy scope lifecycle: %w", err)
	}
	return nil
}

func (c *ChattoCore) waitForPolicyAuthorization(ctx context.Context, actorID string) error {
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

func equalOptionalInt32(a, b *int32) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
