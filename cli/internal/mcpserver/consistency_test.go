package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/testutil"
)

func TestMCPRealtimeMinimumCursor(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	cfg := config.CoreConfig{SecretKey: "cursor-test-secret", Assets: config.AssetsConfig{SigningSecret: "cursor-assets"}}
	c, err := core.NewChattoCore(t.Context(), nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	startTestCore(t, c)
	viewer, err := c.CreateUser(t.Context(), core.SystemActorID, "cursor-viewer", "Cursor Viewer", "password")
	if err != nil {
		t.Fatal(err)
	}
	generation, err := c.CurrentAuthGeneration(t.Context(), viewer.Id)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := c.CreateOAuthBearerSessionForClientGrant(t.Context(), viewer.Id, "https://client.example/metadata.json", "https://chat.example/mcp", config.MCPOAuthScopes(), generation)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, err := authenticatedRequestFromToken(t, c, grant.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := c.PlanRealtimeReplay(ctx, viewer.Id, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) })
	request := func(ctx context.Context, cursor string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "https://chat.example/mcp", nil).WithContext(ctx)
		r.Header.Set("Mcp-Method", "tools/call")
		r.Header.Set("Chatto-Realtime-Minimum-Cursor", cursor)
		return r
	}
	response := httptest.NewRecorder()
	withRealtimeMinimum(c, next).ServeHTTP(response, request(ctx, plan.BoundaryCursor))
	if !called || response.Code != http.StatusNoContent {
		t.Fatal("valid barrier rejected")
	}
	called = false
	response = httptest.NewRecorder()
	withRealtimeMinimum(c, next).ServeHTTP(response, request(ctx, "invalid-cursor"))
	if called || response.Code != http.StatusBadRequest {
		t.Fatal("invalid cursor reached tool")
	}
	cold, err := core.NewChattoCore(t.Context(), nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	response = httptest.NewRecorder()
	withRealtimeMinimum(cold, next).ServeHTTP(response, request(waitCtx, plan.BoundaryCursor))
	if called || response.Code != http.StatusServiceUnavailable {
		t.Fatal("cold replica ran tool before catching up")
	}
	var failure toolErrorOutput
	decodeMCPResponse(t, response, &failure)
	if failure.Error.Outcome != "not_applied" {
		t.Fatal("barrier failure reported a tool effect")
	}
}
