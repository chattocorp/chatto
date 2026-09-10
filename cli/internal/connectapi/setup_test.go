package connectapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	authv1 "hmans.de/chatto/internal/pb/chatto/auth/v1"
	"hmans.de/chatto/internal/pb/chatto/auth/v1/authv1connect"
	discoveryv1 "hmans.de/chatto/internal/pb/chatto/discovery/v1"
	"hmans.de/chatto/internal/pb/chatto/discovery/v1/discoveryv1connect"
	"hmans.de/chatto/internal/testutil"
)

func TestServerSetupPublicAPI(t *testing.T) {
	for _, skip := range []bool{false, true} {
		t.Run(map[bool]string{false: "enabled", true: "disabled"}[skip], func(t *testing.T) {
			_, nc := testutil.StartNATS(t)
			ctx := context.Background()
			cfg := config.ChattoConfig{Core: config.CoreConfig{SkipSetupWizard: skip, SecretKey: "test-core-secret", Assets: config.AssetsConfig{SigningSecret: "test-assets-secret"}}}
			c, err := core.NewChattoCore(ctx, nc, cfg.Core)
			if err != nil {
				t.Fatal(err)
			}
			startConnectAPITestCore(t, c)
			mux := http.NewServeMux()
			for _, handler := range New(c, cfg, "0.5.0").Handlers() {
				mux.Handle(handler.ServicePath, handler.Handler)
			}
			server := httptest.NewServer(mux)
			t.Cleanup(server.Close)
			discovery := discoveryv1connect.NewServerDiscoveryServiceClient(server.Client(), server.URL)
			info, err := discovery.GetServer(ctx, connect.NewRequest(&discoveryv1.GetServerRequest{}))
			if err != nil {
				t.Fatal(err)
			}
			if info.Msg.SetupRequired == skip {
				t.Fatal("incorrect setup discovery")
			}
			client := authv1connect.NewServerSetupServiceClient(server.Client(), server.URL)
			_, err = client.CompleteSetup(ctx, connect.NewRequest(&authv1.CompleteSetupRequest{}))
			if skip {
				requireConnectCode(t, err, connect.CodeFailedPrecondition)
				return
			}
			requireConnectCode(t, err, connect.CodeInvalidArgument)
			_, err = client.CompleteSetup(ctx, connect.NewRequest(&authv1.CompleteSetupRequest{ServerName: "API community", Login: "founder", DisplayName: "Founder", Password: "correct-password"}))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.VerifyPassword(ctx, "founder", "correct-password"); err != nil {
				t.Fatal(err)
			}
			info, err = discovery.GetServer(ctx, connect.NewRequest(&discoveryv1.GetServerRequest{}))
			if err != nil {
				t.Fatal(err)
			}
			if info.Msg.SetupRequired || info.Msg.Profile.Name != "API community" {
				t.Fatal("completion not visible")
			}
			_, err = client.CompleteSetup(ctx, connect.NewRequest(&authv1.CompleteSetupRequest{}))
			requireConnectCode(t, err, connect.CodeFailedPrecondition)
		})
	}
}
