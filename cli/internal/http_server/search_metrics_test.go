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
	"github.com/chattocorp/chatto/cli/internal/search"
)

type fakeSearchStatus struct {
	statuses []*searchv1.GetStatusResponse
	err      error
}

func (f fakeSearchStatus) ProviderReports(context.Context) ([]search.ProviderReport, error) {
	reports := make([]search.ProviderReport, 0, len(f.statuses))
	for _, status := range f.statuses {
		reports = append(reports, search.ClassifyStatus(status, time.Now()))
	}
	return reports, f.err
}

func TestSearchProviderCollectorReportsProviders(t *testing.T) {
	for name, tc := range map[string]struct {
		reader fakeSearchStatus
		want   string
	}{
		"stalled provider": {
			reader: fakeSearchStatus{statuses: []*searchv1.GetStatusResponse{{
				State:                    searchv1.ProviderState_PROVIDER_STATE_DEGRADED,
				Freshness:                searchv1.IndexFreshness_INDEX_FRESHNESS_STALLED,
				CommittedSequence:        new(uint64(40)),
				ObservedSequence:         new(uint64(42)),
				PendingDuration:          durationpb.New(6 * time.Minute),
				CurrentOperationDuration: durationpb.New(6 * time.Minute),
			}}},
			want: `
chatto_search_provider_current_operation_seconds 360
chatto_search_provider_index_freshness{freshness="stalled"} 1
chatto_search_provider_lag_events 2
chatto_search_provider_pending_seconds 360
chatto_search_provider_stalled 1
chatto_search_provider_state{state="degraded"} 1
chatto_search_provider_status_up 1
`,
		},
		"mixed providers": {
			reader: fakeSearchStatus{statuses: []*searchv1.GetStatusResponse{
				{State: searchv1.ProviderState_PROVIDER_STATE_READY, Freshness: searchv1.IndexFreshness_INDEX_FRESHNESS_CURRENT},
				{State: searchv1.ProviderState_PROVIDER_STATE_UNAVAILABLE},
				{
					State:      searchv1.ProviderState_PROVIDER_STATE_READY,
					Freshness:  searchv1.IndexFreshness_INDEX_FRESHNESS_CURRENT,
					ObservedAt: timestamppb.New(time.Now().Add(-time.Minute)),
				},
			}},
			want: `
chatto_search_provider_index_freshness{freshness="current"} 1
chatto_search_provider_index_freshness{freshness="unknown"} 2
chatto_search_provider_stalled 0
chatto_search_provider_state{state="ready"} 1
chatto_search_provider_state{state="unavailable"} 1
chatto_search_provider_state{state="unknown"} 1
chatto_search_provider_status_up 1
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
