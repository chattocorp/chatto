package connectapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
	discoveryv1 "hmans.de/chatto/internal/pb/chatto/discovery/v1"
)

func TestPublicRPCIdempotencyLevels(t *testing.T) {
	tests := []struct {
		name    string
		file    protoreflect.FileDescriptor
		service protoreflect.Name
		method  protoreflect.Name
		want    descriptorpb.MethodOptions_IdempotencyLevel
	}{
		{
			name:    "discovery GET is side-effect free",
			file:    discoveryv1.File_chatto_discovery_v1_server_proto,
			service: "ServerDiscoveryService",
			method:  "GetServer",
			want:    descriptorpb.MethodOptions_NO_SIDE_EFFECTS,
		},
		{
			name:    "delete avatar is idempotent",
			file:    apiv1.File_chatto_api_v1_account_proto,
			service: "MyAccountService",
			method:  "DeleteAvatar",
			want:    descriptorpb.MethodOptions_IDEMPOTENT,
		},
		{
			name:    "delete custom status is idempotent",
			file:    apiv1.File_chatto_api_v1_account_proto,
			service: "MyAccountService",
			method:  "DeleteCustomStatus",
			want:    descriptorpb.MethodOptions_IDEMPOTENT,
		},
		{
			name:    "dismiss notification is idempotent",
			file:    apiv1.File_chatto_api_v1_notifications_proto,
			service: "NotificationService",
			method:  "DismissNotification",
			want:    descriptorpb.MethodOptions_IDEMPOTENT,
		},
		{
			name:    "unsubscribe push is idempotent",
			file:    apiv1.File_chatto_api_v1_push_notifications_proto,
			service: "PushNotificationService",
			method:  "Unsubscribe",
			want:    descriptorpb.MethodOptions_IDEMPOTENT,
		},
		{
			name:    "authenticated server read remains unannotated",
			file:    apiv1.File_chatto_api_v1_server_state_proto,
			service: "ServerService",
			method:  "GetMotd",
			want:    descriptorpb.MethodOptions_IDEMPOTENCY_UNKNOWN,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := test.file.Services().ByName(test.service)
			if service == nil {
				t.Fatalf("service %q not found", test.service)
			}
			method := service.Methods().ByName(test.method)
			if method == nil {
				t.Fatalf("method %q not found", test.method)
			}
			options, ok := method.Options().(*descriptorpb.MethodOptions)
			if !ok {
				t.Fatalf("method options type = %T", method.Options())
			}
			if got := options.GetIdempotencyLevel(); got != test.want {
				t.Fatalf("idempotency level = %v, want %v", got, test.want)
			}
		})
	}
}

func TestOnlySideEffectFreeRPCsAcceptGET(t *testing.T) {
	serverPath, serverHandler := apiv1connect.NewServerServiceHandler(apiv1connect.UnimplementedServerServiceHandler{})
	pushPath, pushHandler := apiv1connect.NewPushNotificationServiceHandler(apiv1connect.UnimplementedPushNotificationServiceHandler{})
	tests := []struct {
		name      string
		path      string
		procedure string
		handler   http.Handler
	}{
		{name: "unannotated read", path: serverPath, procedure: apiv1connect.ServerServiceGetMotdProcedure, handler: serverHandler},
		{name: "idempotent mutation", path: pushPath, procedure: apiv1connect.PushNotificationServiceUnsubscribeProcedure, handler: pushHandler},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.Handle(test.path, test.handler)
			req := httptest.NewRequest(http.MethodGet, test.procedure+"?connect=v1&encoding=json&message=%7B%7D", nil)
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)
			if resp.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d", resp.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestDiscoveryResponseETagChangesWithResponse(t *testing.T) {
	first, err := discoveryResponseETag(&discoveryv1.GetServerResponse{
		Profile: &apiv1.ServerPublicProfile{Name: "First"},
	})
	if err != nil {
		t.Fatalf("first ETag: %v", err)
	}
	second, err := discoveryResponseETag(&discoveryv1.GetServerResponse{
		Profile: &apiv1.ServerPublicProfile{Name: "Second"},
	})
	if err != nil {
		t.Fatalf("second ETag: %v", err)
	}
	if first == second {
		t.Fatalf("ETag did not change: %q", first)
	}
}

func TestIfNoneMatch(t *testing.T) {
	const etag = `"current"`
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{name: "empty", header: "", want: false},
		{name: "exact", header: etag, want: true},
		{name: "weak", header: `W/"current"`, want: true},
		{name: "lowercase weak", header: `w/"current"`, want: true},
		{name: "list", header: `"stale", W/"current"`, want: true},
		{name: "wildcard", header: "*", want: true},
		{name: "different", header: `"stale"`, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ifNoneMatch(test.header, etag); got != test.want {
				t.Fatalf("ifNoneMatch(%q) = %v, want %v", test.header, got, test.want)
			}
		})
	}
}
