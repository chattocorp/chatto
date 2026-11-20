package http_server

import (
	"context"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
)

// searchStatusTimeout bounds the provider status request of one scrape.
const searchStatusTimeout = 2 * time.Second

// searchStatusReader reads the status of one search provider.
type searchStatusReader interface {
	GetStatus(context.Context) (*searchv1.GetStatusResponse, error)
}

// searchProviderCollector exports the status of the search provider that
// answers the scrape's status request. With several providers, each scrape
// can describe a different provider. The labels are fixed enum names, so the
// cardinality is bounded.
type searchProviderCollector struct {
	client searchStatusReader

	up        *prometheus.Desc
	state     *prometheus.Desc
	freshness *prometheus.Desc
	stalled   *prometheus.Desc
	lag       *prometheus.Desc
	pending   *prometheus.Desc
	operation *prometheus.Desc
	// probeFailures and probeInFlight describe the query probe of the
	// reported provider. Both are absent before its first probe starts, for
	// example during initial replay, so that a rebuild does not look like a
	// query failure.
	probeFailures *prometheus.Desc
	probeInFlight *prometheus.Desc
}

func newSearchProviderCollector(client searchStatusReader) *searchProviderCollector {
	return &searchProviderCollector{
		client:    client,
		up:        prometheus.NewDesc("chatto_search_provider_status_up", "Whether a search provider answered the status request of this scrape.", nil, nil),
		state:     prometheus.NewDesc("chatto_search_provider_state", "Reported search provider state; value is 1 for the current state.", []string{"state"}, nil),
		freshness: prometheus.NewDesc("chatto_search_provider_index_freshness", "Reported search index freshness; value is 1 for the current freshness.", []string{"freshness"}, nil),
		stalled:   prometheus.NewDesc("chatto_search_provider_stalled", "Whether the reported search index has pending events without committed progress for the stall threshold.", nil, nil),
		lag:       prometheus.NewDesc("chatto_search_provider_lag_events", "Reported observed EVT sequence minus the last committed sequence; absent when the provider does not report both.", nil, nil),
		pending:   prometheus.NewDesc("chatto_search_provider_pending_seconds", "Seconds that pending events of the reported search index had no committed progress; zero when current.", nil, nil),
		operation: prometheus.NewDesc("chatto_search_provider_current_operation_seconds", "Seconds that the current index operation of the reported provider has run; zero when idle.", nil, nil),
		probeFailures: prometheus.NewDesc("chatto_search_provider_query_probe_consecutive_failures", "Completed query probes of the reported provider that failed or timed out since its last success; absent before the first probe.", nil, nil),
		probeInFlight: prometheus.NewDesc("chatto_search_provider_query_probe_in_flight_seconds", "Age of the running query probe of the reported provider, or 0 when no probe runs; absent before the first probe.", nil, nil),
	}
}

func (c *searchProviderCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{c.up, c.state, c.freshness, c.stalled, c.lag, c.pending, c.operation, c.probeFailures, c.probeInFlight} {
		ch <- desc
	}
}

func (c *searchProviderCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), searchStatusTimeout)
	defer cancel()
	status, err := c.client.GetStatus(ctx)
	if err != nil || status == nil {
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 1)
	ch <- prometheus.MustNewConstMetric(c.state, prometheus.GaugeValue, 1, enumLabel(status.GetState().String(), "PROVIDER_STATE_"))
	ch <- prometheus.MustNewConstMetric(c.freshness, prometheus.GaugeValue, 1, enumLabel(status.GetFreshness().String(), "INDEX_FRESHNESS_"))
	stalled := status.GetFreshness() == searchv1.IndexFreshness_INDEX_FRESHNESS_STALLED
	ch <- prometheus.MustNewConstMetric(c.stalled, prometheus.GaugeValue, boolMetric(stalled))
	if status.CommittedSequence != nil && status.ObservedSequence != nil {
		lag := status.GetObservedSequence() - min(status.GetObservedSequence(), status.GetCommittedSequence())
		ch <- prometheus.MustNewConstMetric(c.lag, prometheus.GaugeValue, float64(lag))
	}
	ch <- prometheus.MustNewConstMetric(c.pending, prometheus.GaugeValue, status.GetPendingDuration().AsDuration().Seconds())
	ch <- prometheus.MustNewConstMetric(c.operation, prometheus.GaugeValue, status.GetCurrentOperationDuration().AsDuration().Seconds())
	if probe := status.GetQueryProbe(); probe != nil {
		ch <- prometheus.MustNewConstMetric(c.probeFailures, prometheus.GaugeValue, float64(probe.GetConsecutiveFailures()))
		ch <- prometheus.MustNewConstMetric(c.probeInFlight, prometheus.GaugeValue, probeInFlightSeconds(status))
	}
}

// probeInFlightSeconds measures the running probe against the provider's
// own observation time, so that clock skew between the provider and this
// replica does not change the value.
func probeInFlightSeconds(status *searchv1.GetStatusResponse) float64 {
	since := status.GetQueryProbe().GetInFlightSince()
	if since == nil {
		return 0
	}
	now := time.Now()
	if observed := status.GetObservedAt(); observed != nil {
		now = observed.AsTime()
	}
	return max(0, now.Sub(since.AsTime()).Seconds())
}

// enumLabel turns a protobuf enum name into a short label. Unknown future
// values keep their numeric name, which stays bounded.
func enumLabel(name, prefix string) string {
	return strings.ToLower(strings.TrimPrefix(name, prefix))
}
