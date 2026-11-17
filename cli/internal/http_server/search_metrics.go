package http_server

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/chattocorp/chatto/cli/internal/search"
)

// searchStatusTimeout bounds the provider status requests of one scrape.
const searchStatusTimeout = 2 * time.Second

// searchStatusReader collects the classified status of every search provider.
type searchStatusReader interface {
	ProviderReports(context.Context) ([]search.ProviderReport, error)
}

// searchProviderCollector exports the status of all search providers that
// answer the scrape's status request. Each provider counts in the state and
// freshness gauges, so a failed provider stays visible next to a healthy one.
// The classification is the same one that Admin → System shows. The labels
// are fixed values, so the cardinality is bounded.
type searchProviderCollector struct {
	client searchStatusReader

	up        *prometheus.Desc
	state     *prometheus.Desc
	freshness *prometheus.Desc
	stalled   *prometheus.Desc
	lag       *prometheus.Desc
	pending   *prometheus.Desc
	operation *prometheus.Desc
}

func newSearchProviderCollector(client searchStatusReader) *searchProviderCollector {
	return &searchProviderCollector{
		client:    client,
		up:        prometheus.NewDesc("chatto_search_provider_status_up", "Whether at least one search provider answered the status request of this scrape.", nil, nil),
		state:     prometheus.NewDesc("chatto_search_provider_state", "Number of search providers in each state; unknown counts providers that reported no state or an outdated status.", []string{"state"}, nil),
		freshness: prometheus.NewDesc("chatto_search_provider_index_freshness", "Number of search providers with each index freshness; unknown counts providers that reported no freshness or an outdated status.", []string{"freshness"}, nil),
		stalled:   prometheus.NewDesc("chatto_search_provider_stalled", "Number of search providers whose index has pending events without committed progress for the stall threshold.", nil, nil),
		lag:       prometheus.NewDesc("chatto_search_provider_lag_events", "Largest observed EVT sequence minus last committed sequence of any search provider; absent when no provider reports both.", nil, nil),
		pending:   prometheus.NewDesc("chatto_search_provider_pending_seconds", "Longest time that pending events of any search provider had no committed progress; zero when all are current.", nil, nil),
		operation: prometheus.NewDesc("chatto_search_provider_current_operation_seconds", "Longest time that the current index operation of any search provider has run; zero when all are idle.", nil, nil),
	}
}

func (c *searchProviderCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{c.up, c.state, c.freshness, c.stalled, c.lag, c.pending, c.operation} {
		ch <- desc
	}
}

func (c *searchProviderCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), searchStatusTimeout)
	defer cancel()
	reports, err := c.client.ProviderReports(ctx)
	if err != nil || len(reports) == 0 {
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 1)

	states := map[search.ProviderHealth]int{}
	freshness := map[search.IndexHealth]int{}
	var lag uint64
	var lagReported bool
	var pending, operation float64
	for _, report := range reports {
		states[report.Health]++
		freshness[report.Index]++
		status := report.Status
		if status.CommittedSequence != nil && status.ObservedSequence != nil {
			lag = max(lag, status.GetObservedSequence()-min(status.GetObservedSequence(), status.GetCommittedSequence()))
			lagReported = true
		}
		pending = max(pending, status.GetPendingDuration().AsDuration().Seconds())
		operation = max(operation, status.GetCurrentOperationDuration().AsDuration().Seconds())
	}
	for _, state := range search.ProviderHealths {
		ch <- prometheus.MustNewConstMetric(c.state, prometheus.GaugeValue, float64(states[state]), string(state))
	}
	for _, value := range search.IndexHealths {
		ch <- prometheus.MustNewConstMetric(c.freshness, prometheus.GaugeValue, float64(freshness[value]), string(value))
	}
	ch <- prometheus.MustNewConstMetric(c.stalled, prometheus.GaugeValue, float64(freshness[search.IndexHealthStalled]))
	if lagReported {
		ch <- prometheus.MustNewConstMetric(c.lag, prometheus.GaugeValue, float64(lag))
	}
	ch <- prometheus.MustNewConstMetric(c.pending, prometheus.GaugeValue, pending)
	ch <- prometheus.MustNewConstMetric(c.operation, prometheus.GaugeValue, operation)
}
