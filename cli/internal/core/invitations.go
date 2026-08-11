package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	inviteLinkTokenVersion = byte('1')
	inviteLinkMACBytes     = 12
	invitationIDLength     = 1 + idLength
	inviteLinkTokenLength  = 1 + invitationIDLength + 16 // 16 base64url characters encode the 96-bit MAC.
)

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

func (m *InvitationModel) LinkToken(id string) string {
	payload := string(inviteLinkTokenVersion) + id
	keyMAC := hmac.New(sha256.New, []byte(m.secretKey))
	_, _ = keyMAC.Write([]byte("chatto/invite-link/v1\x00"))
	signingKey := keyMAC.Sum(nil)
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write([]byte(payload))
	return payload + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:inviteLinkMACBytes])
}

func (m *InvitationModel) ParseLinkToken(token string) (string, error) {
	if len(token) != inviteLinkTokenLength || token[0] != inviteLinkTokenVersion {
		return "", ErrInvitationInvalid
	}
	id := token[1 : 1+invitationIDLength]
	if id[0] != 'I' || !hmac.Equal([]byte(m.LinkToken(id)), []byte(token)) {
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
	id, err := m.ParseLinkToken(token)
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
