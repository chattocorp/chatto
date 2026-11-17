package http_server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/expfmt"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
)

type fakeSearchStatus struct {
	status *searchv1.GetStatusResponse
	err    error
}

func (f fakeSearchStatus) GetStatus(context.Context) (*searchv1.GetStatusResponse, error) {
	return f.status, f.err
}

func TestSearchProviderCollectorReportsStalledIndex(t *testing.T) {
	observedAt := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		reader fakeSearchStatus
		want   string
	}{
		"stalled provider": {
			reader: fakeSearchStatus{status: &searchv1.GetStatusResponse{
				State:                    searchv1.ProviderState_PROVIDER_STATE_DEGRADED,
				Freshness:                searchv1.IndexFreshness_INDEX_FRESHNESS_STALLED,
				CommittedSequence:        new(uint64(40)),
				ObservedSequence:         new(uint64(42)),
				PendingDuration:          durationpb.New(6 * time.Minute),
				CurrentOperationDuration: durationpb.New(6 * time.Minute),
				ObservedAt:               timestamppb.New(observedAt),
				QueryProbe: &searchv1.QueryProbeStatus{
					ConsecutiveFailures: 3,
					InFlightSince:       timestamppb.New(observedAt.Add(-40 * time.Second)),
				},
			}},
			want: `
chatto_search_provider_current_operation_seconds 360
chatto_search_provider_index_freshness{freshness="stalled"} 1
chatto_search_provider_lag_events 2
chatto_search_provider_pending_seconds 360
chatto_search_provider_query_probe_consecutive_failures 3
chatto_search_provider_query_probe_in_flight_seconds 40
chatto_search_provider_stalled 1
chatto_search_provider_state{state="degraded"} 1
chatto_search_provider_status_up 1
`,
		},
		"healthy provider": {
			reader: fakeSearchStatus{status: &searchv1.GetStatusResponse{
				State:      searchv1.ProviderState_PROVIDER_STATE_READY,
				Freshness:  searchv1.IndexFreshness_INDEX_FRESHNESS_CURRENT,
				QueryProbe: &searchv1.QueryProbeStatus{LastResult: searchv1.QueryProbeResult_QUERY_PROBE_RESULT_SUCCEEDED},
			}},
			want: `
chatto_search_provider_query_probe_consecutive_failures 0
chatto_search_provider_query_probe_in_flight_seconds 0
chatto_search_provider_state{state="ready"} 1
`,
		},
		"no provider": {
			reader: fakeSearchStatus{err: errors.New("no responders")},
			want: `
chatto_search_provider_status_up 0
`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			wants := strings.Split(strings.TrimSpace(tc.want), "\n")
			var names []string
			for _, want := range wants {
				names = append(names, strings.FieldsFunc(want, func(r rune) bool { return r == '{' || r == ' ' })[0])
			}
			body, err := testutil.CollectAndFormat(newSearchProviderCollector(tc.reader), expfmt.TypeTextPlain, names...)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range wants {
				if !strings.Contains(string(body), want+"\n") {
					t.Fatalf("metrics missing %q\n%s", want, body)
				}
			}
		})
	}
}

func TestSearchProviderCollectorOmitsProbeBeforeFirstProbe(t *testing.T) {
	collector := newSearchProviderCollector(fakeSearchStatus{status: &searchv1.GetStatusResponse{
		State: searchv1.ProviderState_PROVIDER_STATE_INDEXING,
	}})
	count := testutil.CollectAndCount(collector,
		"chatto_search_provider_query_probe_consecutive_failures",
		"chatto_search_provider_query_probe_in_flight_seconds")
	if count != 0 {
		t.Fatalf("probe metrics before first probe = %d, want 0", count)
	}
}
