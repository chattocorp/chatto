package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// InvitationCode reconstructs a single-use registration capability. The raw
// code is never persisted. Spaces, hyphens, and letter case are presentation.
func (c *ChattoCore) InvitationCode(id string) string {
	return c.invitationModel.code(id)
}

func (m *InvitationModel) code(id string) string {
	mac := hmac.New(sha256.New, m.signingKey)
	_, _ = mac.Write([]byte("registration-code/v1\x00" + id))
	raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(mac.Sum(nil)[:12])
	var groups []string
	for start := 0; start < len(raw); start += 4 {
		groups = append(groups, raw[start:min(start+4, len(raw))])
	}
	return strings.Join(groups, "-")
}

// ValidateInvitationCode checks a code without reserving or consuming it.
// Final account creation must validate its durable invitation again under OCC.
func (c *ChattoCore) ValidateInvitationCode(ctx context.Context, code string) (string, error) {
	if len(code) > 128 {
		return "", ErrInvitationInvalid
	}
	code = strings.ToUpper(strings.Join(strings.Fields(strings.ReplaceAll(code, "-", "")), ""))
	if len(code) != 20 {
		return "", ErrInvitationInvalid
	}
	if err := c.invitationModel.projection.Projector().WaitForCurrent(ctx); err != nil {
		return "", err
	}
	id, err := c.invitationModel.resolveCode(code)
	if err != nil {
		return "", err
	}
	if _, err := c.invitationModel.validateIDAt(id, time.Now()); err != nil {
		return "", err
	}
	return id, nil
}

// ValidateInvitationAdmission rechecks remembered admission, including
// expiry, revocation, and consumption. It does not consume the code.
func (c *ChattoCore) ValidateInvitationAdmission(ctx context.Context, id string) error {
	if err := c.invitationModel.projection.Projector().WaitForCurrent(ctx); err != nil {
		return err
	}
	_, err := c.invitationModel.validateIDAt(id, time.Now())
	return err
}

var (
	ErrInvitationInvalid                = errors.New("invitation is invalid")
	errInvitationCodeCollision          = errors.New("invitation code collision")
	errInvitationMutationRetryExhausted = errors.New("invitation mutation OCC retry exhausted")
)

type InvitationStatus string

const (
	InvitationStatusActive  InvitationStatus = "active"
	InvitationStatusExpired InvitationStatus = "expired"
	InvitationStatusUsed    InvitationStatus = "used"
	InvitationStatusRevoked InvitationStatus = "revoked"
)

type InvitationModel struct {
	projection   events.ProjectionHandle[*InvitationProjection]
	signingKey   []byte
	tokenIndexMu sync.Mutex
	tokenIndex   map[string]string
	indexedIDs   int
}

func newInvitationModel(projection events.ProjectionHandle[*InvitationProjection], secretKey string) *InvitationModel {
	keyMAC := hmac.New(sha256.New, []byte(secretKey))
	_, _ = keyMAC.Write([]byte("chatto/invitation-code/v1\x00"))
	return &InvitationModel{
		projection: projection,
		signingKey: keyMAC.Sum(nil),
		tokenIndex: make(map[string]string),
	}
}

func (m *InvitationModel) resolveCode(token string) (string, error) {
	// Invitation identities are append-only durable facts, so the number of
	// projected IDs is enough to tell whether this derived index is stale.
	m.tokenIndexMu.Lock()
	defer m.tokenIndexMu.Unlock()
	if m.projection.Projection().count() != m.indexedIDs {
		ids := m.projection.Projection().ids()
		index := make(map[string]string, len(ids))
		for _, id := range ids {
			derived := strings.ReplaceAll(m.code(id), "-", "")
			if existing, ok := index[derived]; ok && existing != id {
				return "", errInvitationCodeCollision
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
	if state.Used {
		return InvitationStatusUsed
	}
	if state.ExpiresAt != nil && !now.Before(*state.ExpiresAt) {
		return InvitationStatusExpired
	}
	return InvitationStatusActive
}

func (m *InvitationModel) validateIDAt(id string, now time.Time) (InvitationState, error) {
	state, ok := m.projection.Projection().get(id)
	if !ok || InvitationStatusAt(state, now) != InvitationStatusActive {
		return InvitationState{}, ErrInvitationInvalid
	}
	return state, nil
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

// InvitationNoteMaxLength is the maximum number of Unicode code points in a creation note.
const InvitationNoteMaxLength = 250

// CreateInvitationCode grants admission for exactly one new account. Expiry and
// the manager-only note are optional; all callers must have user.invite permission.
func (c *ChattoCore) CreateInvitationCode(ctx context.Context, actorID string, expiresAt *time.Time, note string) (InvitationState, error) {
	if err := c.requireServerPermission(ctx, actorID, PermUserInvite); err != nil {
		return InvitationState{}, err
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return InvitationState{}, ErrInvalidArgument
	}
	if !utf8.ValidString(note) || utf8.RuneCountInString(note) > InvitationNoteMaxLength {
		return InvitationState{}, ErrInvalidArgument
	}
	id := NewInvitationID()
	payload := &evtv1.InvitationCreatedEvent{InvitationId: id, Note: strings.TrimSpace(note)}
	if expiresAt != nil {
		payload.ExpiresAt = timestamppb.New(*expiresAt)
	}
	event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_InvitationCreated{InvitationCreated: payload}})
	agg := evtstream.InvitationAggregate(id)
	seq, err := c.EventPublisher.AppendAt(ctx, agg.SubjectFor(event), event, events.ExpectFilterSeq(agg.AllEventsFilter(), 0))
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
	for range 5 {
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
		published, err := c.EventPublisher.AppendAt(ctx, agg.SubjectFor(event), event, events.ExpectFilterSeq(agg.AllEventsFilter(), seq))
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
