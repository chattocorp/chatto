package connectapi

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestViewerServiceGetViewerReturnsSelfScopedState(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.viewerService.GetViewer(env.ctx, connect.NewRequest(&apiv1.GetViewerRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetViewer code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if err := env.core.AddVerifiedEmailDirect(env.ctx, env.viewer.Id, "viewer-connect@example.com"); err != nil {
		t.Fatalf("AddVerifiedEmailDirect: %v", err)
	}
	tz := "Europe/Berlin"
	tf := evtv1.TimeFormat_TIME_FORMAT_24H
	if _, err := env.core.UpdateUserSettings(env.ctx, env.viewer.Id, core.UserSettingsInput{
		Timezone:   &tz,
		TimeFormat: &tf,
	}); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	offlineResp, err := env.viewerService.GetViewer(ctx, connect.NewRequest(&apiv1.GetViewerRequest{}))
	if err != nil {
		t.Fatalf("GetViewer offline presence: %v", err)
	}
	if offlineResp.Msg.GetUser().GetProfile().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE {
		t.Fatalf("initial viewer presence = %v, want OFFLINE", offlineResp.Msg.GetUser().GetProfile().GetPresenceStatus())
	}

	if err := env.core.SetPresence(env.ctx, env.viewer.Id, core.PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence: %v", err)
	}
	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermRoleAssign); err != nil {
		t.Fatalf("GrantServerPermission role.assign: %v", err)
	}

	resp, err := env.viewerService.GetViewer(ctx, connect.NewRequest(&apiv1.GetViewerRequest{}))
	if err != nil {
		t.Fatalf("GetViewer: %v", err)
	}
	user := resp.Msg.GetUser()
	profile := user.GetProfile()
	if profile.GetId() != env.viewer.Id || profile.GetLogin() != env.viewer.Login || profile.GetDisplayName() != env.viewer.DisplayName {
		t.Fatalf("viewer user = %+v, want id/login/display name from fixture", user)
	}
	if !user.GetHasVerifiedEmail() {
		t.Fatal("HasVerifiedEmail = false, want true")
	}
	if !user.GetHasPassword() {
		t.Fatal("HasPassword = false, want true")
	}
	if profile.GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_AWAY {
		t.Fatalf("PresenceStatus = %v, want AWAY", profile.GetPresenceStatus())
	}
	if settings := user.GetSettings(); settings.GetTimezone() != tz || settings.GetTimeFormat() != apiv1.TimeFormat_TIME_FORMAT_24_HOUR {
		t.Fatalf("settings = %+v, want timezone %q and 24-hour format", settings, tz)
	}
	if user.GetSettings().ShareTimezone == nil || user.GetSettings().GetShareTimezone() {
		t.Fatalf("share_timezone = %v, want explicit false capability", user.GetSettings().ShareTimezone)
	}
	if profile.GetTimezone() != "" {
		t.Fatalf("private profile timezone = %q, want empty", profile.GetTimezone())
	}
	if caps := resp.Msg.GetCapabilities(); !apiCapabilityGranted(caps.GetGrants(), viewerCapabilityAssignRoles) || apiCapabilityGranted(caps.GetGrants(), viewerCapabilityAdminManageUsers) {
		t.Fatalf("viewer capabilities = %+v, want role.assign true and account management false", caps.GetGrants())
	}
	if apiCapabilityGranted(resp.Msg.GetCapabilities().GetGrants(), viewerCapabilityAdminViewSystem) {
		t.Fatalf("viewer system capability = true for regular viewer, want false")
	}
	if resp.Msg.GetViewerPermissions() == nil {
		t.Fatal("ViewerPermissions = nil")
	}
	if got, want := len(resp.Msg.GetViewerPermissions().GetPermissions()), len(core.AllPermissions()); got != want {
		t.Fatalf("viewer permissions len = %d, want %d", got, want)
	}
	if apiPermissionGrantPresent(resp.Msg.GetViewerPermissions().GetPermissions(), viewerCapabilityAdminViewSystem) {
		t.Fatalf("%s should be exposed only as an owner-only capability, not as a permission grant", viewerCapabilityAdminViewSystem)
	}
	if resp.Msg.GetViewerState() == nil {
		t.Fatal("ViewerState = nil")
	}

	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "viewer-owner", "Viewer Owner", "password")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	if err := env.core.AssignOwnerRole(env.ctx, owner.Id); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}
	ownerResp, err := env.viewerService.GetViewer(withCaller(env.ctx, owner), connect.NewRequest(&apiv1.GetViewerRequest{}))
	if err != nil {
		t.Fatalf("GetViewer owner: %v", err)
	}
	if !apiCapabilityGranted(ownerResp.Msg.GetCapabilities().GetGrants(), viewerCapabilityAdminViewSystem) {
		t.Fatalf("owner system capability = false, want true")
	}
	if apiPermissionGrantPresent(ownerResp.Msg.GetViewerPermissions().GetPermissions(), viewerCapabilityAdminViewSystem) {
		t.Fatalf("%s should not be exposed as a permission grant for owners", viewerCapabilityAdminViewSystem)
	}
}

func TestMyAccountServiceUpdatesSelfProfileAndSettings(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.account.UpdateProfile(env.ctx, connect.NewRequest(&apiv1.UpdateProfileRequest{
		DisplayName: stringPtr("No Auth"),
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdateProfile code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.account.UpdateProfile(ctx, connect.NewRequest(&apiv1.UpdateProfileRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdateProfile code = %v, want invalid_argument", connect.CodeOf(err))
	}

	profileResp, err := env.account.UpdateProfile(ctx, connect.NewRequest(&apiv1.UpdateProfileRequest{
		DisplayName: stringPtr("Connect Profile"),
		Login:       stringPtr("connect-profile"),
		Bio:         stringPtr("Connect profile bio"),
	}))
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if user := profileResp.Msg.GetUser(); user.GetId() != env.viewer.Id || user.GetDisplayName() != "Connect Profile" || user.GetLogin() != "connect-profile" || user.GetBio() != "Connect profile bio" {
		t.Fatalf("updated profile = %+v, want renamed viewer", user)
	}

	tz := "Europe/Berlin"
	settingsResp, err := env.account.UpdateSettings(ctx, connect.NewRequest(&apiv1.UpdateSettingsRequest{
		Timezone:   &tz,
		TimeFormat: apiv1.TimeFormat_TIME_FORMAT_24_HOUR.Enum(),
	}))
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if settings := settingsResp.Msg.GetSettings(); settings.GetTimezone() != tz || settings.GetTimeFormat() != apiv1.TimeFormat_TIME_FORMAT_24_HOUR || settings.GetShareTimezone() {
		t.Fatalf("settings = %+v, want timezone %q and 24-hour format", settings, tz)
	}
	usersResp, err := env.users.BatchGetUsers(ctx, connect.NewRequest(&apiv1.BatchGetUsersRequest{UserIds: []string{env.viewer.Id}}))
	if err != nil {
		t.Fatalf("BatchGetUsers: %v", err)
	}
	if users := usersResp.Msg.GetUsers(); len(users) != 1 || users[0].GetUser().GetBio() != "Connect profile bio" || users[0].GetUser().GetTimezone() != "" {
		t.Fatalf("public user = %+v, want bio and private timezone", users)
	}

	share := true
	sharedResp, err := env.account.UpdateSettings(ctx, connect.NewRequest(&apiv1.UpdateSettingsRequest{ShareTimezone: &share}))
	if err != nil {
		t.Fatalf("UpdateSettings share timezone: %v", err)
	}
	if !sharedResp.Msg.GetSettings().GetShareTimezone() {
		t.Fatal("share_timezone = false, want true")
	}
	usersResp, err = env.users.BatchGetUsers(ctx, connect.NewRequest(&apiv1.BatchGetUsersRequest{UserIds: []string{env.viewer.Id}}))
	if err != nil {
		t.Fatalf("BatchGetUsers shared timezone: %v", err)
	}
	if got := usersResp.Msg.GetUsers()[0].GetUser().GetTimezone(); got != tz {
		t.Fatalf("public timezone = %q, want %q", got, tz)
	}

	clear := ""
	clearResp, err := env.account.UpdateSettings(ctx, connect.NewRequest(&apiv1.UpdateSettingsRequest{
		Timezone: &clear,
	}))
	if err != nil {
		t.Fatalf("UpdateSettings clear timezone: %v", err)
	}
	if clearResp.Msg.GetSettings().Timezone != nil {
		t.Fatalf("cleared timezone = %q, want nil", clearResp.Msg.GetSettings().GetTimezone())
	}
}

func TestMyAccountServiceSetsPassword(t *testing.T) {
	env := newConnectAPITestEnv(t)
	passwordless, err := env.core.CreateUser(env.ctx, core.SystemActorID, "connect-passwordless", "Connect Passwordless", "")
	if err != nil {
		t.Fatalf("CreateUser passwordless: %v", err)
	}
	ctx := withCaller(env.ctx, passwordless)
	freshToken, err := env.core.CreateAuthTokenWithSource(env.ctx, passwordless.Id, "test_login")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource: %v", err)
	}
	freshCtx := withBearerCredential(env.ctx, passwordless, freshToken)
	oauthToken, err := env.core.CreateAuthTokenWithSource(env.ctx, passwordless.Id, "oauth_code_exchange")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource oauth: %v", err)
	}
	oauthCtx := withBearerCredential(env.ctx, passwordless, oauthToken)

	if _, err := env.account.UpdatePassword(env.ctx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password: "newpassword456",
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdatePassword code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.account.UpdatePassword(ctx, connect.NewRequest(&apiv1.UpdatePasswordRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdatePassword code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.account.UpdatePassword(ctx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password: "short",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("short UpdatePassword code = %v, want invalid_argument", connect.CodeOf(err))
	}

	if _, err := env.account.UpdatePassword(ctx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password: "newpassword456",
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("UpdatePassword without fresh credential code = %v, want failed_precondition", connect.CodeOf(err))
	}
	if _, err := env.account.UpdatePassword(oauthCtx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password: "newpassword456",
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("UpdatePassword with OAuth token code = %v, want failed_precondition", connect.CodeOf(err))
	}
	if _, err := env.account.UpdatePassword(freshCtx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password: "newpassword456",
	})); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	if _, err := env.core.VerifyPassword(env.ctx, passwordless.Login, "newpassword456"); err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if _, err := env.account.UpdatePassword(ctx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password: "anotherpassword456",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("second UpdatePassword without current code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.account.UpdatePassword(ctx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password:        "anotherpassword456",
		CurrentPassword: "wrongpassword",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("second UpdatePassword wrong current code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.account.UpdatePassword(ctx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password:        "anotherpassword456",
		CurrentPassword: "newpassword456",
	})); err != nil {
		t.Fatalf("UpdatePassword with current: %v", err)
	}
	if _, err := env.core.VerifyPassword(env.ctx, passwordless.Login, "anotherpassword456"); err != nil {
		t.Fatalf("VerifyPassword changed: %v", err)
	}
}

func TestMyAccountServiceDeletesAvatarAndAccount(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.account.UploadAvatar(env.ctx, connect.NewRequest(&apiv1.UploadAvatarRequest{
		Image: &apiv1.ImageUpload{Image: connectAPITestPNG()},
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UploadAvatar code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.account.UploadAvatar(ctx, connect.NewRequest(&apiv1.UploadAvatarRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UploadAvatar code = %v, want invalid_argument", connect.CodeOf(err))
	}

	uploadAvatarResp, err := env.account.UploadAvatar(ctx, connect.NewRequest(&apiv1.UploadAvatarRequest{
		Image: &apiv1.ImageUpload{
			Image:       connectAPITestPNG(),
			Filename:    "avatar.png",
			ContentType: "image/png",
		},
	}))
	if err != nil {
		t.Fatalf("UploadAvatar: %v", err)
	}
	if user := uploadAvatarResp.Msg.GetUser(); user.GetId() != env.viewer.Id || user.GetAvatarUrl() == "" {
		t.Fatalf("UploadAvatar user = %+v, want viewer with avatar URL", user)
	}

	deleteAvatarResp, err := env.account.DeleteAvatar(ctx, connect.NewRequest(&apiv1.DeleteAvatarRequest{}))
	if err != nil {
		t.Fatalf("DeleteAvatar: %v", err)
	}
	if deleteAvatarResp.Msg.GetUser().GetId() != env.viewer.Id {
		t.Fatalf("DeleteAvatar user id = %q, want %q", deleteAvatarResp.Msg.GetUser().GetId(), env.viewer.Id)
	}
	if deleteAvatarResp.Msg.GetUser().AvatarUrl != nil {
		t.Fatalf("DeleteAvatar avatar URL = %q, want nil", deleteAvatarResp.Msg.GetUser().GetAvatarUrl())
	}

	tokenResp, err := env.account.RequestAccountDeletion(ctx, connect.NewRequest(&apiv1.RequestAccountDeletionRequest{}))
	if err != nil {
		t.Fatalf("RequestAccountDeletion: %v", err)
	}
	if tokenResp.Msg.GetConfirmationToken() == "" {
		t.Fatal("confirmation token is empty")
	}
	if _, err := env.account.DeleteMyAccount(ctx, connect.NewRequest(&apiv1.DeleteMyAccountRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty DeleteMyAccount code = %v, want invalid_argument", connect.CodeOf(err))
	}

	deleteResp, err := env.account.DeleteMyAccount(ctx, connect.NewRequest(&apiv1.DeleteMyAccountRequest{
		ConfirmationToken: tokenResp.Msg.GetConfirmationToken(),
	}))
	if err != nil {
		t.Fatalf("DeleteMyAccount: %v", err)
	}
	if !deleteResp.Msg.GetDeleted() {
		t.Fatal("Deleted = false, want true")
	}
}

func TestAccountDeletionRequiresDeleteSelfPermission(t *testing.T) {
	env := newConnectAPITestEnv(t)
	tokenResp, err := env.account.RequestAccountDeletion(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.RequestAccountDeletionRequest{}))
	if err != nil {
		t.Fatalf("RequestAccountDeletion before deny: %v", err)
	}
	if tokenResp.Msg.GetConfirmationToken() == "" {
		t.Fatal("confirmation token is empty before deny")
	}

	// Revoking user.delete-self must disable the whole self-service flow:
	// new token issuance and redemption of an already-issued token both refuse
	// to act (FDR-018 kill-switch).
	if err := env.core.DenyUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserDeleteSelf); err != nil {
		t.Fatalf("DenyUserPermission user.delete-self: %v", err)
	}

	if _, err := env.account.RequestAccountDeletion(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.RequestAccountDeletionRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("denied RequestAccountDeletion code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.account.DeleteMyAccount(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.DeleteMyAccountRequest{
		ConfirmationToken: tokenResp.Msg.GetConfirmationToken(),
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("denied DeleteMyAccount code = %v, want permission_denied", connect.CodeOf(err))
	}

	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserDeleteSelf); err != nil {
		t.Fatalf("GrantUserPermission user.delete-self: %v", err)
	}
	deleteResp, err := env.account.DeleteMyAccount(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.DeleteMyAccountRequest{
		ConfirmationToken: tokenResp.Msg.GetConfirmationToken(),
	}))
	if err != nil {
		t.Fatalf("DeleteMyAccount with restored permission: %v", err)
	}
	if !deleteResp.Msg.GetDeleted() {
		t.Fatal("Deleted = false, want true")
	}
}

func TestAdminUserServiceUpdatesUsersAndClearsCooldown(t *testing.T) {
	env := newConnectAPITestEnv(t)
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-user-target", "Admin User Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	regular, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-user-regular", "Admin User Regular", "password")
	if err != nil {
		t.Fatalf("CreateUser regular: %v", err)
	}

	if _, err := env.adminUsers.UpdateUser(env.ctx, connect.NewRequest(&adminv1.UpdateUserRequest{
		UserId:      target.Id,
		DisplayName: stringPtr("No Auth"),
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdateUser code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.UpdateUserPassword(env.ctx, connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   target.Id,
		Password: "newpassword456",
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdateUserPassword code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.DeleteUser(env.ctx, connect.NewRequest(&adminv1.DeleteUserRequest{
		UserId: target.Id,
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated DeleteUser code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.UpdateUser(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.UpdateUserRequest{
		UserId:      target.Id,
		DisplayName: stringPtr("Denied"),
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular UpdateUser code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.UpdateUserPassword(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   target.Id,
		Password: "newpassword456",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular UpdateUserPassword code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.DeleteUser(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.DeleteUserRequest{
		UserId: target.Id,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular DeleteUser code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, regular.Id, "admin-user-regular-renamed"); err != nil {
		t.Fatalf("UpdateUserLogin regular: %v", err)
	}
	if _, err := env.adminUsers.UpdateUser(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.UpdateUserRequest{
		UserId: regular.Id,
		Login:  stringPtr("admin-user-regular-bypass"),
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular self UpdateUser code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, regular.Id, "admin-user-regular-cooldown"); !errors.Is(err, core.ErrLoginChangeCooldown) {
		t.Fatalf("regular cooldown after denied self UpdateUser err = %v, want cooldown", err)
	}
	if _, err := env.adminUsers.ClearUsernameCooldown(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.ClearUsernameCooldownRequest{
		UserId: regular.Id,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular self ClearUsernameCooldown code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, regular.Id, "regular-still-cooldown"); !errors.Is(err, core.ErrLoginChangeCooldown) {
		t.Fatalf("regular cooldown after denied self clear err = %v, want cooldown", err)
	}

	roleAssigner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-user-role-assigner", "Admin User Role Assigner", "password")
	if err != nil {
		t.Fatalf("CreateUser role assigner: %v", err)
	}
	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, roleAssigner.Id, core.PermRoleAssign); err != nil {
		t.Fatalf("GrantUserPermission role.assign: %v", err)
	}
	if _, err := env.adminUsers.UpdateUserPassword(withCaller(env.ctx, roleAssigner), connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   target.Id,
		Password: "newpassword456",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("role.assign-only UpdateUserPassword code = %v, want permission_denied", connect.CodeOf(err))
	}

	accountManager, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-user-account-manager", "Admin User Account Manager", "password")
	if err != nil {
		t.Fatalf("CreateUser account manager: %v", err)
	}
	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, accountManager.Id, core.PermUserManageAccounts); err != nil {
		t.Fatalf("GrantUserPermission user.manage-accounts: %v", err)
	}
	if _, err := env.adminUsers.GetMember(withCaller(env.ctx, accountManager), connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_UserId{UserId: target.Id},
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("account manager GetMember code = %v, want permission_denied", connect.CodeOf(err))
	}
	accountUpdateResp, err := env.adminUsers.UpdateUser(withCaller(env.ctx, accountManager), connect.NewRequest(&adminv1.UpdateUserRequest{
		UserId:      target.Id,
		DisplayName: stringPtr("Account Managed Target"),
	}))
	if err != nil {
		t.Fatalf("account manager UpdateUser: %v", err)
	}
	if accountUpdateResp.Msg.GetUser().GetDisplayName() != "Account Managed Target" {
		t.Fatalf("account manager UpdateUser user display name = %q, want Account Managed Target", accountUpdateResp.Msg.GetUser().GetDisplayName())
	}
	if member := accountUpdateResp.Msg.GetMember(); member.GetUser().GetId() != target.Id || member.GetUser().GetDisplayName() != "Account Managed Target" {
		t.Fatalf("account manager UpdateUser member = %+v, want updated target", member)
	}
	if _, err := env.adminUsers.UpdateUserPassword(withCaller(env.ctx, accountManager), connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   target.Id,
		Password: "accountmanagerpass456",
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("account manager stale UpdateUserPassword code = %v, want failed_precondition", connect.CodeOf(err))
	}
	accountManagerToken, err := env.core.CreateAuthTokenWithSource(env.ctx, accountManager.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource account manager: %v", err)
	}
	accountManagerResp, err := env.adminUsers.UpdateUserPassword(withBearerCredential(env.ctx, accountManager, accountManagerToken), connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   target.Id,
		Password: "accountmanagerpass456",
	}))
	if err != nil {
		t.Fatalf("account manager UpdateUserPassword: %v", err)
	}
	if got := accountManagerResp.Msg.GetMember().GetUser().GetId(); got != target.Id {
		t.Fatalf("account manager password member ID = %q, want %q", got, target.Id)
	}
	if _, err := env.core.VerifyPassword(env.ctx, target.Login, "accountmanagerpass456"); err != nil {
		t.Fatalf("account-manager-set password should verify: %v", err)
	}

	admin, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-user-admin", "Admin User Admin", "password")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, admin.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	adminToken, err := env.core.CreateAuthTokenWithSource(env.ctx, admin.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource admin: %v", err)
	}
	adminCtx := withBearerCredential(env.ctx, admin, adminToken)

	if _, err := env.adminUsers.UpdateUser(adminCtx, connect.NewRequest(&adminv1.UpdateUserRequest{
		UserId: target.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdateUser code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.UpdateUserPassword(adminCtx, connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId: target.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdateUserPassword code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.DeleteUser(adminCtx, connect.NewRequest(&adminv1.DeleteUserRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty DeleteUser code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.UpdateUserPassword(adminCtx, connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   target.Id,
		Password: "short",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("short UpdateUserPassword code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.UpdateUserPassword(adminCtx, connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   admin.Id,
		Password: "newpassword456",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("self UpdateUserPassword code = %v, want permission_denied", connect.CodeOf(err))
	}
	resp, err := env.adminUsers.UpdateUser(adminCtx, connect.NewRequest(&adminv1.UpdateUserRequest{
		UserId:      target.Id,
		DisplayName: stringPtr("Managed Target"),
		Login:       stringPtr("managed-target"),
	}))
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if user := resp.Msg.GetUser(); user.GetId() != target.Id || user.GetDisplayName() != "Managed Target" || user.GetLogin() != "managed-target" {
		t.Fatalf("updated user = %+v, want managed target", user)
	}
	passwordResp, err := env.adminUsers.UpdateUserPassword(adminCtx, connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   target.Id,
		Password: "adminpassword456",
	}))
	if err != nil {
		t.Fatalf("UpdateUserPassword: %v", err)
	}
	if got := passwordResp.Msg.GetMember().GetUser().GetId(); got != target.Id {
		t.Fatalf("password member ID = %q, want %q", got, target.Id)
	}
	if _, err := env.core.VerifyPassword(env.ctx, "managed-target", "adminpassword456"); err != nil {
		t.Fatalf("admin password should verify: %v", err)
	}

	if _, err := env.core.UpdateUserLogin(env.ctx, target.Id, "target-self-rename"); err != nil {
		t.Fatalf("UpdateUserLogin target: %v", err)
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, target.Id, "target-blocked"); !errors.Is(err, core.ErrLoginChangeCooldown) {
		t.Fatalf("second self rename err = %v, want cooldown", err)
	}
	clearResp, err := env.adminUsers.ClearUsernameCooldown(adminCtx, connect.NewRequest(&adminv1.ClearUsernameCooldownRequest{
		UserId: target.Id,
	}))
	if err != nil {
		t.Fatalf("ClearUsernameCooldown: %v", err)
	}
	if !clearResp.Msg.GetCleared() {
		t.Fatal("Cleared = false, want true")
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, target.Id, "target-unblocked"); err != nil {
		t.Fatalf("self rename after cooldown clear: %v", err)
	}
	deleteResp, err := env.adminUsers.DeleteUser(adminCtx, connect.NewRequest(&adminv1.DeleteUserRequest{
		UserId: target.Id,
	}))
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if !deleteResp.Msg.GetDeleted() {
		t.Fatal("Deleted = false, want true")
	}
	if _, err := env.core.GetUser(env.ctx, target.Id); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("GetUser after DeleteUser err = %v, want not found", err)
	}
}

func TestAdminUserServiceDeleteUserDoesNotRequireFreshCredential(t *testing.T) {
	env := newConnectAPITestEnv(t)
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "stale-delete-target", "Stale Delete Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, env.viewer.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	staleToken, err := env.core.CreateAuthTokenWithSource(env.ctx, env.viewer.Id, "unknown")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource: %v", err)
	}

	deleteResp, err := env.adminUsers.DeleteUser(
		withBearerCredential(env.ctx, env.viewer, staleToken),
		connect.NewRequest(&adminv1.DeleteUserRequest{UserId: target.Id}),
	)
	if err != nil {
		t.Fatalf("DeleteUser with stale credential: %v", err)
	}
	if !deleteResp.Msg.GetDeleted() {
		t.Fatal("Deleted = false, want true")
	}
	if _, err := env.core.GetUser(env.ctx, target.Id); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("GetUser after DeleteUser err = %v, want not found", err)
	}
}

func TestAdminUserServiceDeleteUserPreservesSelfTargetContract(t *testing.T) {
	env := newConnectAPITestEnv(t)

	deleteResp, err := env.adminUsers.DeleteUser(
		withCaller(env.ctx, env.viewer),
		connect.NewRequest(&adminv1.DeleteUserRequest{UserId: env.viewer.Id}),
	)
	if err != nil {
		t.Fatalf("self DeleteUser: %v", err)
	}
	if !deleteResp.Msg.GetDeleted() {
		t.Fatal("Deleted = false, want true")
	}
	if _, err := env.core.GetUser(env.ctx, env.viewer.Id); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("GetUser after self DeleteUser err = %v, want not found", err)
	}
}

func TestAdminUserServiceListsAndGetsMembers(t *testing.T) {
	env := newConnectAPITestEnv(t)
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-member-target", "Admin Member Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	regular, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-member-regular", "Admin Member Regular", "password")
	if err != nil {
		t.Fatalf("CreateUser regular: %v", err)
	}
	admin, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-member-admin", "Admin Member Admin", "password")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, admin.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, target.Id, core.RoleModerator); err != nil {
		t.Fatalf("AssignServerRole target: %v", err)
	}
	if err := env.core.AddVerifiedEmailDirect(env.ctx, target.Id, "admin-member-target@example.test"); err != nil {
		t.Fatalf("AddVerifiedEmailDirect target: %v", err)
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, target.Id, "admin-member-target-renamed"); err != nil {
		t.Fatalf("UpdateUserLogin target: %v", err)
	}

	if _, err := env.adminUsers.ListMembers(env.ctx, connect.NewRequest(&adminv1.ListMembersRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListMembers code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.BatchGetMembers(env.ctx, connect.NewRequest(&adminv1.BatchGetMembersRequest{UserIds: []string{target.Id}})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated BatchGetMembers code = %v, want unauthenticated", connect.CodeOf(err))
	}

	regularCtx := withCaller(env.ctx, regular)
	if _, err := env.adminUsers.ListMembers(regularCtx, connect.NewRequest(&adminv1.ListMembersRequest{
		Search: "target",
		Page:   &apiv1.PageRequest{Limit: 10},
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular ListMembers code = %v, want permission denied", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.GetMember(regularCtx, connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_UserId{UserId: target.Id},
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular GetMember code = %v, want permission denied", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.BatchGetMembers(regularCtx, connect.NewRequest(&adminv1.BatchGetMembersRequest{UserIds: []string{target.Id}})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular BatchGetMembers code = %v, want permission denied", connect.CodeOf(err))
	}

	adminCtx := withCaller(env.ctx, admin)
	batchResp, err := env.adminUsers.BatchGetMembers(adminCtx, connect.NewRequest(&adminv1.BatchGetMembersRequest{
		UserIds: []string{target.Id, "missing-user", regular.Id, target.Id},
	}))
	if err != nil {
		t.Fatalf("BatchGetMembers admin: %v", err)
	}
	if got := batchResp.Msg.GetMembers(); len(got) != 2 || got[0].GetUser().GetId() != target.Id || got[1].GetUser().GetId() != regular.Id {
		t.Fatalf("BatchGetMembers members = %+v, want target,regular", got)
	}
	batchTarget := batchResp.Msg.GetMembers()[0]
	if got := batchTarget.GetRoles(); len(got) != 1 || got[0] != core.RoleModerator {
		t.Fatalf("BatchGetMembers target roles = %v, want explicit moderator only", got)
	}
	if !batchTarget.GetHasVerifiedEmail() || len(batchTarget.GetVerifiedEmails()) != 1 || batchTarget.GetVerifiedEmails()[0] != "admin-member-target@example.test" {
		t.Fatalf("BatchGetMembers emails = has:%v emails:%v, want target email", batchTarget.GetHasVerifiedEmail(), batchTarget.GetVerifiedEmails())
	}
	if batchTarget.GetLastLoginChange() == nil {
		t.Fatal("BatchGetMembers LastLoginChange is nil, want visible cooldown timestamp")
	}
	if len(batchResp.Msg.GetRoles()) == 0 {
		t.Fatal("BatchGetMembers roles are empty")
	}

	listResp, err := env.adminUsers.ListMembers(adminCtx, connect.NewRequest(&adminv1.ListMembersRequest{
		Search: "target",
		Page:   &apiv1.PageRequest{Limit: 10},
	}))
	if err != nil {
		t.Fatalf("ListMembers admin: %v", err)
	}
	if listResp.Msg.GetPage().GetTotalCount() != 1 || len(listResp.Msg.GetMembers()) != 1 {
		t.Fatalf("ListMembers returned %d/%d members, want 1/1", len(listResp.Msg.GetMembers()), listResp.Msg.GetPage().GetTotalCount())
	}
	listUser := listResp.Msg.GetMembers()[0]
	if listUser.GetUser().GetId() != target.Id {
		t.Fatalf("ListMembers user ID = %q, want %q", listUser.GetUser().GetId(), target.Id)
	}
	if got := listUser.GetRoles(); len(got) != 1 || got[0] != core.RoleModerator {
		t.Fatalf("ListMembers roles = %v, want explicit moderator only", got)
	}
	if !listUser.GetHasVerifiedEmail() || len(listUser.GetVerifiedEmails()) != 1 || listUser.GetVerifiedEmails()[0] != "admin-member-target@example.test" {
		t.Fatalf("ListMembers emails = has:%v emails:%v, want target email", listUser.GetHasVerifiedEmail(), listUser.GetVerifiedEmails())
	}
	if listUser.GetLastLoginChange() == nil {
		t.Fatal("ListMembers LastLoginChange is nil, want visible cooldown timestamp")
	}
	if len(listResp.Msg.GetRoles()) == 0 {
		t.Fatal("ListMembers roles are empty")
	}

	getResp, err := env.adminUsers.GetMember(adminCtx, connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_UserId{UserId: target.Id},
	}))
	if err != nil {
		t.Fatalf("GetMember admin: %v", err)
	}
	member := getResp.Msg.GetMember()
	if member.GetUser().GetId() != target.Id || member.GetUser().GetLogin() != "admin-member-target-renamed" {
		t.Fatalf("GetMember member = %+v, want renamed target", member)
	}
	if !member.GetHasVerifiedEmail() || len(member.GetVerifiedEmails()) != 1 || member.GetVerifiedEmails()[0] != "admin-member-target@example.test" {
		t.Fatalf("GetMember emails = has:%v emails:%v, want target email", member.GetHasVerifiedEmail(), member.GetVerifiedEmails())
	}
	if member.GetLastLoginChange() == nil {
		t.Fatal("GetMember LastLoginChange is nil, want visible cooldown timestamp")
	}
	if !getResp.Msg.GetViewerCanAssignRoles() || !getResp.Msg.GetViewerCanManageRoles() || !getResp.Msg.GetViewerCanManageUserPermissions() {
		t.Fatalf("GetMember admin capabilities = assign:%v manage:%v perms:%v, want all true", getResp.Msg.GetViewerCanAssignRoles(), getResp.Msg.GetViewerCanManageRoles(), getResp.Msg.GetViewerCanManageUserPermissions())
	}
	if len(getResp.Msg.GetRoles()) == 0 || len(getResp.Msg.GetAvailablePermissions()) == 0 {
		t.Fatalf("GetMember roles/perms empty: roles=%d perms=%d", len(getResp.Msg.GetRoles()), len(getResp.Msg.GetAvailablePermissions()))
	}
	getByLoginResp, err := env.adminUsers.GetMember(adminCtx, connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_Login{Login: "admin-member-target-renamed"},
	}))
	if err != nil {
		t.Fatalf("GetMember by login: %v", err)
	}
	if got := getByLoginResp.Msg.GetMember().GetUser().GetId(); got != target.Id {
		t.Fatalf("GetMember by login id = %q, want %q", got, target.Id)
	}
	if _, err := env.adminUsers.GetMember(adminCtx, connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_UserId{UserId: "missing-user"},
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetMember code = %v, want not found", connect.CodeOf(err))
	}
}

func TestAdminUserServiceAssignsAndRevokesRoles(t *testing.T) {
	env := newConnectAPITestEnv(t)
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-role-target", "Admin Role Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	regular, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-role-regular", "Admin Role Regular", "password")
	if err != nil {
		t.Fatalf("CreateUser regular: %v", err)
	}
	admin, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-role-admin", "Admin Role Admin", "password")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, admin.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	adminCtx := withCaller(env.ctx, admin)

	if _, err := env.adminUsers.AssignRole(env.ctx, connect.NewRequest(&adminv1.AssignRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleModerator,
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated AssignRole code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.AssignRole(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.AssignRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleModerator,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular AssignRole code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.AssignRole(adminCtx, connect.NewRequest(&adminv1.AssignRoleRequest{
		UserId: target.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty role AssignRole code = %v, want invalid_argument", connect.CodeOf(err))
	}
	roleAssigner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-role-assigner-only", "Admin Role Assigner Only", "password")
	if err != nil {
		t.Fatalf("CreateUser role assigner: %v", err)
	}
	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, roleAssigner.Id, core.PermRoleAssign); err != nil {
		t.Fatalf("GrantUserPermission role.assign: %v", err)
	}
	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, roleAssigner.Id, core.PermMessageManage); err != nil {
		t.Fatalf("GrantUserPermission message.manage: %v", err)
	}
	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, roleAssigner.Id, core.PermRoomMemberBan); err != nil {
		t.Fatalf("GrantUserPermission room.ban-member: %v", err)
	}
	roleAssignerCtx := withCaller(env.ctx, roleAssigner)
	if _, err := env.adminUsers.GetMember(roleAssignerCtx, connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_UserId{UserId: target.Id},
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("role.assign-only GetMember code = %v, want permission_denied", connect.CodeOf(err))
	}
	roleAssignerResp, err := env.adminUsers.AssignRole(roleAssignerCtx, connect.NewRequest(&adminv1.AssignRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleModerator,
	}))
	if err != nil {
		t.Fatalf("role.assign-only AssignRole: %v", err)
	}
	if !stringSliceContains(roleAssignerResp.Msg.GetMember().GetRoles(), core.RoleModerator) {
		t.Fatalf("role.assign-only AssignRole response = %+v, want assigned moderator", roleAssignerResp.Msg)
	}
	roleAssignerRevokeResp, err := env.adminUsers.RevokeRole(roleAssignerCtx, connect.NewRequest(&adminv1.RevokeRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleModerator,
	}))
	if err != nil {
		t.Fatalf("role.assign-only RevokeRole: %v", err)
	}
	if stringSliceContains(roleAssignerRevokeResp.Msg.GetMember().GetRoles(), core.RoleModerator) {
		t.Fatalf("role.assign-only RevokeRole response = %+v, want revoked moderator", roleAssignerRevokeResp.Msg)
	}
	if _, err := env.adminUsers.AssignRole(roleAssignerCtx, connect.NewRequest(&adminv1.AssignRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleOwner,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("role.assign-only owner assignment code = %v, want permission_denied", connect.CodeOf(err))
	}

	memberDetails, err := env.adminUsers.GetMember(adminCtx, connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_UserId{UserId: target.Id},
	}))
	if err != nil {
		t.Fatalf("GetMember assignment limits: %v", err)
	}
	if !memberDetails.Msg.GetRoleAssignmentLimitsEnforced() || !stringSliceContains(memberDetails.Msg.GetAssignableRoleNames(), core.RoleModerator) || stringSliceContains(memberDetails.Msg.GetAssignableRoleNames(), core.RoleOwner) || stringSliceContains(memberDetails.Msg.GetAssignableRoleNames(), core.RoleEveryone) || stringSliceContains(memberDetails.Msg.GetRevocableRoleNames(), core.RoleEveryone) {
		t.Fatalf("assignment limits = enforced:%v assignable:%v revocable:%v, want moderator but neither owner nor everyone", memberDetails.Msg.GetRoleAssignmentLimitsEnforced(), memberDetails.Msg.GetAssignableRoleNames(), memberDetails.Msg.GetRevocableRoleNames())
	}

	assignResp, err := env.adminUsers.AssignRole(adminCtx, connect.NewRequest(&adminv1.AssignRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleModerator,
	}))
	if err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if !stringSliceContains(assignResp.Msg.GetMember().GetRoles(), core.RoleModerator) {
		t.Fatalf("AssignRole response = %+v, want assigned moderator", assignResp.Msg)
	}
	roles, err := env.core.GetUserRoles(env.ctx, target.Id)
	if err != nil {
		t.Fatalf("GetUserRoles after assign: %v", err)
	}
	if len(roles) != 1 || roles[0] != core.RoleModerator {
		t.Fatalf("roles after assign = %v, want moderator", roles)
	}

	revokeResp, err := env.adminUsers.RevokeRole(adminCtx, connect.NewRequest(&adminv1.RevokeRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleModerator,
	}))
	if err != nil {
		t.Fatalf("RevokeRole: %v", err)
	}
	if stringSliceContains(revokeResp.Msg.GetMember().GetRoles(), core.RoleModerator) {
		t.Fatalf("RevokeRole response = %+v, want revoked moderator", revokeResp.Msg)
	}
	roles, err = env.core.GetUserRoles(env.ctx, target.Id)
	if err != nil {
		t.Fatalf("GetUserRoles after revoke: %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("roles after revoke = %v, want none", roles)
	}

	if _, err := env.adminUsers.RevokeRole(adminCtx, connect.NewRequest(&adminv1.RevokeRoleRequest{
		UserId:   admin.Id,
		RoleName: core.RoleAdmin,
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("self admin RevokeRole code = %v, want failed_precondition", connect.CodeOf(err))
	}
}

func TestServerServiceGetMotdAndRuntimeConfig(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config = config.ChattoConfig{
		Auth: config.AuthConfig{DirectRegistration: boolPtr(false)},
		Push: config.PushConfig{
			Enabled:         true,
			VAPIDPublicKey:  "test-public-key",
			VAPIDPrivateKey: "test-private-key",
			VAPIDSubject:    "mailto:admin@example.com",
		},
		Video: config.VideoConfig{Enabled: true},
		LiveKit: config.LiveKitConfig{
			Enabled:   true,
			URL:       "wss://livekit.example.test",
			APIKey:    "lk-key",
			APISecret: "lk-secret",
		},
	}
	if err := env.core.ConfigModel().SetServerConfig(env.ctx, core.SystemActorID, &configv1.ServerConfig{
		Motd: "Authenticated MOTD",
	}); err != nil {
		t.Fatalf("SetServerConfig: %v", err)
	}

	if _, err := env.serverState.GetMotd(env.ctx, connect.NewRequest(&apiv1.GetMotdRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetMotd code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.serverState.GetRuntimeConfig(env.ctx, connect.NewRequest(&apiv1.GetRuntimeConfigRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetRuntimeConfig code = %v, want unauthenticated", connect.CodeOf(err))
	}

	motdResp, err := env.serverState.GetMotd(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetMotdRequest{}))
	if err != nil {
		t.Fatalf("GetMotd: %v", err)
	}
	if motdResp.Msg.GetMotd() != "Authenticated MOTD" {
		t.Fatalf("MOTD = %q, want Authenticated MOTD", motdResp.Msg.GetMotd())
	}

	runtimeResp, err := env.serverState.GetRuntimeConfig(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRuntimeConfigRequest{}))
	if err != nil {
		t.Fatalf("GetRuntimeConfig: %v", err)
	}
	runtime := runtimeResp.Msg.GetRuntime()
	if !runtime.GetPushNotificationsEnabled() || runtime.GetVapidPublicKey() != "test-public-key" {
		t.Fatalf("push fields = enabled %v key %q, want true/test-public-key", runtime.GetPushNotificationsEnabled(), runtime.GetVapidPublicKey())
	}
	if !runtime.GetVideoProcessingEnabled() {
		t.Fatal("VideoProcessingEnabled = false, want true")
	}
	if runtime.GetLivekitUrl() != "wss://livekit.example.test" {
		t.Fatalf("LivekitUrl = %q, want configured URL", runtime.GetLivekitUrl())
	}
	if runtime.GetMaxUploadSize() <= 0 || runtime.GetMaxVideoUploadSize() <= 0 {
		t.Fatalf("upload sizes = %d/%d, want positive values", runtime.GetMaxUploadSize(), runtime.GetMaxVideoUploadSize())
	}
	if runtime.GetMessageEditWindowSeconds() != int32(core.MessageEditWindow/time.Second) {
		t.Fatalf("MessageEditWindowSeconds = %d, want %d", runtime.GetMessageEditWindowSeconds(), int32(core.MessageEditWindow/time.Second))
	}
}

func TestAdminServerServiceUpdateServerConfig(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.serverState.UpdateServerConfig(env.ctx, connect.NewRequest(&adminv1.UpdateServerConfigRequest{
		ServerName: stringPtr("Nope"),
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdateServerConfig code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.serverState.GetServerConfig(env.ctx, connect.NewRequest(&adminv1.GetServerConfigRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetServerConfig code = %v, want unauthenticated", connect.CodeOf(err))
	}

	if _, err := env.serverState.UpdateServerConfig(ctx, connect.NewRequest(&adminv1.UpdateServerConfigRequest{
		ServerName: stringPtr("Nope"),
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("UpdateServerConfig without permission code = %v, want permission denied", connect.CodeOf(err))
	}
	if _, err := env.serverState.GetServerConfig(ctx, connect.NewRequest(&adminv1.GetServerConfigRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("GetServerConfig without permission code = %v, want permission denied", connect.CodeOf(err))
	}

	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermServerManage); err != nil {
		t.Fatalf("GrantServerPermission manage server: %v", err)
	}

	initialResp, err := env.serverState.GetServerConfig(ctx, connect.NewRequest(&adminv1.GetServerConfigRequest{}))
	if err != nil {
		t.Fatalf("GetServerConfig: %v", err)
	}
	if initialResp.Msg.GetConfig().GetServerName() != "" || initialResp.Msg.GetPublicProfile().GetName() != "Chatto" {
		t.Fatalf("initial server config response = %+v", initialResp.Msg)
	}

	resp, err := env.serverState.UpdateServerConfig(ctx, connect.NewRequest(&adminv1.UpdateServerConfigRequest{
		ServerName:     stringPtr("Connect Settings"),
		Description:    stringPtr("Description from Connect"),
		Motd:           stringPtr("MOTD from Connect"),
		WelcomeMessage: stringPtr("Welcome from Connect"),
	}))
	if err != nil {
		t.Fatalf("UpdateServerConfig: %v", err)
	}
	publicProfile := resp.Msg.GetPublicProfile()
	if publicProfile.GetName() != "Connect Settings" ||
		publicProfile.GetDescription() != "Description from Connect" ||
		publicProfile.GetWelcomeMessage() != "Welcome from Connect" {
		t.Fatalf("updated public profile = %+v", publicProfile)
	}
	if resp.Msg.GetConfig().GetServerName() != "Connect Settings" ||
		resp.Msg.GetConfig().GetDescription() != "Description from Connect" ||
		resp.Msg.GetConfig().GetMotd() != "MOTD from Connect" ||
		resp.Msg.GetConfig().GetWelcomeMessage() != "Welcome from Connect" {
		t.Fatalf("updated config response = %+v", resp.Msg.GetConfig())
	}

	cfg := env.core.ConfigModel().GetServerConfig()
	if cfg.GetServerName() != "Connect Settings" ||
		cfg.GetDescription() != "Description from Connect" ||
		cfg.GetMotd() != "MOTD from Connect" ||
		cfg.GetWelcomeMessage() != "Welcome from Connect" {
		t.Fatalf("stored config = %+v", cfg)
	}

	if _, err := env.serverState.UpdateServerConfig(ctx, connect.NewRequest(&adminv1.UpdateServerConfigRequest{
		Description: stringPtr("Updated description only"),
	})); err != nil {
		t.Fatalf("partial UpdateServerConfig: %v", err)
	}
	cfg = env.core.ConfigModel().GetServerConfig()
	if cfg.GetServerName() != "Connect Settings" || cfg.GetDescription() != "Updated description only" {
		t.Fatalf("partial stored config = %+v", cfg)
	}
	getResp, err := env.serverState.GetServerConfig(ctx, connect.NewRequest(&adminv1.GetServerConfigRequest{}))
	if err != nil {
		t.Fatalf("GetServerConfig after partial update: %v", err)
	}
	if getResp.Msg.GetConfig().GetServerName() != "Connect Settings" ||
		getResp.Msg.GetConfig().GetDescription() != "Updated description only" ||
		getResp.Msg.GetPublicProfile().GetDescription() != "Updated description only" {
		t.Fatalf("partial server config response = %+v", getResp.Msg)
	}
}

func TestAdminServerServiceUpdatesServerBranding(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.serverState.UploadServerLogo(env.ctx, connect.NewRequest(&adminv1.UploadServerLogoRequest{
		Image: &apiv1.ImageUpload{Image: connectAPITestPNG()},
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UploadServerLogo code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.serverState.UploadServerLogo(ctx, connect.NewRequest(&adminv1.UploadServerLogoRequest{
		Image: &apiv1.ImageUpload{Image: connectAPITestPNG()},
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("UploadServerLogo without permission code = %v, want permission_denied", connect.CodeOf(err))
	}

	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermServerManage); err != nil {
		t.Fatalf("GrantServerPermission manage server: %v", err)
	}

	if _, err := env.serverState.UploadServerLogo(ctx, connect.NewRequest(&adminv1.UploadServerLogoRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UploadServerLogo code = %v, want invalid_argument", connect.CodeOf(err))
	}
	logoResp, err := env.serverState.UploadServerLogo(ctx, connect.NewRequest(&adminv1.UploadServerLogoRequest{
		Image: &apiv1.ImageUpload{
			Image:       connectAPITestPNG(),
			Filename:    "logo.png",
			ContentType: "image/png",
		},
	}))
	if err != nil {
		t.Fatalf("UploadServerLogo: %v", err)
	}
	if logoResp.Msg.GetPublicProfile().GetLogoUrl() == "" {
		t.Fatalf("UploadServerLogo public profile = %+v, want logo URL", logoResp.Msg.GetPublicProfile())
	}

	deleteLogoResp, err := env.serverState.DeleteServerLogo(ctx, connect.NewRequest(&adminv1.DeleteServerLogoRequest{}))
	if err != nil {
		t.Fatalf("DeleteServerLogo: %v", err)
	}
	if deleteLogoResp.Msg.GetPublicProfile().LogoUrl != nil {
		t.Fatalf("DeleteServerLogo logo URL = %q, want nil", deleteLogoResp.Msg.GetPublicProfile().GetLogoUrl())
	}

	if _, err := env.serverState.UploadServerBanner(ctx, connect.NewRequest(&adminv1.UploadServerBannerRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UploadServerBanner code = %v, want invalid_argument", connect.CodeOf(err))
	}
	bannerResp, err := env.serverState.UploadServerBanner(ctx, connect.NewRequest(&adminv1.UploadServerBannerRequest{
		Image: &apiv1.ImageUpload{
			Image:       connectAPITestPNG(),
			Filename:    "banner.png",
			ContentType: "image/png",
		},
	}))
	if err != nil {
		t.Fatalf("UploadServerBanner: %v", err)
	}
	if bannerResp.Msg.GetPublicProfile().GetBannerUrl() == "" {
		t.Fatalf("UploadServerBanner public profile = %+v, want banner URL", bannerResp.Msg.GetPublicProfile())
	}

	deleteBannerResp, err := env.serverState.DeleteServerBanner(ctx, connect.NewRequest(&adminv1.DeleteServerBannerRequest{}))
	if err != nil {
		t.Fatalf("DeleteServerBanner: %v", err)
	}
	if deleteBannerResp.Msg.GetPublicProfile().BannerUrl != nil {
		t.Fatalf("DeleteServerBanner banner URL = %q, want nil", deleteBannerResp.Msg.GetPublicProfile().GetBannerUrl())
	}
}

func TestAdminServerServiceSecurityConfig(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.serverState.GetServerSecurityConfig(env.ctx, connect.NewRequest(&adminv1.GetServerSecurityConfigRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetServerSecurityConfig code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.serverState.GetServerSecurityConfig(ctx, connect.NewRequest(&adminv1.GetServerSecurityConfigRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("GetServerSecurityConfig without permission code = %v, want permission denied", connect.CodeOf(err))
	}
	if _, err := env.serverState.UpdateBlockedUsernames(ctx, connect.NewRequest(&adminv1.UpdateBlockedUsernamesRequest{
		BlockedUsernames: []string{"root"},
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("UpdateBlockedUsernames without permission code = %v, want permission denied", connect.CodeOf(err))
	}

	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermServerManage); err != nil {
		t.Fatalf("GrantServerPermission manage server: %v", err)
	}

	configResp, err := env.serverState.GetServerSecurityConfig(ctx, connect.NewRequest(&adminv1.GetServerSecurityConfigRequest{}))
	if err != nil {
		t.Fatalf("GetServerSecurityConfig: %v", err)
	}
	defaultBlockedUsernames := []string{"root", "admin", "superuser", "op", "operator", "support"}
	if !slices.Equal(configResp.Msg.GetBlockedUsernames(), defaultBlockedUsernames) {
		t.Fatalf("default blocked usernames = %q, want %q", configResp.Msg.GetBlockedUsernames(), defaultBlockedUsernames)
	}

	updateResp, err := env.serverState.UpdateBlockedUsernames(ctx, connect.NewRequest(&adminv1.UpdateBlockedUsernamesRequest{
		BlockedUsernames: []string{"root", "Reserved", " admin "},
	}))
	if err != nil {
		t.Fatalf("UpdateBlockedUsernames: %v", err)
	}
	if want := []string{"root", "reserved", "admin"}; !slices.Equal(updateResp.Msg.GetBlockedUsernames(), want) {
		t.Fatalf("updated blocked usernames = %q, want %q", updateResp.Msg.GetBlockedUsernames(), want)
	}
	stored := env.core.ConfigModel().GetEffectiveBlockedUsernames()
	if stored != "root\nreserved\nadmin" {
		t.Fatalf("stored blocked usernames = %q, want root/reserved/admin", stored)
	}

	compatResp, err := env.serverState.UpdateBlockedUsernames(ctx, connect.NewRequest(&adminv1.UpdateBlockedUsernamesRequest{
		BlockedUsernames: []string{"root\nreserved"},
	}))
	if err != nil {
		t.Fatalf("compat UpdateBlockedUsernames: %v", err)
	}
	if want := []string{"root", "reserved"}; !slices.Equal(compatResp.Msg.GetBlockedUsernames(), want) {
		t.Fatalf("compat blocked usernames = %q, want %q", compatResp.Msg.GetBlockedUsernames(), want)
	}

	if _, err := env.serverState.UpdateBlockedUsernames(ctx, connect.NewRequest(&adminv1.UpdateBlockedUsernamesRequest{
		BlockedUsernames: []string{strings.Repeat("u", core.MaxLoginLength+1)},
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("oversized UpdateBlockedUsernames code = %v, want invalid argument", connect.CodeOf(err))
	}
}
