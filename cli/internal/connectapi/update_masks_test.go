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
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func TestResourceUpdateMaskCoverage(t *testing.T) {
	t.Parallel()

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
				for name := range strings.FieldsSeq(spec.fields) {
					leaf := values
					parts := strings.Split(name, ".")
					for i, part := range parts {
						field := leaf.Fields().ByName(protoreflect.Name(part))
						require.NotNil(t, field, "%s.%s", desc.FullName(), name)
						if i < len(parts)-1 {
							leaf = field.Message()
							require.NotNil(t, leaf, "%s.%s must have message parents", desc.FullName(), name)
						}
					}
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

func TestRoomMembershipEventsUpdateMask(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		value *bool
		mask  []string
		want  *bool
	}{
		{"false", new(false), []string{"show_membership_events"}, new(false)},
		{"reset", nil, []string{"show_membership_events"}, new(true)},
		{"unselected", new(false), []string{"description"}, nil},
		{"inferred", new(false), nil, new(false)},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := &apiv1.UpdateRoomRequest{RoomId: "room", ShowMembershipEvents: test.value}
			if test.mask != nil {
				request.UpdateMask = &fieldmaskpb.FieldMask{Paths: test.mask}
			}
			masked, err := normalizeUpdateMask(request)
			require.NoError(t, err)
			require.Equal(t, test.want, masked.ShowMembershipEvents)
			// Interceptor and handler both normalize; defaults must remain stable.
			require.NoError(t, applyUpdateMask(masked))
			require.Equal(t, test.want, masked.ShowMembershipEvents)
		})
	}
}

func TestUpdateMaskSelectionAndIsolation(t *testing.T) {
	t.Parallel()

	request := &apiv1.UpdateSettingsRequest{Timezone: new("Europe/Berlin"), ShareTimezone: new(true), UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"share_timezone", "share_timezone"}}}
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
	t.Parallel()

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
	require.NotNil(t, settings.Timezone)
	require.Empty(t, settings.GetTimezone())
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
	call("api.v1.UserService", "UpdateUserProfile", `{"userId":"`+env.viewer.Id+`","bio":"A short bio","updateMask":"bio"}`, 200)
	call("api.v1.UserService", "UpdateUserProfile", `{"userId":"`+env.viewer.Id+`","bio":null,"login":"!","updateMask":"bio"}`, 200)
	user, err := env.core.GetUser(env.ctx, env.viewer.Id)
	require.NoError(t, err)
	require.Empty(t, user.GetBio())
	require.Equal(t, env.viewer.GetLogin(), user.GetLogin())
	call("api.v1.UserService", "UpdateUserProfile", `{"userId":"`+env.viewer.Id+`","updateMask":"login"}`, 400)
	require.NoError(t, env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermServerManage))
	admin := "admin.v1.AdminServerService"
	call(admin, "UpdateServerConfig", `{"serverName":"Policy test","registration":{"admission":"ACCOUNT_CREATION_POLICY_INVITE_ONLY"}}`, 200)
	result := call(admin, "UpdateServerConfig", `{"description":"Updated profile","registration":{"admission":999},"updateMask":"description"}`, 200)
	config := result["config"].(map[string]any)
	require.Equal(t, "Policy test", config["serverName"])
	require.Equal(t, "ACCOUNT_CREATION_POLICY_INVITE_ONLY", config["registration"].(map[string]any)["admission"])
	result = call(admin, "UpdateServerConfig", `{"updateMask":"registration.admission"}`, 200)
	config = result["config"].(map[string]any)
	require.Equal(t, "ACCOUNT_CREATION_POLICY_OPEN", config["registration"].(map[string]any)["admission"])
	require.Equal(t, "Policy test", config["serverName"])
	require.Equal(t, "Updated profile", config["description"])
	call(admin, "UpdateServerConfig", `{"registration":{"admission":999},"updateMask":"registration.admission"}`, 400)
	call(admin, "UpdateServerConfig", `{"registration":{"admission":"ACCOUNT_CREATION_POLICY_INVITE_ONLY"},"updateMask":"registration"}`, 400)
	call(admin, "UpdateBlockedUsernames", `{"blockedUsernames":["blocked-name"],"updateMask":"blockedUsernames"}`, 200)
	result = call(admin, "UpdateBlockedUsernames", `{"blockedUsernames":[],"updateMask":"blockedUsernames"}`, 200)
	require.Empty(t, result["blockedUsernames"])
	policy := "api.v1.NotificationPolicyService"
	call(policy, "UpdateNotificationPolicy", `{"scope":{"server":{}},"overrides":{"directMessages":"NOTIFICATION_DELIVERY_MODE_OFF","replies":"NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION"},"updateMask":"directMessages,replies"}`, 200)
	result = call(policy, "UpdateNotificationPolicy", `{"scope":{"server":{}},"overrides":{"directMessages":null,"replies":999},"updateMask":"directMessages"}`, 200)
	overrides := result["policy"].(map[string]any)["policy"].(map[string]any)["overrides"].(map[string]any)
	require.NotContains(t, overrides, "directMessages")
	require.Contains(t, overrides, "replies")
}

func TestMyAccountProfileMaskBatch(t *testing.T) {
	t.Parallel()

	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	_, err := env.core.CreateUser(env.ctx, core.SystemActorID, "taken-login", "Other", "password")
	require.NoError(t, err)
	before, err := env.core.GetUser(ctx, env.viewer.Id)
	require.NoError(t, err)
	_, err = env.users.UpdateUserProfile(ctx, connect.NewRequest(&apiv1.UpdateUserProfileRequest{
		UserId:      env.viewer.Id,
		DisplayName: new("Changed name"), Login: new("taken-login"), Bio: new("New bio"),
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
	_, err = env.users.UpdateUserProfile(ctx, connect.NewRequest(&apiv1.UpdateUserProfileRequest{
		UserId:      env.viewer.Id,
		DisplayName: new("Changed name"), Login: new("new-login"), Bio: new("New bio"),
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"display_name", "login", "bio"}},
	}))
	require.NoError(t, err)
	lastChange, err = env.core.GetLastLoginChange(ctx, env.viewer.Id)
	require.NoError(t, err)
	require.False(t, lastChange.IsZero())
	_, err = env.users.UpdateUserProfile(ctx, connect.NewRequest(&apiv1.UpdateUserProfileRequest{
		UserId:      env.viewer.Id,
		DisplayName: new("Should not change"), Login: new("another-login"),
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"display_name", "login"}},
	}))
	require.Error(t, err)
	after, err = env.core.GetUser(ctx, env.viewer.Id)
	require.NoError(t, err)
	require.Equal(t, "Changed name", after.GetDisplayName())
	require.Equal(t, "new-login", after.GetLogin())
	_, err = env.users.UpdateUserProfile(ctx, connect.NewRequest(&apiv1.UpdateUserProfileRequest{
		UserId: env.viewer.Id,
		Login:  new("NEW-LOGIN"), UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"login"}},
	}))
	require.NoError(t, err, "case-only changes bypass the cooldown")
	require.NoError(t, env.core.GrantServerPermission(ctx, core.SystemActorID, core.RoleEveryone, core.PermUserManageAccounts))
	_, err = env.users.UpdateUserProfile(ctx, connect.NewRequest(&apiv1.UpdateUserProfileRequest{
		UserId: env.viewer.Id,
		Login:  new("admin-bypass"), UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"login"}},
	}))
	require.NoError(t, err, "account managers bypass the cooldown")
	afterBypass, err := env.core.GetLastLoginChange(ctx, env.viewer.Id)
	require.NoError(t, err)
	require.Equal(t, lastChange, afterBypass, "bypasses do not advance the cooldown")
}

// Two simultaneous self-service renames must not both spend the same cooldown
// allowance, and the losing request must not leave its other fields behind.
func TestMyAccountProfileConcurrentMasks(t *testing.T) {
	t.Parallel()

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
			_, err := env.users.UpdateUserProfile(ctx, connect.NewRequest(&apiv1.UpdateUserProfileRequest{
				UserId: env.viewer.Id,
				Login:  new(login), DisplayName: new(login), Bio: new(login),
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
