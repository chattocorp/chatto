package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const invitationCodePrefix = "cht_INV1"

var (
	ErrInvitationInvalid                = errors.New("invitation is invalid")
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
	publisher  *evtstream.Publisher
	projection events.ProjectionHandle[*InvitationProjection]
	secretKey  string
}

func newInvitationModel(publisher *evtstream.Publisher, projection events.ProjectionHandle[*InvitationProjection], secretKey string) *InvitationModel {
	return &InvitationModel{publisher: publisher, projection: projection, secretKey: secretKey}
}

func (m *InvitationModel) Code(id string) string {
	payload := invitationCodePrefix + "." + id
	keyMAC := hmac.New(sha256.New, []byte(m.secretKey))
	_, _ = keyMAC.Write([]byte("chatto/invitation-code/v1\x00"))
	signingKey := keyMAC.Sum(nil)
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (m *InvitationModel) ParseCode(code string) (string, error) {
	parts := strings.Split(strings.TrimSpace(code), ".")
	if len(parts) != 3 || parts[0] != invitationCodePrefix || parts[1] == "" {
		return "", ErrInvitationInvalid
	}
	expected := m.Code(parts[1])
	if !hmac.Equal([]byte(expected), []byte(strings.TrimSpace(code))) {
		return "", ErrInvitationInvalid
	}
	return parts[1], nil
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

func (m *InvitationModel) validateCodeAt(code string, now time.Time) (InvitationState, error) {
	id, err := m.ParseCode(code)
	if err != nil {
		return InvitationState{}, ErrInvitationInvalid
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

func (c *ChattoCore) ValidateInvitationCode(ctx context.Context, code string) (string, error) {
	if err := c.invitationModel.projection.Projector().WaitForCurrent(ctx); err != nil {
		return "", err
	}
	state, err := c.invitationModel.validateCodeAt(code, time.Now())
	if err != nil {
		return "", err
	}
	return state.ID, nil
}

// InvitationCode deterministically reconstructs the signed capability for an invitation ID.
func (c *ChattoCore) InvitationCode(id string) string {
	return c.invitationModel.Code(id)
}

func (c *ChattoCore) GetInvitation(ctx context.Context, actorID, id string) (InvitationState, error) {
	if err := c.requireServerPermission(ctx, actorID, PermInviteManage); err != nil {
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
	if err := c.requireServerPermission(ctx, actorID, PermInviteManage); err != nil {
		return nil, err
	}
	if err := c.invitationModel.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	return c.invitationModel.projection.Projection().all(), nil
}

func (c *ChattoCore) CreateInvitation(ctx context.Context, actorID string, maxUses *uint32, expiresAt *time.Time) (InvitationState, error) {
	if err := c.requireServerPermission(ctx, actorID, PermInviteManage); err != nil {
		return InvitationState{}, err
	}
	if maxUses != nil && *maxUses == 0 {
		return InvitationState{}, ErrInvalidArgument
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return InvitationState{}, ErrInvalidArgument
	}
	id := NewInvitationID()
	payload := &corev1.InvitationCreatedEvent{InvitationId: id, MaxUses: maxUses}
	if expiresAt != nil {
		payload.ExpiresAt = timestamppb.New(*expiresAt)
	}
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_InvitationCreated{InvitationCreated: payload}})
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
	if err := c.requireServerPermission(ctx, actorID, PermInviteManage); err != nil {
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
		event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_InvitationRevoked{InvitationRevoked: &corev1.InvitationRevokedEvent{InvitationId: id}}})
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
