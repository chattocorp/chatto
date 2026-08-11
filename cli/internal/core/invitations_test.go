package core

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func invitationAdmin(t *testing.T, c *ChattoCore) string {
	t.Helper()
	ctx := testContext(t)
	admin, err := c.CreateUser(ctx, SystemActorID, "invite-admin", "Invite Admin", "password123")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := c.AssignAdminRole(ctx, admin.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	return admin.Id
}

func TestInvitationCodesAreDeterministicAndSecretBound(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	adminID := invitationAdmin(t, c)

	state, err := c.CreateInvitation(ctx, adminID, nil, nil)
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}
	first := c.InvitationCode(state.ID)
	if second := c.InvitationCode(state.ID); second != first {
		t.Fatalf("InvitationCode changed: %q != %q", second, first)
	}
	if got, err := c.ValidateInvitationCode(ctx, first); err != nil || got != state.ID {
		t.Fatalf("ValidateInvitationCode = %q, %v; want %q, nil", got, err, state.ID)
	}

	rotated := newInvitationModel(c.EventPublisher, c.invitationModel.projection, "rotated-secret")
	if _, err := rotated.ParseCode(first); !errors.Is(err, ErrInvitationInvalid) {
		t.Fatalf("rotated secret ParseCode error = %v, want ErrInvitationInvalid", err)
	}
	if rotated.Code(state.ID) == first {
		t.Fatal("rotating the root secret did not change the invitation code")
	}
}

func TestInvitationManagementRequiresPermissionAndRetainsRevokedInvitation(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	adminID := invitationAdmin(t, c)
	member, err := c.CreateUser(ctx, SystemActorID, "invite-member", "Invite Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}

	if _, err := c.CreateInvitation(ctx, member.Id, nil, nil); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("member CreateInvitation error = %v, want ErrPermissionDenied", err)
	}
	maxUses := uint32(2)
	state, err := c.CreateInvitation(ctx, adminID, &maxUses, nil)
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}
	revoked, err := c.RevokeInvitation(ctx, adminID, state.ID)
	if err != nil {
		t.Fatalf("RevokeInvitation: %v", err)
	}
	if got := InvitationStatusAt(revoked, time.Now()); got != InvitationStatusRevoked {
		t.Fatalf("revoked status = %q, want %q", got, InvitationStatusRevoked)
	}
	listed, err := c.ListInvitations(ctx, adminID)
	if err != nil {
		t.Fatalf("ListInvitations: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != state.ID || listed[0].RevokedAt == nil {
		t.Fatalf("ListInvitations = %+v, want retained revoked invitation %q", listed, state.ID)
	}
	if _, err := c.ValidateInvitationCode(ctx, c.InvitationCode(state.ID)); !errors.Is(err, ErrInvitationInvalid) {
		t.Fatalf("revoked ValidateInvitationCode error = %v, want ErrInvitationInvalid", err)
	}
}

func TestInvitationRedemptionIsAtomicAndLimitedAcrossConcurrentSignups(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	adminID := invitationAdmin(t, c)
	maxUses := uint32(1)
	invitation, err := c.CreateInvitation(ctx, adminID, &maxUses, nil)
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := c.CreateVerifiedUserWithInvitation(
				ctx,
				SystemActorID,
				fmt.Sprintf("invite-racer-%d", i),
				fmt.Sprintf("Invite Racer %d", i),
				"password123",
				fmt.Sprintf("invite-racer-%d@example.test", i),
				invitation.ID,
			)
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)

	succeeded := 0
	rejected := 0
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrInvitationInvalid):
			rejected++
		default:
			t.Fatalf("concurrent signup error = %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent signups = %d succeeded, %d rejected; want 1 and 1", succeeded, rejected)
	}
	state, err := c.GetInvitation(ctx, adminID, invitation.ID)
	if err != nil {
		t.Fatalf("GetInvitation: %v", err)
	}
	if state.UseCount != 1 || InvitationStatusAt(state, time.Now()) != InvitationStatusExhausted {
		t.Fatalf("invitation state = %+v, want one use and exhausted", state)
	}
}

func TestFailedSignupDoesNotConsumeInvitation(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	adminID := invitationAdmin(t, c)
	if _, err := c.CreateVerifiedUser(ctx, SystemActorID, "existing-invite-email", "Existing Invite Email", "password123", "claimed@example.test"); err != nil {
		t.Fatalf("CreateVerifiedUser existing: %v", err)
	}
	maxUses := uint32(1)
	invitation, err := c.CreateInvitation(ctx, adminID, &maxUses, nil)
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}

	if _, err := c.CreateVerifiedUserWithInvitation(ctx, SystemActorID, "failed-invite", "Failed Invite", "password123", "claimed@example.test", invitation.ID); !errors.Is(err, ErrEmailAlreadyVerified) {
		t.Fatalf("CreateVerifiedUserWithInvitation error = %v, want ErrEmailAlreadyVerified", err)
	}
	state, err := c.GetInvitation(ctx, adminID, invitation.ID)
	if err != nil {
		t.Fatalf("GetInvitation: %v", err)
	}
	if state.UseCount != 0 || InvitationStatusAt(state, time.Now()) != InvitationStatusActive {
		t.Fatalf("invitation state after failed signup = %+v, want unused and active", state)
	}
	if _, err := c.GetUserByLogin(ctx, "failed-invite"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetUserByLogin failed signup error = %v, want ErrNotFound", err)
	}
}
