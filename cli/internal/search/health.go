package search

import (
	"cmp"
	"context"
	"slices"
	"time"

	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
)

// MaxStatusAge is the oldest provider status that health classification
// accepts. An older status no longer describes the provider, so its state
// and freshness are unknown.
const MaxStatusAge = 30 * time.Second

// ProviderHealth is whether a provider can answer queries. The values are
// also metric label values.
type ProviderHealth string

const (
	// ProviderHealthUnknown means that the provider reported no state, or
	// that its status is older than MaxStatusAge.
	ProviderHealthUnknown     ProviderHealth = "unknown"
	ProviderHealthStarting    ProviderHealth = "starting"
	ProviderHealthIndexing    ProviderHealth = "indexing"
	ProviderHealthReady       ProviderHealth = "ready"
	ProviderHealthDegraded    ProviderHealth = "degraded"
	ProviderHealthUnavailable ProviderHealth = "unavailable"
)

// ProviderHealths lists every ProviderHealth value, for metrics that report
// a count for each value.
var ProviderHealths = []ProviderHealth{
	ProviderHealthUnknown, ProviderHealthStarting, ProviderHealthIndexing,
	ProviderHealthReady, ProviderHealthDegraded, ProviderHealthUnavailable,
}

// IndexHealth is whether a provider's index follows the event log. The
// values are also metric label values.
type IndexHealth string

const (
	// IndexHealthUnknown means that the provider reported no freshness, or
	// that its status is older than MaxStatusAge.
	IndexHealthUnknown IndexHealth = "unknown"
	IndexHealthCurrent IndexHealth = "current"
	IndexHealthBehind  IndexHealth = "behind"
	IndexHealthStalled IndexHealth = "stalled"
)

// IndexHealths lists every IndexHealth value, for metrics that report a
// count for each value.
var IndexHealths = []IndexHealth{IndexHealthUnknown, IndexHealthCurrent, IndexHealthBehind, IndexHealthStalled}

// ProviderReport is the status of one provider with its classified health.
// The Admin API and the search metrics both use these classifications, so
// they agree about each provider.
type ProviderReport struct {
	Status *searchv1.GetStatusResponse
	Health ProviderHealth
	Index  IndexHealth
}

// ClassifyStatus derives the health of one provider status at time now. A
// status without observed_at is accepted, because some providers do not
// report it.
func ClassifyStatus(status *searchv1.GetStatusResponse, now time.Time) ProviderReport {
	report := ProviderReport{Status: status, Health: ProviderHealthUnknown, Index: IndexHealthUnknown}
	if observedAt := status.GetObservedAt(); observedAt != nil && now.Sub(observedAt.AsTime()) > MaxStatusAge {
		return report
	}
	switch status.GetState() {
	case searchv1.ProviderState_PROVIDER_STATE_STARTING:
		report.Health = ProviderHealthStarting
	case searchv1.ProviderState_PROVIDER_STATE_INDEXING:
		report.Health = ProviderHealthIndexing
	case searchv1.ProviderState_PROVIDER_STATE_READY:
		report.Health = ProviderHealthReady
	case searchv1.ProviderState_PROVIDER_STATE_DEGRADED:
		report.Health = ProviderHealthDegraded
	case searchv1.ProviderState_PROVIDER_STATE_UNAVAILABLE:
		report.Health = ProviderHealthUnavailable
	}
	switch status.GetFreshness() {
	case searchv1.IndexFreshness_INDEX_FRESHNESS_CURRENT:
		report.Index = IndexHealthCurrent
	case searchv1.IndexFreshness_INDEX_FRESHNESS_BEHIND:
		report.Index = IndexHealthBehind
	case searchv1.IndexFreshness_INDEX_FRESHNESS_STALLED:
		report.Index = IndexHealthStalled
	}
	return report
}

// ProviderReports collects and classifies the status of every provider
// process. When no provider answers AllStatusSubject, for example because
// all providers are older versions, it falls back to the status of the one
// provider that answers the queue-grouped status request. It returns an
// error only when no provider answers at all. Reports are sorted by host and
// instance ID.
func (c *Client) ProviderReports(ctx context.Context) ([]ProviderReport, error) {
	statuses, err := c.ListStatuses(ctx)
	if err != nil {
		return nil, err
	}
	if len(statuses) == 0 {
		status, err := c.GetStatus(ctx)
		if err != nil {
			return nil, err
		}
		statuses = []*searchv1.GetStatusResponse{status}
	}
	now := time.Now()
	reports := make([]ProviderReport, 0, len(statuses))
	for _, status := range statuses {
		reports = append(reports, ClassifyStatus(status, now))
	}
	slices.SortFunc(reports, func(a, b ProviderReport) int {
		return cmp.Or(
			cmp.Compare(a.Status.GetHost(), b.Status.GetHost()),
			cmp.Compare(a.Status.GetInstanceId(), b.Status.GetInstanceId()),
		)
	})
	return reports, nil
}
