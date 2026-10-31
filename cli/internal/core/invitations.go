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

// GetInvitation returns a visible code. Inaccessible and unknown IDs both return ErrNotFound.
func (c *ChattoCore) GetInvitation(ctx context.Context, actorID, id string) (InvitationState, error) {
	manageAll, err := c.invitationAccess(ctx, actorID)
	if err != nil {
		return InvitationState{}, err
	}
	if err := c.invitationModel.projection.Projector().WaitForCurrent(ctx); err != nil {
		return InvitationState{}, err
	}
	state, ok := c.invitationModel.projection.Projection().get(id)
	if !ok || (!manageAll && state.CreatedBy != actorID) {
		return InvitationState{}, ErrNotFound
	}
	return state, nil
}

// ListInvitations returns all visible codes. Callers must paginate after this ownership filter.
func (c *ChattoCore) ListInvitations(ctx context.Context, actorID string) ([]InvitationState, error) {
	manageAll, err := c.invitationAccess(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if err := c.invitationModel.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	states := c.invitationModel.projection.Projection().all()
	if manageAll {
		return states, nil
	}
	// Filter before transport pagination so counts and offsets reveal only own codes.
	owned := make([]InvitationState, 0, len(states))
	for _, state := range states {
		if state.CreatedBy == actorID {
			owned = append(owned, state)
		}
	}
	return owned, nil
}

// invitationAccess grants all-code access only through effective management
// permission. Without privileged mode, an admin with user.invite can still
// manage their own codes. Ownership never replaces the permission requirement.
func (c *ChattoCore) invitationAccess(ctx context.Context, actorID string) (bool, error) {
	manageAll, err := c.HasServerPermission(ctx, actorID, PermUserManageInvites)
	if err != nil || manageAll {
		return manageAll, err
	}
	return false, c.requireServerPermission(ctx, actorID, PermUserInvite)
}

// InvitationNoteMaxLength is the maximum number of Unicode code points in a creation note.
const InvitationNoteMaxLength = 250

// CreateInvitationCode grants admission for exactly one new account for seven
// days. The server sets expiry for all callers. The note is optional.
func (c *ChattoCore) CreateInvitationCode(ctx context.Context, actorID string, note string) (InvitationState, error) {
	if _, err := c.invitationAccess(ctx, actorID); err != nil {
		return InvitationState{}, err
	}
	return c.createInvitationCode(ctx, actorID, note)
}

// IssueInvitationCode issues a code with the same fixed seven-day lifetime.
// Callers can retrieve their own codes through the management API.
// Like admin creation, this is not idempotent and must not be retried automatically.
func (c *ChattoCore) IssueInvitationCode(ctx context.Context, actorID string) (InvitationState, error) {
	if err := c.requireServerPermission(ctx, actorID, PermUserInvite); err != nil {
		return InvitationState{}, err
	}
	return c.createInvitationCode(ctx, actorID, "")
}

// createInvitationCode is shared by the separately authorized member and admin operations.
func (c *ChattoCore) createInvitationCode(ctx context.Context, actorID string, note string) (InvitationState, error) {
	if !utf8.ValidString(note) || utf8.RuneCountInString(note) > InvitationNoteMaxLength {
		return InvitationState{}, ErrInvalidArgument
	}
	id := NewInvitationID()
	payload := &evtv1.InvitationCreatedEvent{InvitationId: id, Note: strings.TrimSpace(note)}
	event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_InvitationCreated{InvitationCreated: payload}})
	// Use the same server timestamp for creation and expiry so the lifetime is exact.
	payload.ExpiresAt = timestamppb.New(event.CreatedAt.AsTime().Add(7 * 24 * time.Hour))
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

// RevokeInvitation invalidates a visible code and preserves its history. Repeated revocation is safe.
func (c *ChattoCore) RevokeInvitation(ctx context.Context, actorID, id string) (InvitationState, error) {
	manageAll, err := c.invitationAccess(ctx, actorID)
	if err != nil {
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
		if !ok || (!manageAll && state.CreatedBy != actorID) {
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
