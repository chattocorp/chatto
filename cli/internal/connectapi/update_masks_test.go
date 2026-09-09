package connectapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestResourceUpdateMaskCoverage(t *testing.T) {
	seen := map[protoreflect.FullName]bool{}
	protoregistry.GlobalFiles.RangeFiles(func(file protoreflect.FileDescriptor) bool {
		if file.Package() != "chatto.api.v1" && file.Package() != "chatto.admin.v1" {
			return true
		}
		for i := 0; i < file.Services().Len(); i++ {
			methods := file.Services().Get(i).Methods()
			for j := 0; j < methods.Len(); j++ {
				method := methods.Get(j)
				if !strings.HasPrefix(string(method.Name()), "Update") {
					continue
				}
				desc := method.Input()
				spec, ok := updateMaskSpecs[desc.FullName()]
				require.True(t, ok, "%s needs a mask contract", method.FullName())
				seen[desc.FullName()] = true
				require.Equal(t, protoreflect.FullName("google.protobuf.FieldMask"), desc.Fields().ByName("update_mask").Message().FullName())
				values := desc
				if spec.container != "" {
					values = desc.Fields().ByName(spec.container).Message()
				}
				for _, name := range strings.Fields(spec.fields) {
					require.NotNil(t, values.Fields().ByName(protoreflect.Name(name)), "%s.%s", desc.FullName(), name)
				}
				// Every endpoint supports wildcard expansion and rejects an explicit empty mask.
				request := dynamicpb.NewMessage(desc)
				maskField := desc.Fields().ByName("update_mask")
				request.Set(maskField, protoreflect.ValueOfMessage((&fieldmaskpb.FieldMask{Paths: []string{"*"}}).ProtoReflect()))
				require.NoError(t, applyUpdateMask(request))
				mask := request.Get(maskField).Message().Interface().(*fieldmaskpb.FieldMask)
				require.Equal(t, strings.Fields(spec.fields), mask.Paths)
				request.Set(maskField, protoreflect.ValueOfMessage((&fieldmaskpb.FieldMask{}).ProtoReflect()))
				requireConnectCode(t, applyUpdateMask(request), connect.CodeInvalidArgument)
			}
		}
		return true
	})
	require.Len(t, seen, len(updateMaskSpecs), "mask registry must contain only mounted resource updates")
}

func TestUpdateMaskSelectionAndIsolation(t *testing.T) {
	request := &apiv1.UpdateSettingsRequest{Timezone: stringPtr("Europe/Berlin"), ShareTimezone: proto.Bool(true), UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"share_timezone", "share_timezone"}}}
	masked, err := normalizeUpdateMask(request)
	require.NoError(t, err)
	require.Nil(t, masked.Timezone)
	require.True(t, masked.GetShareTimezone())
	require.Equal(t, []string{"share_timezone"}, masked.UpdateMask.Paths)
	require.Equal(t, "Europe/Berlin", request.GetTimezone(), "must not mutate the caller's request")
	for _, paths := range [][]string{{"timezone.nope"}, {"update_mask"}, {"unknown"}, {"*", "timezone"}} {
		request.UpdateMask.Paths = paths
		_, err := normalizeUpdateMask(request)
		requireConnectCode(t, err, connect.CodeInvalidArgument)
	}
	_, err = normalizeUpdateMask(&apiv1.UpdateRoomRequest{RoomId: "room", UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"room_id"}}})
	requireConnectCode(t, err, connect.CodeInvalidArgument)
}

// Exercise ProtoJSON and mounted interceptors: null becomes absence, while the
// mask must survive to distinguish a reset from an untouched field. Invalid
// unselected values must be removed before protobuf and domain validation.
func TestResourceUpdatesThroughJSON(t *testing.T) {
	env := newConnectAPITestEnv(t)
	mux := http.NewServeMux()
	for _, handler := range env.api.Handlers() {
		mux.Handle(handler.ServicePath, handler.Handler)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(withCaller(r.Context(), env.viewer)))
	}))
	t.Cleanup(server.Close)
	call := func(service, method, body string, status int) map[string]any {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, server.URL+"/chatto."+service+"/"+method, bytes.NewBufferString(body))
		require.NoError(t, err)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Connect-Protocol-Version", "1")
		response, err := server.Client().Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, status, response.StatusCode, "%s", data)
		result := map[string]any{}
		require.NoError(t, json.Unmarshal(data, &result))
		return result
	}
	account := "api.v1.MyAccountService"
	call(account, "UpdateSettings", `{"timezone":"Europe/Berlin","shareTimezone":true,"timeFormat":"TIME_FORMAT_24_HOUR","updateMask":"timezone,shareTimezone,timeFormat"}`, 200)
	call(account, "UpdateSettings", `{"timezone":null,"timeFormat":999,"updateMask":"timezone"}`, 200)
	settings, err := env.core.GetUserSettings(env.ctx, env.viewer.Id)
	require.NoError(t, err)
	require.Nil(t, settings.Timezone)
	require.True(t, settings.GetShareTimezone())
	require.EqualValues(t, 2, settings.GetTimeFormat())
	call(account, "UpdateSettings", `{"shareTimezone":false,"updateMask":"shareTimezone"}`, 200)
	call(account, "UpdateSettings", `{"updateMask":"timeFormat"}`, 200)
	settings, err = env.core.GetUserSettings(env.ctx, env.viewer.Id)
	require.NoError(t, err)
	require.False(t, settings.GetShareTimezone())
	require.EqualValues(t, 0, settings.GetTimeFormat())
	call(account, "UpdateSettings", `{"timezone":"UTC"}`, 200) // inferred mask
	call(account, "UpdateSettings", `{"timezone":"UTC","updateMask":""}`, 400)
	call(account, "UpdateSettings", `{"timeFormat":999,"updateMask":"timeFormat"}`, 400)
	call(account, "UpdateProfile", `{"bio":"A short bio","updateMask":"bio"}`, 200)
	call(account, "UpdateProfile", `{"bio":null,"login":"!","updateMask":"bio"}`, 200)
	user, err := env.core.GetUser(env.ctx, env.viewer.Id)
	require.NoError(t, err)
	require.Empty(t, user.GetBio())
	require.Equal(t, env.viewer.GetLogin(), user.GetLogin())
	call(account, "UpdateProfile", `{"updateMask":"login"}`, 400)
	require.NoError(t, env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermServerManage))
	admin := "admin.v1.AdminServerService"
	call(admin, "UpdateBlockedUsernames", `{"blockedUsernames":["blocked-name"],"updateMask":"blockedUsernames"}`, 200)
	result := call(admin, "UpdateBlockedUsernames", `{"blockedUsernames":[],"updateMask":"blockedUsernames"}`, 200)
	require.Empty(t, result["blockedUsernames"])
	policy := "api.v1.NotificationPolicyService"
	call(policy, "UpdateNotificationPolicy", `{"scope":{"server":{}},"overrides":{"directMessages":"NOTIFICATION_DELIVERY_MODE_OFF","replies":"NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION"},"updateMask":"directMessages,replies"}`, 200)
	result = call(policy, "UpdateNotificationPolicy", `{"scope":{"server":{}},"overrides":{"directMessages":null,"replies":999},"updateMask":"directMessages"}`, 200)
	overrides := result["policy"].(map[string]any)["policy"].(map[string]any)["overrides"].(map[string]any)
	require.NotContains(t, overrides, "directMessages")
	require.Contains(t, overrides, "replies")
}

func TestMyAccountProfileMaskBatch(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	_, err := env.core.CreateUser(env.ctx, core.SystemActorID, "taken-login", "Other", "password")
	require.NoError(t, err)
	before, err := env.core.GetUser(ctx, env.viewer.Id)
	require.NoError(t, err)
	_, err = env.account.UpdateProfile(ctx, connect.NewRequest(&apiv1.UpdateProfileRequest{
		DisplayName: stringPtr("Changed name"), Login: stringPtr("taken-login"), Bio: stringPtr("New bio"),
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"display_name", "login", "bio"}},
	}))
	require.Error(t, err)
	after, err := env.core.GetUser(ctx, env.viewer.Id)
	require.NoError(t, err)
	require.Equal(t, before.GetDisplayName(), after.GetDisplayName(), "failed login change must not commit the display name")
	require.Equal(t, before.GetBio(), after.GetBio())
	lastChange, err := env.core.GetLastLoginChange(ctx, env.viewer.Id)
	require.NoError(t, err)
	require.True(t, lastChange.IsZero())
	_, err = env.account.UpdateProfile(ctx, connect.NewRequest(&apiv1.UpdateProfileRequest{
		DisplayName: stringPtr("Changed name"), Login: stringPtr("new-login"), Bio: stringPtr("New bio"),
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"display_name", "login", "bio"}},
	}))
	require.NoError(t, err)
	lastChange, err = env.core.GetLastLoginChange(ctx, env.viewer.Id)
	require.NoError(t, err)
	require.False(t, lastChange.IsZero())
	_, err = env.account.UpdateProfile(ctx, connect.NewRequest(&apiv1.UpdateProfileRequest{
		DisplayName: stringPtr("Should not change"), Login: stringPtr("another-login"),
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"display_name", "login"}},
	}))
	require.Error(t, err)
	after, err = env.core.GetUser(ctx, env.viewer.Id)
	require.NoError(t, err)
	require.Equal(t, "Changed name", after.GetDisplayName())
	require.Equal(t, "new-login", after.GetLogin())
	_, err = env.account.UpdateProfile(ctx, connect.NewRequest(&apiv1.UpdateProfileRequest{
		Login: stringPtr("NEW-LOGIN"), UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"login"}},
	}))
	require.NoError(t, err, "case-only changes bypass the cooldown")
	require.NoError(t, env.core.GrantServerPermission(ctx, core.SystemActorID, core.RoleEveryone, core.PermUserManageAccounts))
	_, err = env.account.UpdateProfile(ctx, connect.NewRequest(&apiv1.UpdateProfileRequest{
		Login: stringPtr("admin-bypass"), UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"login"}},
	}))
	require.NoError(t, err, "account managers bypass the cooldown")
	afterBypass, err := env.core.GetLastLoginChange(ctx, env.viewer.Id)
	require.NoError(t, err)
	require.Equal(t, lastChange, afterBypass, "bypasses do not advance the cooldown")
}

// Two simultaneous self-service renames must not both spend the same cooldown
// allowance, and the losing request must not leave its other fields behind.
func TestMyAccountProfileConcurrentMasks(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	start := make(chan struct{})
	type result struct {
		login string
		err   error
	}
	results := make(chan result, 2)
	for _, login := range []string{"first-rename", "second-rename"} {
		go func() {
			<-start
			_, err := env.account.UpdateProfile(ctx, connect.NewRequest(&apiv1.UpdateProfileRequest{
				Login: stringPtr(login), DisplayName: stringPtr(login), Bio: stringPtr(login),
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"login", "display_name", "bio"}},
			}))
			results <- result{login, err}
		}()
	}
	close(start)
	var winner string
	for range 2 {
		result := <-results
		if result.err == nil {
			require.Empty(t, winner, "only one rename can succeed")
			winner = result.login
		}
	}
	require.NotEmpty(t, winner)
	user, err := env.core.GetUser(ctx, env.viewer.Id)
	require.NoError(t, err)
	require.Equal(t, winner, user.GetLogin())
	require.Equal(t, winner, user.GetDisplayName())
	require.Equal(t, winner, user.GetBio())
}
