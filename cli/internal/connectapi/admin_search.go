package connectapi

import (
	"context"
	"strconv"
	"time"

	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
	searchsvc "github.com/chattocorp/chatto/cli/internal/search"
)

// adminSearchStatusTimeout bounds the provider status requests of one
// diagnostics call.
const adminSearchStatusTimeout = 2 * time.Second

// adminSearchStatus collects the status of every search provider. It returns
// nil when message search is disabled, and a status without providers when
// no provider answers.
func (s *adminDiagnosticsService) adminSearchStatus(ctx context.Context) *adminv1.AdminSearchStatus {
	if !s.api.config.Search.Enabled {
		return nil
	}
	status := &adminv1.AdminSearchStatus{}
	if s.api.searchProvider == nil {
		return status
	}
	ctx, cancel := context.WithTimeout(ctx, adminSearchStatusTimeout)
	defer cancel()
	reports, err := s.api.searchProvider.ProviderReports(ctx)
	if err != nil {
		return status
	}
	status.Available = len(reports) > 0
	for _, report := range reports {
		status.Providers = append(status.Providers, adminSearchProvider(report))
	}
	return status
}

func adminSearchProvider(report searchsvc.ProviderReport) *adminv1.AdminSearchProvider {
	status := report.Status
	provider := &adminv1.AdminSearchProvider{
		InstanceId:            status.GetInstanceId(),
		Host:                  status.GetHost(),
		ImplementationVersion: status.GetImplementationVersion(),
		State:                 adminSearchProviderState(report.Health),
		Freshness:             adminSearchIndexFreshness(report.Index),
		IndexedEventCount:     optionalUint64String(status.IndexedEventCount),
		TargetEventCount:      optionalUint64String(status.TargetEventCount),
		CommittedSequence:     optionalUint64String(status.CommittedSequence),
		ObservedSequence:      optionalUint64String(status.ObservedSequence),
		LastCommitTime:        status.GetLastCommitTime(),
		CurrentOperation:      status.GetCurrentOperation(),
		ObservedAt:            status.GetObservedAt(),
	}
	if pending := status.GetPendingDuration(); pending != nil {
		provider.PendingSeconds = new(pending.AsDuration().Seconds())
	}
	if operation := status.GetCurrentOperationDuration(); operation != nil {
		provider.CurrentOperationSeconds = new(operation.AsDuration().Seconds())
	}
	if probe := status.GetQueryProbe(); probe != nil {
		provider.QueryProbeResult = adminSearchQueryProbeResult(probe.GetLastResult())
		provider.QueryProbeCompletedAt = probe.GetLastCompletedAt()
		provider.QueryProbeSucceededAt = probe.GetLastSucceededAt()
		provider.QueryProbeConsecutiveFailures = probe.GetConsecutiveFailures()
		provider.QueryProbeInFlightSince = probe.GetInFlightSince()
	}
	return provider
}

func optionalUint64String(value *uint64) *string {
	if value == nil {
		return nil
	}
	return new(strconv.FormatUint(*value, 10))
}

func adminSearchProviderState(health searchsvc.ProviderHealth) adminv1.AdminSearchProviderState {
	switch health {
	case searchsvc.ProviderHealthUnknown:
		return adminv1.AdminSearchProviderState_ADMIN_SEARCH_PROVIDER_STATE_UNKNOWN
	case searchsvc.ProviderHealthStarting:
		return adminv1.AdminSearchProviderState_ADMIN_SEARCH_PROVIDER_STATE_STARTING
	case searchsvc.ProviderHealthIndexing:
		return adminv1.AdminSearchProviderState_ADMIN_SEARCH_PROVIDER_STATE_INDEXING
	case searchsvc.ProviderHealthReady:
		return adminv1.AdminSearchProviderState_ADMIN_SEARCH_PROVIDER_STATE_READY
	case searchsvc.ProviderHealthDegraded:
		return adminv1.AdminSearchProviderState_ADMIN_SEARCH_PROVIDER_STATE_DEGRADED
	case searchsvc.ProviderHealthUnavailable:
		return adminv1.AdminSearchProviderState_ADMIN_SEARCH_PROVIDER_STATE_UNAVAILABLE
	default:
		return adminv1.AdminSearchProviderState_ADMIN_SEARCH_PROVIDER_STATE_UNSPECIFIED
	}
}

func adminSearchIndexFreshness(health searchsvc.IndexHealth) adminv1.AdminSearchIndexFreshness {
	switch health {
	case searchsvc.IndexHealthUnknown:
		return adminv1.AdminSearchIndexFreshness_ADMIN_SEARCH_INDEX_FRESHNESS_UNKNOWN
	case searchsvc.IndexHealthCurrent:
		return adminv1.AdminSearchIndexFreshness_ADMIN_SEARCH_INDEX_FRESHNESS_CURRENT
	case searchsvc.IndexHealthBehind:
		return adminv1.AdminSearchIndexFreshness_ADMIN_SEARCH_INDEX_FRESHNESS_BEHIND
	case searchsvc.IndexHealthStalled:
		return adminv1.AdminSearchIndexFreshness_ADMIN_SEARCH_INDEX_FRESHNESS_STALLED
	default:
		return adminv1.AdminSearchIndexFreshness_ADMIN_SEARCH_INDEX_FRESHNESS_UNSPECIFIED
	}
}

func adminSearchQueryProbeResult(result searchv1.QueryProbeResult) adminv1.AdminSearchQueryProbeResult {
	switch result {
	case searchv1.QueryProbeResult_QUERY_PROBE_RESULT_SUCCEEDED:
		return adminv1.AdminSearchQueryProbeResult_ADMIN_SEARCH_QUERY_PROBE_RESULT_SUCCEEDED
	case searchv1.QueryProbeResult_QUERY_PROBE_RESULT_FAILED:
		return adminv1.AdminSearchQueryProbeResult_ADMIN_SEARCH_QUERY_PROBE_RESULT_FAILED
	case searchv1.QueryProbeResult_QUERY_PROBE_RESULT_TIMED_OUT:
		return adminv1.AdminSearchQueryProbeResult_ADMIN_SEARCH_QUERY_PROBE_RESULT_TIMED_OUT
	default:
		return adminv1.AdminSearchQueryProbeResult_ADMIN_SEARCH_QUERY_PROBE_RESULT_UNSPECIFIED
	}
}
