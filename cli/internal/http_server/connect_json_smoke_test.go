package http_server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
)

// TestConnectJSONIntegrationSmoke exercises a client workflow through the real
// HTTP auth and Connect routes. Keep procedure names, JSON fields, enum values,
// and error codes literal so generated clients cannot hide contract changes.
func TestConnectJSONIntegrationSmoke(t *testing.T) {
	s, server := setupConnectTestServer(t, config.AuthConfig{})
	ctx := context.Background()
	// Provision the human credential out of band, as an integration would
	// obtain it from OAuth. Every workflow operation below uses HTTP JSON.
	owner, err := s.core.CreateUser(ctx, core.SystemActorID, "json-smoke", "JSON Smoke", "password")
	require.NoError(t, err)
	token, err := s.core.CreateAuthToken(ctx, owner.GetId())
	require.NoError(t, err)

	call := func(credential, procedure, body string, status int) map[string]any {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/connect/"+procedure, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Connect-Protocol-Version", "1")
		if credential != "" {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		resp, err := server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		// Do not include response bodies: credential-creation responses contain secrets.
		require.Equal(t, status, resp.StatusCode, "procedure %s", procedure)
		require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
		var result map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		return result
	}
	object := func(value any) map[string]any {
		t.Helper()
		result, ok := value.(map[string]any)
		require.True(t, ok, "expected JSON object")
		return result
	}
	const viewer = "chatto.api.v1.ViewerService/GetViewer"
	const account = "chatto.api.v1.MyAccountService/"
	const bots = "chatto.api.v1.BotService/"

	discovery := call("", "chatto.discovery.v1.ServerDiscoveryService/GetServer", "{}", 200)
	require.Equal(t, "1.2.3", object(discovery["profile"])["version"])
	require.Equal(t, "/oauth/authorize", object(discovery["login"])["authorizeUrl"])
	require.Equal(t, "unauthenticated", call("", viewer, "{}", 401)["code"])
	require.Equal(t, "unauthenticated", call("invalid-credential", viewer, "{}", 401)["code"])
	current := call(token, viewer, "{}", 200)
	require.Equal(t, owner.GetId(), object(object(current["user"])["profile"])["id"])

	// Create out of sort order; authenticate with a credential returned in JSON.
	for _, login := range []string{"smoke_c_bot", "smoke_a_bot", "smoke_b_bot"} {
		created := call(token, bots+"CreateBot", `{"login":"`+login+`","displayName":"Smoke Bot"}`, 200)
		user := object(object(created["bot"])["user"])
		require.Equal(t, login, user["login"])
		key, ok := created["apiKey"].(string)
		require.True(t, ok)
		require.NotEmpty(t, key)
		authenticated := call(key, viewer, "{}", 200)
		require.Equal(t, user["id"], object(object(authenticated["user"])["profile"])["id"])
	}

	// A comma-separated camelCase FieldMask can reset null/false while
	// preserving an unselected value. Verify through a separate public read.
	call(token, account+"UpdateSettings", `{"timezone":"Europe/Berlin","shareTimezone":true,"timeFormat":"TIME_FORMAT_24_HOUR","updateMask":"timezone,shareTimezone,timeFormat"}`, 200)
	call(token, account+"UpdateSettings", `{"timezone":null,"shareTimezone":false,"updateMask":"timezone,shareTimezone"}`, 200)
	current = call(token, viewer, "{}", 200)
	settings := object(object(current["user"])["settings"])
	require.NotContains(t, settings, "timezone")
	require.Equal(t, false, settings["shareTimezone"])
	require.Equal(t, "TIME_FORMAT_24_HOUR", settings["timeFormat"])

	first := call(token, bots+"ListBots", `{"search":"smoke_","page":{"limit":2}}`, 200)
	second := call(token, bots+"ListBots", `{"search":"smoke_","page":{"limit":2,"offset":2}}`, 200)
	var logins []string
	for _, page := range []map[string]any{first, second} {
		rows, ok := page["bots"].([]any)
		require.True(t, ok)
		for _, row := range rows {
			logins = append(logins, object(object(row)["user"])["login"].(string))
		}
		// ProtoJSON encodes int64 counts as decimal strings.
		require.Equal(t, "3", object(page["page"])["totalCount"])
	}
	require.Len(t, first["bots"], 2)
	require.Len(t, second["bots"], 1)
	require.Equal(t, []string{"smoke_a_bot", "smoke_b_bot", "smoke_c_bot"}, logins)
	require.Equal(t, true, object(first["page"])["hasMore"])
	// ProtoJSON omits non-optional false scalars.
	require.NotContains(t, object(second["page"]), "hasMore")

	require.Equal(t, "invalid_argument", call(token, account+"UpdateSettings", `{"updateMask":"unknownField"}`, 400)["code"])
	require.Equal(t, "invalid_argument", call(token, bots+"ListBots", `{"page":{"limit":501}}`, 400)["code"])
	require.Equal(t, "not_found", call(token, "chatto.api.v1.UserService/GetUser", `{"userId":"missing-user"}`, 404)["code"])
	require.Equal(t, "permission_denied", call(token, "chatto.admin.v1.AdminServerService/UpdateBlockedUsernames", `{"blockedUsernames":["blocked"],"updateMask":"blockedUsernames"}`, 403)["code"])
	call(token, account+"UpdateProfile", `{"login":"json-smoke-renamed","updateMask":"login"}`, 200)
	require.Equal(t, "failed_precondition", call(token, account+"UpdateProfile", `{"login":"json-smoke-again","updateMask":"login"}`, 400)["code"])
}
