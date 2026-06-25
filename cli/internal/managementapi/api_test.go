package managementapi

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/events"
	managementv1 "hmans.de/chatto/internal/pb/chatto/management/v1"
	"hmans.de/chatto/internal/pb/chatto/management/v1/managementv1connect"
	"hmans.de/chatto/internal/testutil"
)

func TestUserAdminServiceCreateUpdateAndSetPassword(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := context.Background()
	chattoCore, err := core.NewChattoCore(ctx, nc, config.CoreConfig{})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	startCoreServices(t, chattoCore)

	service := &userAdminService{api: New(chattoCore)}
	createResp, err := service.CreateUser(ctx, connect.NewRequest(&managementv1.CreateUserRequest{
		Login:         "managed-user",
		DisplayName:   "Managed User",
		Password:      "password123",
		VerifiedEmail: "managed@example.com",
	}))
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	user := createResp.Msg.GetUser()
	if user.GetId() == "" || user.GetLogin() != "managed-user" || user.GetDisplayName() != "Managed User" {
		t.Fatalf("created user = %+v", user)
	}
	if hasEmail, err := chattoCore.HasVerifiedEmail(ctx, user.GetId()); err != nil || !hasEmail {
		t.Fatalf("HasVerifiedEmail = %v, %v; want true, nil", hasEmail, err)
	}
	assertLatestUserEventActor(t, chattoCore, ctx, user.GetId(), events.EventUserAccountCreated, OperatorActorID)
	assertLatestUserEventActor(t, chattoCore, ctx, user.GetId(), events.EventUserVerifiedEmailAdded, OperatorActorID)

	updateResp, err := service.UpdateUser(ctx, connect.NewRequest(&managementv1.UpdateUserRequest{
		User:        &managementv1.UserSelector{Selector: &managementv1.UserSelector_Login{Login: "managed-user"}},
		Login:       "managed-renamed",
		DisplayName: "Managed Renamed",
	}))
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if got := updateResp.Msg.GetUser(); got.GetLogin() != "managed-renamed" || got.GetDisplayName() != "Managed Renamed" {
		t.Fatalf("updated user = %+v", got)
	}
	assertLatestUserEventActor(t, chattoCore, ctx, user.GetId(), events.EventUserDisplayNameChanged, OperatorActorID)
	assertLatestUserEventActor(t, chattoCore, ctx, user.GetId(), events.EventUserLoginChanged, OperatorActorID)

	if _, err := service.SetUserPassword(ctx, connect.NewRequest(&managementv1.SetUserPasswordRequest{
		User:     &managementv1.UserSelector{Selector: &managementv1.UserSelector_UserId{UserId: user.GetId()}},
		Password: "newpassword123",
	})); err != nil {
		t.Fatalf("SetUserPassword: %v", err)
	}
	if _, err := chattoCore.VerifyPassword(ctx, "managed-renamed", "password123"); err == nil {
		t.Fatal("old password still verifies")
	}
	if verified, err := chattoCore.VerifyPassword(ctx, "managed-renamed", "newpassword123"); err != nil || verified.GetId() != user.GetId() {
		t.Fatalf("new password VerifyPassword = %+v, %v", verified, err)
	}
	assertLatestUserEventActor(t, chattoCore, ctx, user.GetId(), events.EventUserPasswordHashChanged, OperatorActorID)
}

func TestUserAdminServiceCreateRejectsDuplicateVerifiedEmail(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := context.Background()
	chattoCore, err := core.NewChattoCore(ctx, nc, config.CoreConfig{})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	startCoreServices(t, chattoCore)

	service := &userAdminService{api: New(chattoCore)}
	if _, err := service.CreateUser(ctx, connect.NewRequest(&managementv1.CreateUserRequest{
		Login:         "email-owner",
		DisplayName:   "Email Owner",
		VerifiedEmail: "shared@example.com",
	})); err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}

	_, err = service.CreateUser(ctx, connect.NewRequest(&managementv1.CreateUserRequest{
		Login:         "email-conflict",
		DisplayName:   "Email Conflict",
		VerifiedEmail: "shared@example.com",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("duplicate email code = %v, want invalid_argument; err = %v", connect.CodeOf(err), err)
	}
	if _, err := chattoCore.GetUserByLogin(ctx, "email-conflict"); err == nil {
		t.Fatal("duplicate email request created a user")
	}
}

func TestManagementServiceIsNotMountedOnPublicConnectAPI(t *testing.T) {
	publicAPI := connectapi.New(nil, config.ChattoConfig{}, "")
	for _, handler := range publicAPI.Handlers() {
		if handler.ServicePath == "/"+managementv1connect.UserAdminServiceName+"/" {
			t.Fatalf("management service mounted on public Connect API at %s", handler.ServicePath)
		}
	}
}

func assertLatestUserEventActor(t *testing.T, c *core.ChattoCore, ctx context.Context, userID, eventName, actorID string) {
	t.Helper()
	published, _, err := c.EventPublisher.SubjectEvents(ctx, events.UserAggregate(userID).Subject(eventName))
	if err != nil {
		t.Fatalf("SubjectEvents(%s): %v", eventName, err)
	}
	if len(published) == 0 {
		t.Fatalf("SubjectEvents(%s) returned no events", eventName)
	}
	if got := published[len(published)-1].GetActorId(); got != actorID {
		t.Fatalf("latest %s actor = %q, want %q", eventName, got, actorID)
	}
}

func startCoreServices(t *testing.T, c *core.ChattoCore) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("core.Run did not stop within timeout")
		}
	})
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bootCancel()
	if err := c.WaitForBoot(bootCtx); err != nil {
		t.Fatalf("WaitForBoot: %v", err)
	}
}
