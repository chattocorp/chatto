package connectapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1/apiv1connect"
	discoveryv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/discovery/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestDiscoveryFeatureCatalog(t *testing.T) {
	t.Parallel()
	service := &serverDiscoveryService{api: &API{version: "0.5.0"}}
	response, err := service.ListFeatures(context.Background(), connect.NewRequest(&discoveryv1.ListFeaturesRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	features := response.Msg.GetFeatures()
	if len(features) == 0 {
		t.Fatal("public discovery omitted the product catalog")
	}
	seen := map[string]bool{}
	for _, feature := range features {
		if feature.Id == "" || feature.Name == "" || feature.Description == "" || feature.FrontendIntroducedVersion == "" || seen[feature.Id] {
			t.Fatalf("incomplete or duplicate feature: %v", feature)
		}
		seen[feature.Id] = true
	}
	before, err := discoveryResponseETag(response.Msg)
	if err != nil {
		t.Fatal(err)
	}
	features[0].Description = "Changed catalog text"
	after, err := discoveryResponseETag(response.Msg)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("catalog change must invalidate discovery caches")
	}
	next, err := service.ListFeatures(context.Background(), connect.NewRequest(&discoveryv1.ListFeaturesRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if next.Msg.Features[0].Description == features[0].Description {
		t.Fatal("discovery responses must not share mutable catalog messages")
	}
}

func TestDiscoveryFeatureLocale(t *testing.T) {
	t.Parallel()
	service := &serverDiscoveryService{api: &API{version: "0.5.0"}}
	english, err := service.ListFeatures(t.Context(), connect.NewRequest(&discoveryv1.ListFeaturesRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	englishETag, err := discoveryResponseETag(english.Msg)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ locale, name string }{
		{"", "Separate thread views"},
		{"en-GB", "Separate thread views"},
		{"de-DE", "Getrennte Thread-Ansichten"},
		{"de-AT", "Getrennte Thread-Ansichten"},
		{"DE-ch", "Getrennte Thread-Ansichten"},
		{"fr-CA", "Vues séparées des fils"},
		{"zh-TW", "獨立的討論串檢視"},
		{"zh-CN", "独立的主题视图"},
		{"invalid_locale!", "Separate thread views"},
		{"fi-FI", "Separate thread views"},
	} {
		t.Run(test.locale, func(t *testing.T) {
			response, err := service.ListFeatures(t.Context(), connect.NewRequest(&discoveryv1.ListFeaturesRequest{Locale: new(test.locale)}))
			if err != nil {
				t.Fatal(err)
			}
			if got := response.Msg.Features[0].Name; got != test.name {
				t.Fatalf("name = %q, want %q", got, test.name)
			}
			for i, feature := range response.Msg.Features {
				base := english.Msg.Features[i]
				if feature.Id != base.Id || feature.FrontendIntroducedVersion != base.FrontendIntroducedVersion {
					t.Fatal("locale changed feature identity or version")
				}
			}
			etag, err := discoveryResponseETag(response.Msg)
			if err != nil {
				t.Fatal(err)
			}
			if test.name != english.Msg.Features[0].Name && etag == englishETag {
				t.Fatal("translated response reused the English ETag")
			}
		})
	}
}

func TestFeatureTranslations(t *testing.T) {
	t.Parallel()
	for locale, translations := range featureTranslations {
		for _, feature := range serverFeatureCatalog("") {
			text, ok := translations[feature.Id]
			if !ok || text.Name == "" || text.Description == "" {
				t.Errorf("%s: missing text for %s", locale, feature.Id)
			}
		}
	}
	if _, ok := featureTranslation("de-DE", "unknown-future-feature"); ok {
		t.Fatal("missing translations must fall back to English")
	}
}

func TestListFeaturesVersionFilter(t *testing.T) {
	t.Parallel()
	service := &serverDiscoveryService{api: &API{}}
	for _, test := range []struct {
		version string
		count   int
	}{
		{"0.5.0-beta.15", 2},
		{"0.5.0-beta.16", 1},
		{"0.5.0-beta.17", 1},
		{"0.5.0-beta.18", 0},
		{"0.5.0-beta.18+custom.1", 0},
		{"0.5.0", 0},
		{"9.0.0", 0},
	} {
		t.Run(test.version, func(t *testing.T) {
			response, err := service.ListFeatures(t.Context(), connect.NewRequest(&discoveryv1.ListFeaturesRequest{
				Locale: new("de-DE"), IntroducedAfterFrontendVersion: new(test.version),
			}))
			if err != nil {
				t.Fatal(err)
			}
			if len(response.Msg.Features) != test.count {
				t.Fatalf("got %d features, want %d", len(response.Msg.Features), test.count)
			}
		})
	}
	for _, version := range []string{"", "dev", "0.5", "v0.5.0", "0.5.0-beta.01", "0.5.0 "} {
		_, err := service.ListFeatures(t.Context(), connect.NewRequest(&discoveryv1.ListFeaturesRequest{IntroducedAfterFrontendVersion: new(version)}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%q: expected INVALID_ARGUMENT, got %v", version, err)
		}
	}
}

func TestPublicRPCIdempotencyLevels(t *testing.T) {
	t.Parallel()

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
			name:    "feature catalog GET is side-effect free",
			file:    discoveryv1.File_chatto_discovery_v1_server_proto,
			service: "ServerDiscoveryService",
			method:  "ListFeatures",
			want:    descriptorpb.MethodOptions_NO_SIDE_EFFECTS,
		},
		{
			name:    "delete avatar is idempotent",
			file:    apiv1.File_chatto_api_v1_user_service_proto,
			service: "UserService",
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
			name:    "update notification occurrence is idempotent",
			file:    apiv1.File_chatto_api_v1_notifications_proto,
			service: "NotificationService",
			method:  "MarkNotificationRead",
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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
