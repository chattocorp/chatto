import type { AdminSearchProvider, AdminSystemInfo } from '$lib/api/adminDiagnostics';

/**
 * Severity of one health check. `unknown` means the server did not report
 * the data that the check needs. It does not raise the overall status above
 * the other checks, but when every check is unknown, the overall status is
 * unknown too.
 */
export type HealthStatus = 'ok' | 'warning' | 'critical' | 'unknown';

/** Identifies a health check, so the view can choose its label and icon. */
export type HealthCheckId =
  | 'broker'
  | 'projections'
  | 'workers'
  | 'backlog'
  | 'storage'
  | 'voice_calls'
  | 'search'
  | 'search_index';

/**
 * A health check result. `detail` is a discriminated value that the view
 * localizes, so this module stays free of message catalog lookups.
 */
export type HealthCheck = {
  id: HealthCheckId;
  status: HealthStatus;
  detail: HealthDetail;
};

export type HealthDetail =
  | { kind: 'unavailable' }
  | { kind: 'broker_connected'; version: string }
  | { kind: 'broker_disconnected' }
  | { kind: 'projections_failed'; count: number }
  | { kind: 'projections_stalled'; count: number }
  | { kind: 'projections_catching_up'; count: number }
  | { kind: 'projections_running'; count: number }
  | { kind: 'workers_failing'; count: number }
  | { kind: 'workers_unconfirmed'; count: number }
  | { kind: 'workers_running'; count: number }
  | { kind: 'backlog_waiting'; count: number }
  | { kind: 'backlog_clear' }
  | { kind: 'storage_used'; percent: number }
  | { kind: 'storage_unlimited' }
  | { kind: 'livekit_connected' }
  | { kind: 'livekit_unreachable' }
  | { kind: 'livekit_unauthorized' }
  | { kind: 'livekit_error' }
  | { kind: 'livekit_insecure_url' }
  | { kind: 'livekit_webhooks_rejected' }
  | { kind: 'search_none' }
  | { kind: SearchProviderDetail; count: number; total: number }
  | { kind: SearchIndexDetail; count: number; total: number };

/** Details of the search availability check; see `searchCheck`. */
type SearchProviderDetail =
  'search_failed' | 'search_degraded' | 'search_indexing' | 'search_unknown' | 'search_ready';

/** Details of the search index check; see `searchIndexCheck`. */
type SearchIndexDetail =
  'search_index_stalled' | 'search_index_behind' | 'search_index_unknown' | 'search_index_current';

/** Usage at or above this share of an account limit is a warning. */
export const STORAGE_WARNING_RATIO = 0.75;
/** Usage at or above this share of an account limit is critical. */
export const STORAGE_CRITICAL_RATIO = 0.9;

const severity: Record<HealthStatus, number> = { unknown: 0, ok: 1, warning: 2, critical: 3 };

/**
 * Derives the System page health checks from one diagnostics snapshot.
 *
 * Only lasting conditions raise a check above `ok`: a lost broker
 * connection, failed projections, stalled projections (a warning), stalled or
 * missing durable workers, and account usage close to a limit. Projection lag, consumer backlog, and
 * unconfirmed workers are normal while work flows, so they appear only as
 * details.
 *
 * The voice calls check appears only when LiveKit is configured. A failed
 * LiveKit API check is critical. An insecure LiveKit URL, or a rejected
 * webhook more recent than the last accepted one, is a warning.
 *
 * The search checks appear only when message search is enabled. They show
 * search availability and index freshness separately; see `searchCheck` and
 * `searchIndexCheck`.
 *
 * The broker, projection, and voice calls checks describe the replica that
 * handled the request. The search checks cover every search provider that
 * answered. The other checks use broker state that every replica shares.
 */
export function systemHealthChecks(info: AdminSystemInfo): HealthCheck[] {
  const checks = [
    brokerCheck(info),
    projectionsCheck(info),
    workersCheck(info),
    backlogCheck(info),
    storageCheck(info)
  ];
  if (info.search) checks.push(searchCheck(info.search), searchIndexCheck(info.search));
  const voiceCalls = voiceCallsCheck(info);
  if (voiceCalls) checks.push(voiceCalls);
  return checks;
}

/**
 * Returns the most severe status of the checks. Unknown checks do not count,
 * unless every check is unknown: then the overall status is unknown, because
 * no data supports `ok`.
 */
export function overallHealth(checks: HealthCheck[]): HealthStatus {
  if (checks.length > 0 && checks.every((check) => check.status === 'unknown')) return 'unknown';
  return checks.reduce<HealthStatus>(
    (current, check) =>
      check.status !== 'unknown' && severity[check.status] > severity[current]
        ? check.status
        : current,
    'ok'
  );
}

/**
 * Returns the highest share of an account limit in use, from 0 to 1, or
 * `null` when the account has no limits. A limit of zero or less means
 * unlimited.
 */
export function highestLimitUsage(account: AdminSystemInfo['account']): number | null {
  const ratios = [
    [account.storageUsed, account.storage],
    [account.memoryUsed, account.memory],
    [account.streamsUsed, account.streams],
    [account.consumersUsed, account.consumers]
  ]
    .filter(([, limit]) => limit > 0)
    .map(([used, limit]) => used / limit);
  return ratios.length > 0 ? Math.max(...ratios) : null;
}

function brokerCheck(info: AdminSystemInfo): HealthCheck {
  return info.connection.connected
    ? {
        id: 'broker',
        status: 'ok',
        detail: { kind: 'broker_connected', version: info.connection.version }
      }
    : { id: 'broker', status: 'critical', detail: { kind: 'broker_disconnected' } };
}

function projectionsCheck(info: AdminSystemInfo): HealthCheck {
  if (!info.projectionsAvailable || info.projections.length === 0) {
    return { id: 'projections', status: 'unknown', detail: { kind: 'unavailable' } };
  }
  const failed = info.projections.filter((projection) => projection.failed).length;
  if (failed > 0) {
    return {
      id: 'projections',
      status: 'critical',
      detail: { kind: 'projections_failed', count: failed }
    };
  }
  // The server reports a projection as stalled only when events wait without
  // committed progress for its stall threshold, so ordinary lag stays ok.
  const stalled = info.projections.filter((projection) => projection.stalled).length;
  if (stalled > 0) {
    return {
      id: 'projections',
      status: 'warning',
      detail: { kind: 'projections_stalled', count: stalled }
    };
  }
  const catchingUp = info.projections.filter(
    (projection) => !projection.started || projection.lag > 0
  ).length;
  if (catchingUp > 0) {
    return {
      id: 'projections',
      status: 'ok',
      detail: { kind: 'projections_catching_up', count: catchingUp }
    };
  }
  return {
    id: 'projections',
    status: 'ok',
    detail: { kind: 'projections_running', count: info.projections.length }
  };
}

function workersCheck(info: AdminSystemInfo): HealthCheck {
  // Worker health comes from consumer state. Without it, the server reports
  // every required worker as unavailable, which says nothing about the workers.
  const active = info.natsAvailable
    ? info.durableWorkers.filter((worker) => worker.health !== 'inactive')
    : [];
  if (active.length === 0) {
    return { id: 'workers', status: 'unknown', detail: { kind: 'unavailable' } };
  }
  const failing = active.filter(
    (worker) => worker.health === 'stalled' || worker.health === 'unavailable'
  ).length;
  if (failing > 0) {
    return {
      id: 'workers',
      status: 'critical',
      detail: { kind: 'workers_failing', count: failing }
    };
  }
  // Broker state cannot tell a busy handler from a crashed one that waits for
  // redelivery, so unconfirmed workers stay ok but are not counted as running.
  const unconfirmed = active.filter((worker) => worker.health === 'unconfirmed').length;
  if (unconfirmed > 0) {
    return {
      id: 'workers',
      status: 'ok',
      detail: { kind: 'workers_unconfirmed', count: unconfirmed }
    };
  }
  return {
    id: 'workers',
    status: 'ok',
    detail: { kind: 'workers_running', count: active.length }
  };
}

function backlogCheck(info: AdminSystemInfo): HealthCheck {
  if (!info.natsAvailable) {
    return { id: 'backlog', status: 'unknown', detail: { kind: 'unavailable' } };
  }
  const waiting = info.nats.totalConsumerPending + info.nats.totalAckPending;
  return {
    id: 'backlog',
    status: 'ok',
    detail: waiting > 0 ? { kind: 'backlog_waiting', count: waiting } : { kind: 'backlog_clear' }
  };
}

function storageCheck(info: AdminSystemInfo): HealthCheck {
  if (!info.accountAvailable) {
    return { id: 'storage', status: 'unknown', detail: { kind: 'unavailable' } };
  }
  const usage = highestLimitUsage(info.account);
  if (usage === null) {
    return { id: 'storage', status: 'ok', detail: { kind: 'storage_unlimited' } };
  }
  const status =
    usage >= STORAGE_CRITICAL_RATIO
      ? 'critical'
      : usage >= STORAGE_WARNING_RATIO
        ? 'warning'
        : 'ok';
  return {
    id: 'storage',
    status,
    // Round down, so the shown percentage never reaches a threshold that the
    // status has not reached. The small tolerance keeps exact ratios such as
    // 29/100, which multiply to 28.999…, from losing a percent.
    detail: { kind: 'storage_used', percent: Math.floor(usage * 100 + 1e-9) }
  };
}

type SearchStatus = NonNullable<AdminSystemInfo['search']>;

/**
 * Search availability across all providers. A failed provider stays visible
 * when another provider is healthy:
 *
 * - No provider answered: critical, because search cannot work.
 * - No provider can answer queries: critical when one failed, a warning while
 *   providers start or build the index, unknown when no status is known.
 * - Some provider can answer queries: a warning for each failed, degraded, or
 *   unknown provider, in this order; otherwise ok. Providers that start or
 *   build the index next to a ready one are normal, for example in an update.
 */
export function searchCheck(search: SearchStatus): HealthCheck {
  const total = search.providers.length;
  if (total === 0) {
    return { id: 'search', status: 'critical', detail: { kind: 'search_none' } };
  }
  const count = (states: AdminSearchProvider['state'][]) =>
    search.providers.filter((provider) => states.includes(provider.state)).length;
  const ready = count(['ready']);
  const degraded = count(['degraded']);
  const failed = count(['unavailable']);
  const unknown = count(['unknown']);
  const check = (
    status: HealthStatus,
    kind: SearchProviderDetail,
    providers: number
  ): HealthCheck => ({ id: 'search', status, detail: { kind, count: providers, total } });

  if (ready + degraded === 0) {
    if (failed > 0) return check('critical', 'search_failed', failed);
    const indexing = count(['starting', 'indexing']);
    if (indexing > 0) return check('warning', 'search_indexing', indexing);
    return check('unknown', 'search_unknown', unknown);
  }
  if (failed > 0) return check('warning', 'search_failed', failed);
  if (degraded > 0) return check('warning', 'search_degraded', degraded);
  if (unknown > 0) return check('warning', 'search_unknown', unknown);
  return check('ok', 'search_ready', ready);
}

/**
 * Search index freshness across all providers. A stalled index is a warning,
 * like a stalled projection. An index that catches up is ok. When some
 * providers report freshness and others do not, the unknown ones are a
 * warning; when none reports it, the check is unknown.
 */
export function searchIndexCheck(search: SearchStatus): HealthCheck {
  const total = search.providers.length;
  if (total === 0) {
    return { id: 'search_index', status: 'unknown', detail: { kind: 'unavailable' } };
  }
  const count = (freshness: AdminSearchProvider['freshness']) =>
    search.providers.filter((provider) => provider.freshness === freshness).length;
  const check = (
    status: HealthStatus,
    kind: SearchIndexDetail,
    providers: number
  ): HealthCheck => ({
    id: 'search_index',
    status,
    detail: { kind, count: providers, total }
  });

  const unknown = count('unknown');
  if (count('stalled') > 0) return check('warning', 'search_index_stalled', count('stalled'));
  if (unknown === total) return check('unknown', 'search_index_unknown', unknown);
  if (unknown > 0) return check('warning', 'search_index_unknown', unknown);
  if (count('behind') > 0) return check('ok', 'search_index_behind', count('behind'));
  return check('ok', 'search_index_current', count('current'));
}

function voiceCallsCheck({ livekit }: AdminSystemInfo): HealthCheck | null {
  switch (livekit.connectionState) {
    case 'unavailable':
    case 'not_configured':
      return null;
    case 'unreachable':
      return { id: 'voice_calls', status: 'critical', detail: { kind: 'livekit_unreachable' } };
    case 'unauthorized':
      return { id: 'voice_calls', status: 'critical', detail: { kind: 'livekit_unauthorized' } };
    case 'error':
      return { id: 'voice_calls', status: 'critical', detail: { kind: 'livekit_error' } };
    case 'ok':
      if (livekit.insecureUrl) {
        return { id: 'voice_calls', status: 'warning', detail: { kind: 'livekit_insecure_url' } };
      }
      if (liveKitWebhooksRejected(livekit)) {
        return {
          id: 'voice_calls',
          status: 'warning',
          detail: { kind: 'livekit_webhooks_rejected' }
        };
      }
      return { id: 'voice_calls', status: 'ok', detail: { kind: 'livekit_connected' } };
  }
}

/**
 * Reports whether the most recent LiveKit webhook had an invalid signature.
 * An accepted webhook after a rejected one shows that the signing key now
 * matches.
 */
export function liveKitWebhooksRejected(livekit: AdminSystemInfo['livekit']): boolean {
  const rejected = livekit.lastRejectedWebhookAt;
  if (!rejected) return false;
  const accepted = livekit.lastWebhookAt;
  return !accepted || rejected.getTime() > accepted.getTime();
}
