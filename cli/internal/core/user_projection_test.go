package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func userEvent(id string, ts time.Time, event *corev1.Event) *corev1.Event {
	event.Id = id
	event.CreatedAt = timestamppb.New(ts)
	return event
}

func accountCreated(userID, login, displayName string) *corev1.Event {
	return &corev1.Event{Event: &corev1.Event_UserAccountCreated{UserAccountCreated: &corev1.UserAccountCreatedEvent{
		UserId:      userID,
		Login:       login,
		DisplayName: displayName,
	}}}
}

func loginChanged(userID, login string) *corev1.Event {
	return &corev1.Event{Event: &corev1.Event_UserLoginChanged{UserLoginChanged: &corev1.UserLoginChangedEvent{
		UserId: userID,
		Login:  login,
	}}}
}

func loginCooldownStarted(userID string) *corev1.Event {
	return &corev1.Event{Event: &corev1.Event_UserLoginCooldownStarted{UserLoginCooldownStarted: &corev1.UserLoginCooldownStartedEvent{
		UserId: userID,
	}}}
}

func displayNameChanged(userID, displayName string) *corev1.Event {
	return &corev1.Event{Event: &corev1.Event_UserDisplayNameChanged{UserDisplayNameChanged: &corev1.UserDisplayNameChangedEvent{
		UserId:      userID,
		DisplayName: displayName,
	}}}
}

func TestUserProjection_AccountProfileAndLogin(t *testing.T) {
	p := NewUserProjection()
	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)

	require.NoError(t, p.Apply(userEvent("E1", createdAt, accountCreated("U1", "Alice", "Alice A.")), 1))

	got, ok := p.Get("U1")
	require.True(t, ok)
	require.Equal(t, "U1", got.GetId())
	require.Equal(t, "Alice", got.GetLogin())
	require.Equal(t, "Alice A.", got.GetDisplayName())
	require.True(t, got.GetCreatedAt().AsTime().Equal(createdAt))

	byLogin, ok := p.GetByLogin("alice")
	require.True(t, ok)
	require.Equal(t, "U1", byLogin.GetId())
	require.Equal(t, 1, p.Count())
}

func TestUserProjection_LoginCooldownUsesEnvelopeTime(t *testing.T) {
	p := NewUserProjection()
	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	changedAt := createdAt.Add(5 * time.Minute)

	require.NoError(t, p.Apply(userEvent("E1", createdAt, accountCreated("U1", "Alice", "Alice A.")), 1))
	require.True(t, p.LoginChangedAt("U1").IsZero())

	require.NoError(t, p.Apply(userEvent("E3", changedAt, loginChanged("U1", "Alice2")), 3))
	require.True(t, p.LoginChangedAt("U1").IsZero())
	require.False(t, p.LoginExists("Alice"))
	require.True(t, p.LoginExists("alice2"))

	require.NoError(t, p.Apply(userEvent("E4", changedAt, loginCooldownStarted("U1")), 4))
	require.True(t, p.LoginChangedAt("U1").Equal(changedAt))

	require.NoError(t, p.Apply(&corev1.Event{
		Id:    "E5",
		Event: &corev1.Event_UserLoginCooldownCleared{UserLoginCooldownCleared: &corev1.UserLoginCooldownClearedEvent{UserId: "U1"}},
	}, 5))
	require.True(t, p.LoginChangedAt("U1").IsZero())
}

func TestUserProjection_VerifiedEmailAvatarOIDCAndDelete(t *testing.T) {
	p := NewUserProjection()
	createdAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	verifiedAt := createdAt.Add(time.Hour)

	require.NoError(t, p.Apply(userEvent("E1", createdAt, accountCreated("U1", "Alice", "Alice A.")), 1))
	require.NoError(t, p.Apply(&corev1.Event{
		Id:        "E3",
		CreatedAt: timestamppb.New(verifiedAt),
		Event: &corev1.Event_UserVerifiedEmailAdded{UserVerifiedEmailAdded: &corev1.UserVerifiedEmailAddedEvent{
			UserId: "U1",
			Email:  "Alice@Example.com",
		}},
	}, 3))
	require.NoError(t, p.Apply(&corev1.Event{
		Id: "E4",
		Event: &corev1.Event_UserAvatarSet{UserAvatarSet: &corev1.UserAvatarSetEvent{
			UserId: "U1",
			Avatar: &corev1.DeprecatedAsset{Asset: &corev1.DeprecatedAsset_S3{S3: &corev1.S3Asset{Key: "avatars/U1"}}},
		}},
	}, 4))
	require.NoError(t, p.Apply(&corev1.Event{
		Id: "E5",
		Event: &corev1.Event_UserOidcSubjectLinked{UserOidcSubjectLinked: &corev1.UserOIDCSubjectLinkedEvent{
			UserId:  "U1",
			Issuer:  "https://issuer.example",
			Subject: "subject-1",
		}},
	}, 5))

	emails := p.VerifiedEmails("U1")
	require.Len(t, emails, 1)
	require.Equal(t, "Alice@Example.com", emails[0].Email)
	require.True(t, emails[0].VerifiedAt.Equal(verifiedAt))
	byEmail, ok := p.GetByEmail("alice@example.com")
	require.True(t, ok)
	require.Equal(t, "U1", byEmail.GetId())
	byOIDC, ok := p.GetByOIDCSubject("https://issuer.example", "subject-1")
	require.True(t, ok)
	require.Equal(t, "U1", byOIDC.GetId())
	avatar, ok := p.Avatar("U1")
	require.True(t, ok)
	require.Equal(t, "avatars/U1", avatar.GetS3().GetKey())

	require.NoError(t, p.Apply(&corev1.Event{
		Id:    "E6",
		Event: &corev1.Event_UserAvatarCleared{UserAvatarCleared: &corev1.UserAvatarClearedEvent{UserId: "U1"}},
	}, 6))
	_, ok = p.Avatar("U1")
	require.False(t, ok)

	require.NoError(t, p.Apply(&corev1.Event{
		Id:    "E7",
		Event: &corev1.Event_UserAccountDeleted{UserAccountDeleted: &corev1.UserAccountDeletedEvent{UserId: "U1"}},
	}, 7))
	_, ok = p.Get("U1")
	require.False(t, ok)
	require.False(t, p.LoginExists("alice"))
	require.False(t, p.EmailClaimed("Alice@Example.com"))
	_, ok = p.GetByOIDCSubject("https://issuer.example", "subject-1")
	require.False(t, ok)
}
