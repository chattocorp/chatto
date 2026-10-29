package core

import (
	"testing"

	configv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/config/v1"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/stretchr/testify/require"
)

func TestRegistrationAdmissionProjectionSnapshot(t *testing.T) {
	t.Parallel()
	p, model := newConfigProjectionUnderModel()
	require.Equal(t, evtv1.RegistrationAdmission_REGISTRATION_ADMISSION_OPEN, model.RegistrationAdmission())
	require.NoError(t, p.Restore(nil))
	require.Equal(t, evtv1.RegistrationAdmission_REGISTRATION_ADMISSION_OPEN, model.RegistrationAdmission())
	for _, admission := range []evtv1.RegistrationAdmission{
		evtv1.RegistrationAdmission_REGISTRATION_ADMISSION_INVITE_ONLY,
		99, // Preserve unknown values so account creation can fail closed.
		evtv1.RegistrationAdmission_REGISTRATION_ADMISSION_OPEN,
	} {
		require.NoError(t, p.Apply(&evtv1.Event{Event: &evtv1.Event_ServerRegistrationAdmissionChanged{
			ServerRegistrationAdmissionChanged: &evtv1.ServerRegistrationAdmissionChangedEvent{Admission: admission},
		}}, 1))
		payload, err := p.Snapshot()
		require.NoError(t, err)
		restored, restoredModel := newConfigProjectionUnderModel()
		require.NoError(t, restored.Restore(payload))
		require.Equal(t, admission, restoredModel.RegistrationAdmission())
	}
}

func TestRegistrationAdmissionPermissionsAndCompletion(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	// Fresh servers accept self-service accounts without an invitation.
	member, err := c.CreateVerifiedUserWithInvitation(ctx, SystemActorID, "policy-member", "Member", "password123", "member@example.test", "")
	require.NoError(t, err)
	inviteOnly := evtv1.RegistrationAdmission_REGISTRATION_ADMISSION_INVITE_ONLY
	_, err = c.UpdateServerConfig(ctx, member.Id, ServerConfigUpdateInput{RegistrationAdmission: &inviteOnly})
	require.ErrorIs(t, err, ErrPermissionDenied)
	adminID := invitationAdmin(t, c)
	_, err = c.CreateVerifiedUserWithInvitation(ctx, SystemActorID, "blocked-signup", "Blocked", "password123", "blocked@example.test", "")
	require.ErrorIs(t, err, ErrInvitationInvalid)
	_, err = c.GetUserByLogin(ctx, "blocked-signup")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = c.UpdateServerConfig(ctx, adminID, ServerConfigUpdateInput{RegistrationAdmission: new(evtv1.RegistrationAdmission(99))})
	require.ErrorIs(t, err, ErrInvalidArgument)
	// Existing accounts remain available, and trusted operator creation is exempt.
	_, err = c.GetUser(ctx, member.Id)
	require.NoError(t, err)
	_, err = c.CreateUser(ctx, SystemActorID, "operator-created", "Operator Created", "password123")
	require.NoError(t, err)
	// A policy written by a newer server must never silently permit signups.
	require.NoError(t, c.ConfigModel().SetServerConfig(ctx, SystemActorID, &configv1.ServerConfig{RegistrationAdmission: 99}))
	_, err = c.CreateVerifiedUserWithInvitation(ctx, SystemActorID, "unknown-policy", "Unknown", "password123", "unknown@example.test", "")
	require.ErrorIs(t, err, ErrInvitationInvalid)
}

func TestRegistrationAdmissionRecomposesAfterPolicyChange(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	adminID := invitationAdmin(t, c)
	invitation, err := c.CreateInvitationCode(ctx, adminID, nil, "")
	require.NoError(t, err)
	attempts := 0
	_, err = c.createUserWithOptions(ctx, SystemActorID, "policy-retry", "Policy Retry", "password123", userCreationOptions{
		selfService: true, invitationID: invitation.ID, verifiedEmail: "retry@example.test",
		authorize: func() error {
			attempts++
			if attempts == 1 {
				// Commit after account creation captured its OCC sequence. The first
				// append must conflict and rebuild the batch under the new policy.
				_, err := c.UpdateServerConfig(ctx, adminID, ServerConfigUpdateInput{
					RegistrationAdmission: new(evtv1.RegistrationAdmission_REGISTRATION_ADMISSION_OPEN),
				})
				return err
			}
			return nil
		},
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, attempts, 2)
	state, err := c.GetInvitation(ctx, adminID, invitation.ID)
	require.NoError(t, err)
	require.False(t, state.Used, "open registration must not consume a previously supplied code")
}
