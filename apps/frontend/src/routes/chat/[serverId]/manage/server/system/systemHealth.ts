import type { AdminSystemInfo } from '$lib/api/adminDiagnostics';

/**
 * Severity of one health check. `unknown` means the server did not report
 * the data that the check needs; it never raises the overall status.
 */
export type HealthStatus = 'ok' | 'warning' | 'critical' | 'unknown';

/** Identifies a health check, so the view can choose its label and icon. */
export type HealthCheckId = 'broker' | 'projections' | 'workers' | 'backlog' | 'storage';

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
  | { kind: 'projections_catching_up'; count: number }
  | { kind: 'projections_running'; count: number }
  | { kind: 'workers_failing'; count: number }
  | { kind: 'workers_unconfirmed'; count: number }
  | { kind: 'workers_running'; count: number }
  | { kind: 'backlog_waiting'; count: number }
  | { kind: 'backlog_clear' }
  | { kind: 'storage_used'; percent: number }
  | { kind: 'storage_unlimited' };

/** Usage at or above this share of an account limit is a warning. */
export const STORAGE_WARNING_RATIO = 0.75;
/** Usage at or above this share of an account limit is critical. */
export const STORAGE_CRITICAL_RATIO = 0.9;

const severity: Record<HealthStatus, number> = { unknown: 0, ok: 1, warning: 2, critical: 3 };

/**
 * Derives the System page health checks from one diagnostics snapshot.
 *
 * Only lasting conditions raise a check above `ok`: a lost broker
 * connection, failed projections, stalled or missing durable workers, and
 * account usage close to a limit. Projection lag, consumer backlog, and
 * unconfirmed workers are normal while work flows, so they appear only as
 * details.
 *
 * The broker and projection checks describe the replica that handled the
 * request. The other checks use broker state that every replica shares.
 */
export function systemHealthChecks(info: AdminSystemInfo): HealthCheck[] {
  return [
    brokerCheck(info),
    projectionsCheck(info),
    workersCheck(info),
    backlogCheck(info),
    storageCheck(info)
  ];
}

/** Returns the most severe status of the checks, or `ok` when all are unknown. */
export function overallHealth(checks: HealthCheck[]): Exclude<HealthStatus, 'unknown'> {
  const worst = checks.reduce<HealthStatus>(
    (current, check) => (severity[check.status] > severity[current] ? check.status : current),
    'ok'
  );
  return worst === 'unknown' ? 'ok' : worst;
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
