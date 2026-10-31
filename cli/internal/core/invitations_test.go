package core

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/config"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
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
	if _, err := c.UpdateServerConfig(ctx, admin.Id, ServerConfigUpdateInput{RegistrationAdmission: new(evtv1.RegistrationAdmission_REGISTRATION_ADMISSION_INVITE_ONLY)}); err != nil {
		t.Fatal(err)
	}
	return admin.Id
}

func TestInvitationCodesAreDeterministicAndSecretBound(t *testing.T) {
	t.Parallel()

	c, _ := setupTestCore(t)
	ctx := testContext(t)
	adminID := invitationAdmin(t, c)

	state, err := c.CreateInvitationCode(ctx, adminID, "")
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}
	first := c.InvitationCode(state.ID)
	if !regexp.MustCompile(`^[A-Z2-7]{4}(-[A-Z2-7]{4}){4}$`).MatchString(first) {
		t.Fatal("invitation code does not have five groups of four base32 characters")
	}
	if strings.Contains(first, state.ID) {
		t.Fatal("invitation code exposes its public ID")
	}
	if second := c.InvitationCode(state.ID); second != first {
		t.Fatal("InvitationCode changed")
	}
	if got, err := c.ValidateInvitationCode(ctx, first); err != nil || got != state.ID {
		t.Fatalf("ValidateInvitationCode = %q, %v; want %q, nil", got, err, state.ID)
	}
	for _, invalid := range []string{"short", strings.Repeat("!", 16), first + "A"} {
		if _, err := c.ValidateInvitationCode(ctx, invalid); !errors.Is(err, ErrInvitationInvalid) {
			t.Errorf("invalid code error = %v, want ErrInvitationInvalid", err)
		}
	}

	rotated := newInvitationModel(c.invitationModel.projection, "rotated-secret")
	if _, err := rotated.resolveCode(strings.ReplaceAll(first, "-", "")); !errors.Is(err, ErrInvitationInvalid) {
		t.Fatalf("rotated secret validation error = %v, want ErrInvitationInvalid", err)
	}
	if rotated.code(state.ID) == first {
		t.Fatal("rotating the root secret did not change the invitation code")
	}
	if got, err := c.ValidateInvitationCode(ctx, "  "+strings.ToLower(strings.ReplaceAll(first, "-", " "))+"  "); err != nil || got != state.ID {
		t.Fatalf("case and space normalization: %v", err)
	}
}

func TestInvitationNotesValidateAndSurviveColdReplay(t *testing.T) {
	t.Parallel()
	ns, _ := testutil.StartNATS(t)
	c := startReplica(t, ns, testCoreConfig())
	adminID := invitationAdmin(t, c)
	ctx := testContext(t)
	for _, note := range []string{strings.Repeat("a", 251), strings.Repeat("界", 251), string([]byte{0xff})} {
		if _, err := c.CreateInvitationCode(ctx, adminID, note); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("invalid note: got %v, want invalid argument", err)
		}
	}
	for _, note := range []string{"", "  \t", "  For the design team  ", strings.Repeat("界", 250)} {
		state, err := c.CreateInvitationCode(ctx, adminID, note)
		if err != nil {
			t.Fatal(err)
		}
		if state.Note != strings.TrimSpace(note) {
			t.Fatal("note was not normalized and projected")
		}
	}
	states, err := c.ListInvitations(ctx, adminID)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 4 {
		t.Fatalf("invalid notes created codes: got %d codes", len(states))
	}
	// Invitations are cold-replayed, not restored from a projection snapshot.
	replayed := startReplica(t, ns, testCoreConfig())
	for _, state := range states {
		got, err := replayed.GetInvitation(ctx, adminID, state.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Note != state.Note {
			t.Fatal("cold replay lost the creation note")
		}
		revoked, err := replayed.RevokeInvitation(ctx, adminID, state.ID)
		if err != nil {
			t.Fatal(err)
		}
		if revoked.Note != state.Note {
			t.Fatal("revocation lost the creation note")
		}
	}
}

func TestManagedProviderSignupRequiresInvitation(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	adminID := invitationAdmin(t, c)
	ctx := testContext(t)
	c.config.AuthProviders = []config.AuthProviderConfig{{ID: "company", Type: "oidc", IssuerURL: "https://company.example", ManageUsernames: true}}
	flow := &PendingExternalIdentityFlow{Kind: ExternalIdentityFlowKindCreate, ProviderID: "company", ProviderType: "oidc", Issuer: "https://company.example", Subject: "new-subject", ProviderUsername: "new-member"}
	if _, err := c.CreateUserForExternalIdentity(ctx, "ignored", "", flow); !errors.Is(err, ErrInvitationInvalid) {
		t.Fatalf("managed signup without code: got %v, want invalid invitation", err)
	}
	invitation, err := c.CreateInvitationCode(ctx, adminID, "For new member")
	if err != nil {
		t.Fatal(err)
	}
	flow.InvitationID = invitation.ID
	user, err := c.CreateUserForExternalIdentity(ctx, "ignored", "", flow)
	if err != nil {
		t.Fatal(err)
	}
	if user.Login != "new-member" {
		t.Fatalf("managed username = %q", user.Login)
	}
	state, err := c.GetInvitation(ctx, adminID, invitation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Used || state.Note != invitation.Note {
		t.Fatal("managed signup did not retain and consume the invitation")
	}
}

func TestInvitationCodeIndexRefreshesFromProjectedInvitations(t *testing.T) {
	t.Parallel()

	c, _ := setupTestCore(t)
	ctx := testContext(t)
	adminID := invitationAdmin(t, c)

	first, err := c.CreateInvitationCode(ctx, adminID, "")
	if err != nil {
		t.Fatalf("CreateInvitation first: %v", err)
	}
	firstToken := c.InvitationCode(first.ID)
	if got, err := c.ValidateInvitationCode(ctx, firstToken); err != nil || got != first.ID {
		t.Fatalf("ValidateInvitationCode first = %q, %v; want %q, nil", got, err, first.ID)
	}

	second, err := c.CreateInvitationCode(ctx, adminID, "")
	if err != nil {
		t.Fatalf("CreateInvitation second: %v", err)
	}
	secondToken := c.InvitationCode(second.ID)
	if got, err := c.ValidateInvitationCode(ctx, secondToken); err != nil || got != second.ID {
		t.Fatalf("ValidateInvitationCode second = %q, %v; want %q, nil", got, err, second.ID)
	}
}

func TestInvitationAdmissionDoesNotConsumeAndRejectsExpiry(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	adminID := invitationAdmin(t, c)
	invitation, err := c.CreateInvitationCode(ctx, adminID, "")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := c.ValidateInvitationAdmission(ctx, invitation.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := c.ValidateInvitationCode(ctx, c.InvitationCode(invitation.ID)); err != nil {
			t.Fatal(err)
		}
	}
	state, err := c.GetInvitation(ctx, adminID, invitation.ID)
	if err != nil || state.Used {
		t.Fatalf("validation consumed invitation: %v", err)
	}
	if invitation.ExpiresAt == nil || !invitation.ExpiresAt.Equal(invitation.CreatedAt.Add(7*24*time.Hour)) {
		t.Fatal("server must set expiry to exactly seven days after creation")
	}
	if _, err := c.invitationModel.validateIDAt(invitation.ID, *invitation.ExpiresAt); !errors.Is(err, ErrInvitationInvalid) {
		t.Fatalf("expired admission: %v", err)
	}
}

func TestInvitationManagementRequiresPermissionAndRetainsRevokedInvitation(t *testing.T) {
	t.Parallel()

	c, _ := setupTestCore(t)
	ctx := testContext(t)
	adminID := invitationAdmin(t, c)
	member, err := c.CreateUser(ctx, SystemActorID, "invite-member", "Invite Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}

	if _, err := c.CreateInvitationCode(ctx, member.Id, ""); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("member CreateInvitation error = %v, want ErrPermissionDenied", err)
	}
	state, err := c.CreateInvitationCode(ctx, adminID, "")
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
	token := c.InvitationCode(state.ID)
	if _, err := c.ValidateInvitationCode(ctx, token); !errors.Is(err, ErrInvitationInvalid) {
		t.Fatalf("revoked ValidateInvitationCode error = %v, want ErrInvitationInvalid", err)
	}
}

func TestInvitationRedemptionIsAtomicAndLimitedAcrossConcurrentSignups(t *testing.T) {
	t.Parallel()

	c, _ := setupTestCore(t)
	ctx := testContext(t)
	adminID := invitationAdmin(t, c)
	invitation, err := c.CreateInvitationCode(ctx, adminID, "")
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := c.CreateSelfServiceUser(
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
	if !state.Used || InvitationStatusAt(state, time.Now()) != InvitationStatusUsed {
		t.Fatalf("invitation state = %+v, want used", state)
	}
	// A consumed code stays used after its expiry date.
	past := time.Now().Add(-time.Hour)
	state.ExpiresAt = &past
	if InvitationStatusAt(state, time.Now()) != InvitationStatusUsed {
		t.Fatal("used code became expired")
	}
}

func TestFailedSignupDoesNotConsumeInvitation(t *testing.T) {
	t.Parallel()

	c, _ := setupTestCore(t)
	ctx := testContext(t)
	adminID := invitationAdmin(t, c)
	if _, err := c.CreateVerifiedUser(ctx, SystemActorID, "existing-invite-email", "Existing Invite Email", "password123", "claimed@example.test"); err != nil {
		t.Fatalf("CreateVerifiedUser existing: %v", err)
	}
	invitation, err := c.CreateInvitationCode(ctx, adminID, "")
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}

	if _, err := c.CreateSelfServiceUser(ctx, SystemActorID, "failed-invite", "Failed Invite", "password123", "claimed@example.test", invitation.ID); !errors.Is(err, ErrEmailAlreadyVerified) {
		t.Fatalf("CreateSelfServiceUser error = %v, want ErrEmailAlreadyVerified", err)
	}
	state, err := c.GetInvitation(ctx, adminID, invitation.ID)
	if err != nil {
		t.Fatalf("GetInvitation: %v", err)
	}
	if state.Used || InvitationStatusAt(state, time.Now()) != InvitationStatusActive {
		t.Fatalf("invitation state after failed signup = %+v, want unused and active", state)
	}
	if _, err := c.GetUserByLogin(ctx, "failed-invite"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetUserByLogin failed signup error = %v, want ErrNotFound", err)
	}
}
