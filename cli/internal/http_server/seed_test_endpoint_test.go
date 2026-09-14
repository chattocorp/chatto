//go:build test_endpoints

package http_server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/email"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
	"hmans.de/chatto/internal/pb/chatto/operator/v1/operatorv1connect"
)

func TestSeedEndpoint(t *testing.T) {
	server, client, c, _ := setupTestHTTPServerWithMailer(t)
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"users":2,"rooms":2,"messages":4,"threadReplies":1,"seed":42}`, 200},
		{`{"users":2,"rooms":2,"messages":4,"threadReplies":1,"seed":42}`, 200},
		{`{"users":-1,"rooms":2}`, 400},
		{`{`, 400},
	} {
		response, err := client.Post(server.URL+"/auth/test/seed", "application/json", strings.NewReader(tc.body))
		require.NoError(t, err)
		require.Equal(t, tc.status, response.StatusCode)
		if tc.status == http.StatusOK {
			var result core.SeedResult
			require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
			require.Len(t, result.Users, 2)
			require.Len(t, result.Rooms, 2)
			require.Len(t, result.Messages, 4)
			body, err := c.GetMessageBody(testContext(t), result.Messages[3].ID)
			require.NoError(t, err)
			require.Equal(t, result.Messages[3].Body, body)
		}
		require.NoError(t, response.Body.Close())
	}
}

func TestOperatorSeedIsLocalOnly(t *testing.T) {
	s, public := setupConnectTestServerWithConfig(t, config.ChattoConfig{})
	request := &operatorv1.SeedDataRequest{Seed: 1, Users: 2, Rooms: 1, Messages: 3, ThreadReplies: 1}
	publicClient := operatorv1connect.NewOperatorSeedServiceClient(public.Client(), public.URL+connectAPIPrefix)
	_, err := publicClient.SeedData(context.Background(), connect.NewRequest(request))
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
	local := newOperatorAPITestServer(t, s)
	localClient := operatorv1connect.NewOperatorSeedServiceClient(local.Client(), local.URL+connectAPIPrefix)
	result, err := localClient.SeedData(testContext(t), connect.NewRequest(request))
	require.NoError(t, err)
	require.Len(t, result.Msg.Users, 2)
	require.Len(t, result.Msg.Messages, 3)
	_, err = localClient.SeedData(testContext(t), connect.NewRequest(request))
	require.NoError(t, err)
}

func TestSeededUserSessionUsesRealCookieAuthentication(t *testing.T) {
	server, client, c := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.mockMailer = email.NewMockSender(true)
		s.setupConnectAPI()
	})
	scene, err := c.SeedData(testContext(t), core.SeedOptions{Seed: 42, Users: 1, Rooms: 1})
	require.NoError(t, err)
	response, err := client.Post(server.URL+"/auth/test/create-session", "application/json", strings.NewReader(`{"userId":"`+scene.Users[0].ID+`"}`))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	viewer, err := client.Post(server.URL+"/api/connect/chatto.api.v1.ViewerService/GetViewer", "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	defer viewer.Body.Close()
	require.Equal(t, http.StatusOK, viewer.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(viewer.Body).Decode(&body))
	user := body["user"].(map[string]any)["profile"].(map[string]any)
	require.Equal(t, scene.Users[0].ID, user["id"])
	for _, body := range []string{`{}`, `{"userId":"missing"}`} {
		response, err := client.Post(server.URL+"/auth/test/create-session", "application/json", strings.NewReader(body))
		require.NoError(t, err)
		require.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound}, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
}
