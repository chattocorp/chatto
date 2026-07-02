package http_server

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	"hmans.de/chatto/internal/pb/chatto/admin/v1/adminv1connect"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	discoveryv1 "hmans.de/chatto/internal/pb/chatto/discovery/v1"
	"hmans.de/chatto/internal/pb/chatto/discovery/v1/discoveryv1connect"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
	"hmans.de/chatto/internal/pb/chatto/operator/v1/operatorv1connect"
)

func setupConnectTestServer(t *testing.T, authConfig config.AuthConfig) (*HTTPServer, *httptest.Server) {
	return setupConnectTestServerWithConfig(t, config.ChattoConfig{Auth: authConfig})
}

func setupConnectTestServerWithConfig(t *testing.T, cfg config.ChattoConfig) (*HTTPServer, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	s := setupHTTPServerTestServer(t, cfg.Auth)
	s.config = cfg
	s.setupConnectAPI()

	ts := httptest.NewServer(s.router)
	t.Cleanup(ts.Close)

	return s, ts
}

func TestConnectOperatorAPISeparation(t *testing.T) {
	t.Run("operator server serves only operator API", func(t *testing.T) {
		s, _ := setupConnectTestServerWithConfig(t, config.ChattoConfig{})
		ctx := context.Background()
		user, err := s.core.CreateUser(ctx, core.SystemActorID, "operator-connect", "Operator Connect", "password")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}

		operatorTS := newOperatorAPITestServer(t, s)
		operatorClient := operatorv1connect.NewOperatorUserServiceClient(operatorTS.Client(), operatorTS.URL+connectAPIPrefix)
		resp, err := operatorClient.GetUser(ctx, connect.NewRequest(&operatorv1.GetUserRequest{UserId: user.GetId()}))
		if err != nil {
			t.Fatalf("GetUser: %v", err)
		}
		if got := resp.Msg.GetMember().GetUser().GetLogin(); got != "operator-connect" {
			t.Fatalf("GetUser login = %q, want operator-connect", got)
		}

		adminClient := adminv1connect.NewAdminUserServiceClient(operatorTS.Client(), operatorTS.URL+connectAPIPrefix)
		if _, err := adminClient.ListMembers(ctx, connect.NewRequest(&adminv1.ListMembersRequest{})); connect.CodeOf(err) != connect.CodeUnimplemented {
			t.Fatalf("AdminUserService on operator server err = %v, want unimplemented", err)
		}
	})

	t.Run("public listener does not serve operator API", func(t *testing.T) {
		_, publicTS := setupConnectTestServerWithConfig(t, config.ChattoConfig{})
		operatorClient := operatorv1connect.NewOperatorUserServiceClient(publicTS.Client(), publicTS.URL+connectAPIPrefix)
		if _, err := operatorClient.ListUsers(context.Background(), connect.NewRequest(&operatorv1.ListUsersRequest{})); connect.CodeOf(err) != connect.CodeUnimplemented {
			t.Fatalf("OperatorUserService on public server err = %v, want unimplemented", err)
		}
	})
}

func newOperatorAPITestServer(t *testing.T, s *HTTPServer) *httptest.Server {
	t.Helper()
	operatorServer := s.newOperatorAPIServer()
	operatorTS := httptest.NewServer(operatorServer.Handler)
	t.Cleanup(operatorTS.Close)
	return operatorTS
}

func TestPrepareOperatorAPISocket(t *testing.T) {
	t.Run("creates socket with fixed mode", func(t *testing.T) {
		socketPath := shortTestSocketPath(t)
		s := &HTTPServer{config: config.ChattoConfig{OperatorAPI: config.OperatorAPIConfig{
			Enabled:    true,
			SocketPath: socketPath,
		}}}
		listener, info, err := s.prepareOperatorAPISocket()
		if err != nil {
			t.Fatalf("prepareOperatorAPISocket(): %v", err)
		}
		t.Cleanup(func() {
			_ = listener.Close()
			s.cleanupOperatorAPISocket(info)
		})
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("socket mode = %04o, want 0600", info.Mode().Perm())
		}
	})

	t.Run("rejects existing socket with different mode", func(t *testing.T) {
		socketPath := shortTestSocketPath(t)
		listener, err := net.Listen("unix", socketPath)
		if err != nil {
			t.Fatalf("listen setup socket: %v", err)
		}
		t.Cleanup(func() {
			_ = listener.Close()
			_ = os.Remove(socketPath)
		})
		if err := os.Chmod(socketPath, 0o666); err != nil {
			t.Fatalf("chmod setup socket: %v", err)
		}
		s := &HTTPServer{config: config.ChattoConfig{OperatorAPI: config.OperatorAPIConfig{
			Enabled:    true,
			SocketPath: socketPath,
		}}}
		if _, _, err := s.prepareOperatorAPISocket(); err == nil || !strings.Contains(err.Error(), "has mode 0666, want 0600") {
			t.Fatalf("prepareOperatorAPISocket() error = %v, want mode mismatch", err)
		}
	})

	t.Run("rejects active socket", func(t *testing.T) {
		socketPath := shortTestSocketPath(t)
		listener, err := net.Listen("unix", socketPath)
		if err != nil {
			t.Fatalf("listen setup socket: %v", err)
		}
		t.Cleanup(func() {
			_ = listener.Close()
			_ = os.Remove(socketPath)
		})
		if err := os.Chmod(socketPath, 0o600); err != nil {
			t.Fatalf("chmod setup socket: %v", err)
		}
		s := &HTTPServer{config: config.ChattoConfig{OperatorAPI: config.OperatorAPIConfig{
			Enabled:    true,
			SocketPath: socketPath,
		}}}
		if _, _, err := s.prepareOperatorAPISocket(); err == nil || !strings.Contains(err.Error(), "already in use") {
			t.Fatalf("prepareOperatorAPISocket() error = %v, want already in use", err)
		}
	})

	t.Run("rejects parent directory accessible by group", func(t *testing.T) {
		parent := shortTestSocketParent(t)
		if err := os.Chmod(parent, 0o770); err != nil {
			t.Fatalf("chmod unsafe parent: %v", err)
		}
		socketPath := parent + "/operator.sock"
		s := &HTTPServer{config: config.ChattoConfig{OperatorAPI: config.OperatorAPIConfig{
			Enabled:    true,
			SocketPath: socketPath,
		}}}
		if _, _, err := s.prepareOperatorAPISocket(); err == nil || !strings.Contains(err.Error(), "must not be accessible by group or other users") {
			t.Fatalf("prepareOperatorAPISocket() error = %v, want unsafe parent mode", err)
		}
	})

	t.Run("rejects parent directory accessible by other users", func(t *testing.T) {
		parent := shortTestSocketParent(t)
		if err := os.Chmod(parent, 0o777); err != nil {
			t.Fatalf("chmod unsafe parent: %v", err)
		}
		socketPath := parent + "/operator.sock"
		s := &HTTPServer{config: config.ChattoConfig{OperatorAPI: config.OperatorAPIConfig{
			Enabled:    true,
			SocketPath: socketPath,
		}}}
		if _, _, err := s.prepareOperatorAPISocket(); err == nil || !strings.Contains(err.Error(), "must not be accessible by group or other users") {
			t.Fatalf("prepareOperatorAPISocket() error = %v, want unsafe parent mode", err)
		}
	})

	t.Run("rejects parent directory with setgid bit", func(t *testing.T) {
		parent := shortTestSocketParent(t)
		if err := os.Chmod(parent, os.FileMode(0o700)|os.ModeSetgid); err != nil {
			t.Fatalf("chmod setgid parent: %v", err)
		}
		info, err := os.Lstat(parent)
		if err != nil {
			t.Fatalf("stat setgid parent: %v", err)
		}
		if info.Mode()&os.ModeSetgid == 0 {
			t.Skip("filesystem did not preserve setgid bit on test directory")
		}
		socketPath := parent + "/operator.sock"
		s := &HTTPServer{config: config.ChattoConfig{OperatorAPI: config.OperatorAPIConfig{
			Enabled:    true,
			SocketPath: socketPath,
		}}}
		if _, _, err := s.prepareOperatorAPISocket(); err == nil || !strings.Contains(err.Error(), "unsafe mode bits") {
			t.Fatalf("prepareOperatorAPISocket() error = %v, want unsafe parent mode bits", err)
		}
	})
}

func shortTestSocketPath(t *testing.T) string {
	t.Helper()
	return shortTestSocketParent(t) + "/operator.sock"
}

func shortTestSocketParent(t *testing.T) string {
	t.Helper()
	parent := fmt.Sprintf("/tmp/chatto-test-%d", time.Now().UnixNano())
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatalf("mkdir test socket parent: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(parent) })
	return parent
}

func setupConnectHTTP2TestServer(t *testing.T, authConfig config.AuthConfig) (*HTTPServer, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	s := setupHTTPServerTestServer(t, authConfig)
	s.setupConnectAPI()

	ts := httptest.NewUnstartedServer(s.router)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	t.Cleanup(ts.Close)

	return s, ts
}

func setupConnectH2CTestServer(t *testing.T, authConfig config.AuthConfig) (*HTTPServer, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	s := setupHTTPServerTestServer(t, authConfig)
	s.setupConnectAPI()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := newAppHTTPServer(listener.Addr().String(), s.router)
	go func() {
		_ = srv.Serve(listener)
	}()
	t.Cleanup(func() {
		_ = srv.Shutdown(context.Background())
	})

	return s, "http://" + listener.Addr().String()
}

func newH2CClient() *http.Client {
	return &http.Client{Transport: &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, addr)
		},
	}}
}

func TestConnectServerDiscoveryServiceGetServer(t *testing.T) {
	t.Run("returns public server metadata", func(t *testing.T) {
		_, ts := setupConnectTestServer(t, config.AuthConfig{
			Providers: []config.AuthProviderConfig{
				{ID: "hub", Type: config.AuthProviderTypeOpenIDConnect, Label: "Chatto Hub"},
			},
		})

		client := discoveryv1connect.NewServerDiscoveryServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		resp, err := client.GetServer(context.Background(), connect.NewRequest(&discoveryv1.GetServerRequest{}))
		if err != nil {
			t.Fatalf("GetServer: %v", err)
		}

		msg := resp.Msg
		if msg.GetProfile().GetName() != "Chatto" {
			t.Fatalf("profile name = %q, want Chatto", msg.GetProfile().GetName())
		}
		if msg.GetProfile().GetVersion() != "1.2.3" {
			t.Fatalf("profile version = %q, want 1.2.3", msg.GetProfile().GetVersion())
		}
		if !msg.GetLogin().GetDirectRegistrationEnabled() {
			t.Fatal("DirectRegistrationEnabled = false, want true")
		}
		if msg.GetLogin().GetAuthorizeUrl() != "/oauth/authorize" {
			t.Fatalf("AuthorizeUrl = %q, want /oauth/authorize", msg.GetLogin().GetAuthorizeUrl())
		}
		if len(msg.GetLogin().GetProviders()) != 1 {
			t.Fatalf("providers len = %d, want 1", len(msg.GetLogin().GetProviders()))
		}
		provider := msg.GetLogin().GetProviders()[0]
		if provider.Id != "hub" || provider.Type != config.AuthProviderTypeOpenIDConnect || provider.Label != "Chatto Hub" || provider.LoginUrl != "/auth/providers/hub" {
			t.Fatalf("AuthProviders[0] = %+v", provider)
		}
	})

	t.Run("serves protobuf over HTTP", func(t *testing.T) {
		_, ts := setupConnectTestServer(t, config.AuthConfig{})

		body := strings.NewReader("")
		req, err := http.NewRequest(http.MethodPost, ts.URL+connectAPIPrefix+discoveryv1connect.ServerDiscoveryServiceGetServerProcedure, body)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/proto")

		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("raw Connect request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/proto") {
			t.Fatalf("Content-Type = %q, want application/proto", ct)
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var msg discoveryv1.GetServerResponse
		if err := proto.Unmarshal(data, &msg); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if msg.GetProfile().GetName() != "Chatto" {
			t.Fatalf("profile name = %q, want Chatto", msg.GetProfile().GetName())
		}
	})

	t.Run("serves JSON over HTTP", func(t *testing.T) {
		_, ts := setupConnectTestServer(t, config.AuthConfig{})

		body := strings.NewReader("{}")
		req, err := http.NewRequest(http.MethodPost, ts.URL+connectAPIPrefix+discoveryv1connect.ServerDiscoveryServiceGetServerProcedure, body)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Connect-Protocol-Version", "1")

		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("raw JSON Connect request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("Content-Type = %q, want application/json", ct)
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var msg discoveryv1.GetServerResponse
		if err := protojson.Unmarshal(data, &msg); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if msg.GetProfile().GetName() != "Chatto" || msg.GetLogin().GetAuthorizeUrl() != "/oauth/authorize" {
			t.Fatalf("response = %+v, want Chatto metadata", &msg)
		}
	})

	t.Run("uses request origin for relative asset URLs", func(t *testing.T) {
		s, ts := setupConnectTestServer(t, config.AuthConfig{})

		ctx := testContext(t)
		asset, err := s.core.UploadServerBanner(ctx, bannerImageBytes(t))
		if err != nil {
			t.Fatalf("upload banner: %v", err)
		}
		if err := s.core.SetServerBanner(ctx, "test-admin", asset); err != nil {
			t.Fatalf("set banner: %v", err)
		}

		client := discoveryv1connect.NewServerDiscoveryServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		resp, err := client.GetServer(context.Background(), connect.NewRequest(&discoveryv1.GetServerRequest{}))
		if err != nil {
			t.Fatalf("GetServer: %v", err)
		}

		if !strings.HasPrefix(resp.Msg.GetProfile().GetBannerUrl(), ts.URL+"/") {
			t.Fatalf("profile BannerUrl = %q, want %s prefix", resp.Msg.GetProfile().GetBannerUrl(), ts.URL+"/")
		}
	})
}

func TestConnectReflection(t *testing.T) {
	_, ts := setupConnectHTTP2TestServer(t, config.AuthConfig{})

	client := grpcreflect.NewClient(ts.Client(), ts.URL+connectAPIPrefix)
	stream := client.NewStream(context.Background())
	t.Cleanup(func() {
		_, _ = stream.Close()
	})

	names, err := stream.ListServices()
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	nameSet := make(map[protoreflect.FullName]bool, len(names))
	for _, name := range names {
		nameSet[name] = true
	}
	for _, want := range []protoreflect.FullName{
		protoreflect.FullName(discoveryv1connect.ServerDiscoveryServiceName),
		protoreflect.FullName(apiv1connect.RoomServiceName),
		protoreflect.FullName(adminv1connect.AdminDiagnosticsServiceName),
	} {
		if !nameSet[want] {
			t.Fatalf("reflection services = %v, missing %s", names, want)
		}
	}

	files, err := stream.FileContainingSymbol(protoreflect.FullName(discoveryv1connect.ServerDiscoveryServiceName))
	if err != nil {
		t.Fatalf("FileContainingSymbol(%s): %v", discoveryv1connect.ServerDiscoveryServiceName, err)
	}
	if !descriptorFilesContain(files, "chatto/discovery/v1/server.proto") {
		t.Fatalf("descriptors for %s did not include chatto/discovery/v1/server.proto", discoveryv1connect.ServerDiscoveryServiceName)
	}

	if _, err := stream.FileContainingSymbol("chatto.core.v1.Event"); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("FileContainingSymbol(chatto.core.v1.Event) err = %v, want not found", err)
	}
}

func TestConnectReflectionSupportsPlaintextHTTP2(t *testing.T) {
	_, baseURL := setupConnectH2CTestServer(t, config.AuthConfig{})

	client := grpcreflect.NewClient(newH2CClient(), baseURL+connectAPIPrefix)
	stream := client.NewStream(context.Background())
	t.Cleanup(func() {
		_, _ = stream.Close()
	})

	names, err := stream.ListServices()
	if err != nil {
		t.Fatalf("ListServices over h2c: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("ListServices over h2c returned no services")
	}
}

func TestAppHTTPServerDoesNotBufferH2CUpgradeRequestBodies(t *testing.T) {
	_, baseURL := setupConnectH2CTestServer(t, config.AuthConfig{})
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("Parse(%q): %v", baseURL, err)
	}

	conn, err := net.Dial("tcp", parsedURL.Host)
	if err != nil {
		t.Fatalf("Dial(%s): %v", parsedURL.Host, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}

	_, err = fmt.Fprintf(conn, "POST /missing-h2c-upgrade-target HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Connection: Upgrade, HTTP2-Settings\r\n"+
		"Upgrade: h2c\r\n"+
		"HTTP2-Settings: AAMAAABkAAQAAP__\r\n"+
		"Content-Length: 1073741824\r\n"+
		"Content-Type: application/json\r\n"+
		"\r\n", parsedURL.Host)
	if err != nil {
		t.Fatalf("write upgrade request: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want HTTP response without h2c upgrade", resp.StatusCode)
	}
}

func descriptorFilesContain(files []*descriptorpb.FileDescriptorProto, name string) bool {
	for _, file := range files {
		if file.GetName() == name {
			return true
		}
	}
	return false
}

func TestConnectServerServiceGetServerStateRequiresAuth(t *testing.T) {
	_, ts := setupConnectTestServer(t, config.AuthConfig{})

	client := apiv1connect.NewServerServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
	_, err := client.GetServerState(context.Background(), connect.NewRequest(&apiv1.GetServerStateRequest{}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("GetServerState code = %v, want unauthenticated", connect.CodeOf(err))
	}
}

func TestConnectAPIRejectsOversizedRequestMessages(t *testing.T) {
	s, ts := setupConnectTestServer(t, config.AuthConfig{})
	ctx := context.Background()
	user, err := s.core.CreateUser(ctx, core.SystemActorID, "connect-oversized", "Connect Oversized", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := s.core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	client := apiv1connect.NewRoomServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
	req := connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: strings.Repeat("a", connectapi.MaxRequestMessageBytes+1),
	})
	req.Header().Set("Authorization", "Bearer "+token)
	_, err = client.GetRoomEvents(ctx, req)
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("GetRoomEvents oversized err = %v, want resource exhausted", err)
	}
}

func TestConnectAPIValidatesRequiredRequestFields(t *testing.T) {
	s, ts := setupConnectTestServer(t, config.AuthConfig{})
	ctx := context.Background()
	user, err := s.core.CreateUser(ctx, core.SystemActorID, "connect-validation", "Connect Validation", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := s.core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	authorize := func(req interface{ Header() http.Header }) {
		req.Header().Set("Authorization", "Bearer "+token)
	}
	requireInvalidArgument := func(t *testing.T, err error) {
		t.Helper()
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("err = %v, want invalid argument", err)
		}
	}

	t.Run("message room id", func(t *testing.T) {
		client := apiv1connect.NewMessageServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.CreateMessageRequest{Body: "hello"})
		authorize(req)
		_, err := client.CreateMessage(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("read state room id", func(t *testing.T) {
		client := apiv1connect.NewRoomServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.MarkRoomAsReadRequest{})
		authorize(req)
		_, err := client.MarkRoomAsRead(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("read state thread root id", func(t *testing.T) {
		client := apiv1connect.NewThreadServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.MarkThreadAsReadRequest{RoomId: "room"})
		authorize(req)
		_, err := client.MarkThreadAsRead(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("reaction room id", func(t *testing.T) {
		client := apiv1connect.NewMessageServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.AddReactionRequest{
			MessageEventId: "event",
			Emoji:          "thumbsup",
		})
		authorize(req)
		_, err := client.AddReaction(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("reaction message event id", func(t *testing.T) {
		client := apiv1connect.NewMessageServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.AddReactionRequest{
			RoomId: "room",
			Emoji:  "thumbsup",
		})
		authorize(req)
		_, err := client.AddReaction(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("reaction emoji", func(t *testing.T) {
		client := apiv1connect.NewMessageServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.RemoveReactionRequest{
			RoomId:         "room",
			MessageEventId: "event",
		})
		authorize(req)
		_, err := client.RemoveReaction(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("timeline room id", func(t *testing.T) {
		client := apiv1connect.NewRoomServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.GetRoomEventsRequest{})
		authorize(req)
		_, err := client.GetRoomEvents(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("timeline event id", func(t *testing.T) {
		client := apiv1connect.NewRoomServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.GetRoomEventsAroundRequest{RoomId: "room"})
		authorize(req)
		_, err := client.GetRoomEventsAround(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("thread timeline root id", func(t *testing.T) {
		client := apiv1connect.NewThreadServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.GetThreadEventsRequest{RoomId: "room"})
		authorize(req)
		_, err := client.GetThreadEvents(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("thread timeline event id", func(t *testing.T) {
		client := apiv1connect.NewThreadServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
			RoomId:            "room",
			ThreadRootEventId: "root",
		})
		authorize(req)
		_, err := client.GetThreadEventsAround(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("thread follow room id", func(t *testing.T) {
		client := apiv1connect.NewThreadServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.FollowThreadRequest{ThreadRootEventId: "root"})
		authorize(req)
		_, err := client.FollowThread(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("thread unfollow root id", func(t *testing.T) {
		client := apiv1connect.NewThreadServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.UnfollowThreadRequest{RoomId: "room"})
		authorize(req)
		_, err := client.UnfollowThread(ctx, req)
		requireInvalidArgument(t, err)
	})
}

func TestConnectAPIAuthenticatesBeforeValidation(t *testing.T) {
	_, ts := setupConnectTestServer(t, config.AuthConfig{})

	client := apiv1connect.NewMessageServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
	_, err := client.CreateMessage(context.Background(), connect.NewRequest(&apiv1.CreateMessageRequest{}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("CreateMessage err = %v, want unauthenticated", err)
	}
}

func TestAuthenticateConnectRequest(t *testing.T) {
	t.Run("rejects missing injected user", func(t *testing.T) {
		_, err := authenticateConnectRequest(context.Background(), nil)
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Fatalf("authenticateConnectRequest err = %v, want unauthenticated", err)
		}
	})

	t.Run("returns narrow Connect caller", func(t *testing.T) {
		info, err := authenticateConnectRequest(authctx.WithUser(context.Background(), &corev1.User{
			Id:          "user-123",
			Login:       "should-not-leak",
			DisplayName: "Should Not Leak",
		}), nil)
		if err != nil {
			t.Fatalf("authenticateConnectRequest: %v", err)
		}
		caller, ok := info.(connectapi.Caller)
		if !ok {
			t.Fatalf("auth info type = %T, want connectapi.Caller", info)
		}
		if caller != (connectapi.Caller{UserID: "user-123"}) {
			t.Fatalf("caller = %+v, want user id only", caller)
		}
	})
}

func TestConnectRequestBaseURLTrustModel(t *testing.T) {
	t.Run("uses configured public URL before request headers", func(t *testing.T) {
		s := &HTTPServer{config: config.ChattoConfig{
			Webserver: config.WebserverConfig{URL: "https://configured.example.com/chatto"},
		}}
		req := httptest.NewRequest(http.MethodGet, "http://request.example.com/api/connect", nil)
		req.Header.Set("X-Forwarded-Proto", "https")

		if got, want := s.requestBaseURL(req), "https://configured.example.com"; got != want {
			t.Fatalf("requestBaseURL = %q, want %q", got, want)
		}
	})

	t.Run("uses direct TLS state when no public URL is configured", func(t *testing.T) {
		s := &HTTPServer{}
		req := httptest.NewRequest(http.MethodGet, "https://direct.example.com/api/connect", nil)

		if got, want := s.requestBaseURL(req), "https://direct.example.com"; got != want {
			t.Fatalf("requestBaseURL = %q, want %q", got, want)
		}
	})

	t.Run("ignores untrusted forwarded proto when no public URL is configured", func(t *testing.T) {
		s := &HTTPServer{}
		req := httptest.NewRequest(http.MethodGet, "http://direct.example.com/api/connect", nil)
		req.Header.Set("X-Forwarded-Proto", "https")

		if got, want := s.requestBaseURL(req), "http://direct.example.com"; got != want {
			t.Fatalf("requestBaseURL = %q, want %q", got, want)
		}
	})
}

func TestConnectNotificationPreferencesService(t *testing.T) {
	t.Run("requires authentication", func(t *testing.T) {
		s, ts := setupConnectTestServer(t, config.AuthConfig{})
		ctx := context.Background()
		member, err := s.core.CreateUser(ctx, core.SystemActorID, "connect-member", "Connect Member", "password")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		room, err := s.core.CreateRoom(ctx, member.Id, core.KindChannel, "", "connect-room", "")
		if err != nil {
			t.Fatalf("CreateRoom: %v", err)
		}
		if _, err := s.core.JoinRoom(ctx, member.Id, core.KindChannel, member.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}

		client := apiv1connect.NewNotificationPreferencesServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		_, err = client.UpdateRoomNotificationPreference(ctx, connect.NewRequest(&apiv1.UpdateRoomNotificationPreferenceRequest{
			RoomId: room.Id,
			Level:  apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Fatalf("UpdateRoomNotificationPreference err = %v, want unauthenticated", err)
		}

		_, err = client.GetRoomNotificationPreference(ctx, connect.NewRequest(&apiv1.GetRoomNotificationPreferenceRequest{
			RoomId: room.Id,
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Fatalf("GetRoomNotificationPreference err = %v, want unauthenticated", err)
		}
	})

	t.Run("requires room membership", func(t *testing.T) {
		s, ts := setupConnectTestServer(t, config.AuthConfig{})
		ctx := context.Background()
		member, err := s.core.CreateUser(ctx, core.SystemActorID, "connect-member", "Connect Member", "password")
		if err != nil {
			t.Fatalf("CreateUser(member): %v", err)
		}
		other, err := s.core.CreateUser(ctx, core.SystemActorID, "connect-other", "Connect Other", "password")
		if err != nil {
			t.Fatalf("CreateUser(other): %v", err)
		}
		room, err := s.core.CreateRoom(ctx, member.Id, core.KindChannel, "", "connect-room", "")
		if err != nil {
			t.Fatalf("CreateRoom: %v", err)
		}
		if _, err := s.core.JoinRoom(ctx, member.Id, core.KindChannel, member.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}
		token, err := s.core.CreateAuthToken(ctx, other.Id)
		if err != nil {
			t.Fatalf("CreateAuthToken: %v", err)
		}

		client := apiv1connect.NewNotificationPreferencesServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.UpdateRoomNotificationPreferenceRequest{
			RoomId: room.Id,
			Level:  apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
		})
		req.Header().Set("Authorization", "Bearer "+token)
		_, err = client.UpdateRoomNotificationPreference(ctx, req)
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("UpdateRoomNotificationPreference err = %v, want permission denied", err)
		}

		getReq := connect.NewRequest(&apiv1.GetRoomNotificationPreferenceRequest{
			RoomId: room.Id,
		})
		getReq.Header().Set("Authorization", "Bearer "+token)
		_, err = client.GetRoomNotificationPreference(ctx, getReq)
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("GetRoomNotificationPreference err = %v, want permission denied", err)
		}
	})

	t.Run("rejects invalid room notification requests", func(t *testing.T) {
		s, ts := setupConnectTestServer(t, config.AuthConfig{})
		ctx := context.Background()
		member, err := s.core.CreateUser(ctx, core.SystemActorID, "connect-member", "Connect Member", "password")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		token, err := s.core.CreateAuthToken(ctx, member.Id)
		if err != nil {
			t.Fatalf("CreateAuthToken: %v", err)
		}

		client := apiv1connect.NewNotificationPreferencesServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.UpdateRoomNotificationPreferenceRequest{
			RoomId: "",
			Level:  apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
		})
		req.Header().Set("Authorization", "Bearer "+token)
		_, err = client.UpdateRoomNotificationPreference(ctx, req)
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("UpdateRoomNotificationPreference empty room err = %v, want invalid argument", err)
		}

		req = connect.NewRequest(&apiv1.UpdateRoomNotificationPreferenceRequest{
			RoomId: "missing-room",
			Level:  apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
		})
		req.Header().Set("Authorization", "Bearer "+token)
		_, err = client.UpdateRoomNotificationPreference(ctx, req)
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Fatalf("UpdateRoomNotificationPreference missing room err = %v, want not found", err)
		}

		req = connect.NewRequest(&apiv1.UpdateRoomNotificationPreferenceRequest{
			RoomId: "missing-room",
			Level:  apiv1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED,
		})
		req.Header().Set("Authorization", "Bearer "+token)
		_, err = client.UpdateRoomNotificationPreference(ctx, req)
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("UpdateRoomNotificationPreference unspecified level err = %v, want invalid argument", err)
		}

		getReq := connect.NewRequest(&apiv1.GetRoomNotificationPreferenceRequest{
			RoomId: "",
		})
		getReq.Header().Set("Authorization", "Bearer "+token)
		_, err = client.GetRoomNotificationPreference(ctx, getReq)
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("GetRoomNotificationPreference empty room err = %v, want invalid argument", err)
		}
	})

	t.Run("sets a room notification level for a member", func(t *testing.T) {
		s, ts := setupConnectTestServer(t, config.AuthConfig{})
		ctx := context.Background()
		member, err := s.core.CreateUser(ctx, core.SystemActorID, "connect-member", "Connect Member", "password")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		room, err := s.core.CreateRoom(ctx, member.Id, core.KindChannel, "", "connect-room", "")
		if err != nil {
			t.Fatalf("CreateRoom: %v", err)
		}
		if _, err := s.core.JoinRoom(ctx, member.Id, core.KindChannel, member.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}
		token, err := s.core.CreateAuthToken(ctx, member.Id)
		if err != nil {
			t.Fatalf("CreateAuthToken: %v", err)
		}

		client := apiv1connect.NewNotificationPreferencesServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.UpdateRoomNotificationPreferenceRequest{
			RoomId: room.Id,
			Level:  apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
		})
		req.Header().Set("Authorization", "Bearer "+token)
		resp, err := client.UpdateRoomNotificationPreference(ctx, req)
		if err != nil {
			t.Fatalf("UpdateRoomNotificationPreference: %v", err)
		}
		if resp.Msg.Level != apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			t.Fatalf("Level = %v, want muted", resp.Msg.Level)
		}
		if resp.Msg.EffectiveLevel != apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			t.Fatalf("EffectiveLevel = %v, want muted", resp.Msg.EffectiveLevel)
		}

		getReq := connect.NewRequest(&apiv1.GetRoomNotificationPreferenceRequest{
			RoomId: room.Id,
		})
		getReq.Header().Set("Authorization", "Bearer "+token)
		getResp, err := client.GetRoomNotificationPreference(ctx, getReq)
		if err != nil {
			t.Fatalf("GetRoomNotificationPreference: %v", err)
		}
		if getResp.Msg.Level != apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			t.Fatalf("Get level = %v, want muted", getResp.Msg.Level)
		}
		if getResp.Msg.EffectiveLevel != apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			t.Fatalf("Get effective level = %v, want muted", getResp.Msg.EffectiveLevel)
		}

		got, err := s.core.GetRoomNotificationLevel(ctx, member.Id, room.Id)
		if err != nil {
			t.Fatalf("GetRoomNotificationLevel: %v", err)
		}
		if got != corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			t.Fatalf("stored level = %v, want muted", got)
		}
	})
}
