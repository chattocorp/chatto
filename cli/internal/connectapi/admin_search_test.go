package connectapi

import (
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAdminDiagnosticsReportsSearchProviders(t *testing.T) {
	ready := &searchv1.GetStatusResponse{
		State:                    searchv1.ProviderState_PROVIDER_STATE_READY,
		Freshness:                searchv1.IndexFreshness_INDEX_FRESHNESS_CURRENT,
		CommittedSequence:        new(uint64(42)),
		ObservedSequence:         new(uint64(42)),
		InstanceId:               "a",
		Host:                     "search-0",
		ImplementationVersion:    "0.5.0",
		CurrentOperationDuration: durationpb.New(2 * time.Second),
		ObservedAt:               timestamppb.Now(),
		QueryProbe: &searchv1.QueryProbeStatus{
			LastResult:          searchv1.QueryProbeResult_QUERY_PROBE_RESULT_TIMED_OUT,
			ConsecutiveFailures: 3,
		},
	}
	unavailable := &searchv1.GetStatusResponse{State: searchv1.ProviderState_PROVIDER_STATE_UNAVAILABLE, InstanceId: "b"}
	stale := &searchv1.GetStatusResponse{
		State:      searchv1.ProviderState_PROVIDER_STATE_READY,
		Freshness:  searchv1.IndexFreshness_INDEX_FRESHNESS_CURRENT,
		InstanceId: "c",
		ObservedAt: timestamppb.New(time.Now().Add(-time.Minute)),
	}

	for name, tc := range map[string]struct {
		enabled  bool
		provider *fakeMessageSearchProvider
		want     func(*testing.T, *adminv1.AdminSearchStatus)
	}{
		"disabled": {
			want: func(t *testing.T, status *adminv1.AdminSearchStatus) {
				if status != nil {
					t.Fatalf("Search = %+v, want nil", status)
				}
			},
		},
		"no provider": {
			enabled:  true,
			provider: &fakeMessageSearchProvider{statusErr: errors.New("no responders")},
			want: func(t *testing.T, status *adminv1.AdminSearchStatus) {
				if status == nil || status.GetAvailable() || len(status.GetProviders()) != 0 {
					t.Fatalf("Search = %+v, want unavailable without providers", status)
				}
			},
		},
		"mixed providers": {
			enabled:  true,
			provider: &fakeMessageSearchProvider{providers: []*searchv1.GetStatusResponse{ready, unavailable, stale}},
			want: func(t *testing.T, status *adminv1.AdminSearchStatus) {
				if !status.GetAvailable() || len(status.GetProviders()) != 3 {
					t.Fatalf("Search = %+v, want three providers", status)
				}
				first := status.GetProviders()[0]
				if first.GetHost() != "search-0" || first.GetImplementationVersion() != "0.5.0" ||
					first.GetState() != adminv1.AdminSearchProviderState_ADMIN_SEARCH_PROVIDER_STATE_READY ||
					first.GetFreshness() != adminv1.AdminSearchIndexFreshness_ADMIN_SEARCH_INDEX_FRESHNESS_CURRENT ||
					first.GetCommittedSequence() != "42" || first.GetCurrentOperationSeconds() != 2 ||
					first.PendingSeconds != nil ||
					first.GetQueryProbeResult() != adminv1.AdminSearchQueryProbeResult_ADMIN_SEARCH_QUERY_PROBE_RESULT_TIMED_OUT ||
					first.GetQueryProbeConsecutiveFailures() != 3 {
					t.Fatalf("ready provider = %+v", first)
				}
				if got := status.GetProviders()[1].GetState(); got != adminv1.AdminSearchProviderState_ADMIN_SEARCH_PROVIDER_STATE_UNAVAILABLE {
					t.Fatalf("unavailable provider state = %v", got)
				}
				staleProvider := status.GetProviders()[2]
				if staleProvider.GetState() != adminv1.AdminSearchProviderState_ADMIN_SEARCH_PROVIDER_STATE_UNKNOWN ||
					staleProvider.GetFreshness() != adminv1.AdminSearchIndexFreshness_ADMIN_SEARCH_INDEX_FRESHNESS_UNKNOWN {
					t.Fatalf("stale provider = %+v, want unknown", staleProvider)
				}
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := newConnectAPITestEnv(t)
			env.api.config.Search.Enabled = tc.enabled
			if tc.provider != nil {
				env.api.searchProvider = tc.provider
			}
			if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, env.viewer.Id, core.RoleOwner); err != nil {
				t.Fatalf("AssignServerRole owner: %v", err)
			}
			resp, err := env.adminDiagnostics.GetSystemInfo(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.GetSystemInfoRequest{}))
			if err != nil {
				t.Fatalf("GetSystemInfo: %v", err)
			}
			tc.want(t, resp.Msg.GetSearch())
		})
	}
}
