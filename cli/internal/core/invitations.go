package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	inviteLinkTokenBytes  = 12
	inviteLinkTokenLength = 16 // 16 unpadded base64url characters encode 96 bits.
)

var (
	ErrInvitationInvalid                = errors.New("invitation is invalid")
	errInviteLinkTokenCollision         = errors.New("invite-link token collision")
	errInvitationMutationRetryExhausted = errors.New("invitation mutation OCC retry exhausted")
)

type InvitationStatus string

const (
	InvitationStatusActive    InvitationStatus = "active"
	InvitationStatusExpired   InvitationStatus = "expired"
	InvitationStatusExhausted InvitationStatus = "exhausted"
	InvitationStatusRevoked   InvitationStatus = "revoked"
)

type InvitationModel struct {
	publisher    *evtstream.Publisher
	projection   events.ProjectionHandle[*InvitationProjection]
	signingKey   []byte
	tokenIndexMu sync.Mutex
	tokenIndex   map[string]string
	indexedIDs   int
}

func newInvitationModel(publisher *evtstream.Publisher, projection events.ProjectionHandle[*InvitationProjection], secretKey string) *InvitationModel {
	keyMAC := hmac.New(sha256.New, []byte(secretKey))
	_, _ = keyMAC.Write([]byte("chatto/invite-link/v2\x00"))
	return &InvitationModel{
		publisher:  publisher,
		projection: projection,
		signingKey: keyMAC.Sum(nil),
		tokenIndex: make(map[string]string),
	}
}

func (m *InvitationModel) LinkToken(id string) string {
	mac := hmac.New(sha256.New, m.signingKey)
	_, _ = mac.Write([]byte(id))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:inviteLinkTokenBytes])
}

func (m *InvitationModel) resolveLinkToken(token string) (string, error) {
	if len(token) != inviteLinkTokenLength {
		return "", ErrInvitationInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != inviteLinkTokenBytes || base64.RawURLEncoding.EncodeToString(decoded) != token {
		return "", ErrInvitationInvalid
	}

	// Invitation identities are append-only durable facts, so the number of
	// projected IDs is enough to tell whether this derived index is stale.
	m.tokenIndexMu.Lock()
	defer m.tokenIndexMu.Unlock()
	if m.projection.Projection().count() != m.indexedIDs {
		ids := m.projection.Projection().ids()
		index := make(map[string]string, len(ids))
		for _, id := range ids {
			derived := m.LinkToken(id)
			if existing, ok := index[derived]; ok && existing != id {
				return "", errInviteLinkTokenCollision
			}
			index[derived] = id
		}
		m.tokenIndex = index
		m.indexedIDs = len(ids)
	}
	id, ok := m.tokenIndex[token]
	if !ok {
		return "", ErrInvitationInvalid
	}
	return id, nil
}

func InvitationStatusAt(state InvitationState, now time.Time) InvitationStatus {
	if state.RevokedAt != nil {
		return InvitationStatusRevoked
	}
	if state.ExpiresAt != nil && !now.Before(*state.ExpiresAt) {
		return InvitationStatusExpired
	}
	if state.MaxUses != nil && state.UseCount >= *state.MaxUses {
		return InvitationStatusExhausted
	}
	return InvitationStatusActive
}

func (m *InvitationModel) validateLinkTokenAt(token string, now time.Time) (InvitationState, error) {
	id, err := m.resolveLinkToken(token)
	if err != nil {
		return InvitationState{}, err
	}
	return m.validateIDAt(id, now)
}

func (m *InvitationModel) validateIDAt(id string, now time.Time) (InvitationState, error) {
	state, ok := m.projection.Projection().get(id)
	if !ok || InvitationStatusAt(state, now) != InvitationStatusActive {
		return InvitationState{}, ErrInvitationInvalid
	}
	return state, nil
}

func (c *ChattoCore) ValidateInviteLinkToken(ctx context.Context, token string) (string, error) {
	if err := c.invitationModel.projection.Projector().WaitForCurrent(ctx); err != nil {
		return "", err
	}
	state, err := c.invitationModel.validateLinkTokenAt(token, time.Now())
	if err != nil {
		return "", err
	}
	return state.ID, nil
}

// InvitationLinkPath deterministically reconstructs the shareable path for an invitation ID.
func (c *ChattoCore) InvitationLinkPath(id string) string {
	return "/invite/" + c.invitationModel.LinkToken(id)
}

func (c *ChattoCore) GetInvitation(ctx context.Context, actorID, id string) (InvitationState, error) {
	if err := c.requireServerPermission(ctx, actorID, PermUserInvite); err != nil {
		return InvitationState{}, err
	}
	if err := c.invitationModel.projection.Projector().WaitForCurrent(ctx); err != nil {
		return InvitationState{}, err
	}
	state, ok := c.invitationModel.projection.Projection().get(id)
	if !ok {
		return InvitationState{}, ErrNotFound
	}
	return state, nil
}

func (c *ChattoCore) ListInvitations(ctx context.Context, actorID string) ([]InvitationState, error) {
	if err := c.requireServerPermission(ctx, actorID, PermUserInvite); err != nil {
		return nil, err
	}
	if err := c.invitationModel.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	return c.invitationModel.projection.Projection().all(), nil
}

func (c *ChattoCore) CreateInvitation(ctx context.Context, actorID string, maxUses *uint32, expiresAt *time.Time) (InvitationState, error) {
	if err := c.requireServerPermission(ctx, actorID, PermUserInvite); err != nil {
		return InvitationState{}, err
	}
	if maxUses != nil && *maxUses == 0 {
		return InvitationState{}, ErrInvalidArgument
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return InvitationState{}, ErrInvalidArgument
	}
	id := NewInvitationID()
	payload := &evtv1.InvitationCreatedEvent{InvitationId: id, MaxUses: maxUses}
	if expiresAt != nil {
		payload.ExpiresAt = timestamppb.New(*expiresAt)
	}
	event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_InvitationCreated{InvitationCreated: payload}})
	agg := evtstream.InvitationAggregate(id)
	seq, err := c.EventPublisher.AppendAtFilter(ctx, agg.SubjectFor(event), event, agg.AllEventsFilter(), 0)
	if err != nil {
		return InvitationState{}, err
	}
	if err := c.invitationModel.projection.Projector().WaitFor(ctx, events.SubjectPosition(agg.SubjectFor(event), seq)); err != nil {
		return InvitationState{}, err
	}
	state, ok := c.invitationModel.projection.Projection().get(id)
	if !ok {
		return InvitationState{}, fmt.Errorf("created invitation was not projected")
	}
	return state, nil
}

func (c *ChattoCore) RevokeInvitation(ctx context.Context, actorID, id string) (InvitationState, error) {
	if err := c.requireServerPermission(ctx, actorID, PermUserInvite); err != nil {
		return InvitationState{}, err
	}
	agg := evtstream.InvitationAggregate(id)
	for attempt := 0; attempt < 5; attempt++ {
		seq, err := c.EventPublisher.LastSubjectSeq(ctx, agg.AllEventsFilter())
		if err != nil {
			return InvitationState{}, err
		}
		if err := c.invitationModel.projection.Projector().WaitFor(ctx, events.SubjectPosition(agg.AllEventsFilter(), seq)); err != nil {
			return InvitationState{}, err
		}
		state, ok := c.invitationModel.projection.Projection().get(id)
		if !ok {
			return InvitationState{}, ErrNotFound
		}
		if state.RevokedAt != nil {
			return state, nil
		}
		event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_InvitationRevoked{InvitationRevoked: &evtv1.InvitationRevokedEvent{InvitationId: id}}})
		published, err := c.EventPublisher.AppendAtFilter(ctx, agg.SubjectFor(event), event, agg.AllEventsFilter(), seq)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return InvitationState{}, err
		}
		if err := c.invitationModel.projection.Projector().WaitFor(ctx, events.SubjectPosition(agg.SubjectFor(event), published)); err != nil {
			return InvitationState{}, err
		}
		state, _ = c.invitationModel.projection.Projection().get(id)
		return state, nil
	}
	return InvitationState{}, errInvitationMutationRetryExhausted
}

func (c *ChattoCore) requireServerPermission(ctx context.Context, actorID string, permission Permission) error {
	allowed, err := c.HasServerPermission(ctx, actorID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPermissionDenied
	}
	return nil
}
