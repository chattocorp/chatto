import type { AdminSystemInfo } from '$lib/api/adminDiagnostics';
import type { HealthCheck, HealthCheckId, HealthStatus } from './systemHealth';

/** Identifies one tab of the System page. Each section has its own route. */
export type SystemSection =
  'overview' | 'streams' | 'projections' | 'workers' | 'search' | 'livekit';

const base = '/chat/[serverId]/manage/server/system' as const;

/** Route ID of each section, for `resolve()` with the `serverId` parameter. */
export const systemSectionRoutes = {
  overview: base,
  streams: `${base}/streams`,
  projections: `${base}/projections`,
  workers: `${base}/workers`,
  search: `${base}/search`,
  livekit: `${base}/livekit`
} as const satisfies Record<SystemSection, string>;

/**
 * The section that shows the details of each health check. The Overview
 * section shows the broker connection and the account limits itself.
 */
export const healthCheckSection: Record<HealthCheckId, SystemSection> = {
  broker: 'overview',
  storage: 'overview',
  backlog: 'streams',
  projections: 'projections',
  workers: 'workers',
  voice_calls: 'livekit',
  search: 'search',
  search_index: 'search'
};

/**
 * Returns the sections that the System page shows as tabs, in tab order. The
 * Search section is absent when the server does not report search status, and
 * the LiveKit section when it does not report LiveKit status, unless the
 * section is the current one, so the current page always has a tab.
 */
export function visibleSystemSections(
  info: AdminSystemInfo | null,
  current: SystemSection | null = null
): SystemSection[] {
  const sections: SystemSection[] = ['overview', 'streams', 'projections', 'workers'];
  if (info?.search || current === 'search') sections.push('search');
  if ((info && info.livekit.connectionState !== 'unavailable') || current === 'livekit') {
    sections.push('livekit');
  }
  return sections;
}

/**
 * Returns the most severe status of the health checks that a section shows,
 * or `null` when none of them is a warning or a problem.
 */
export function systemSectionStatus(
  checks: HealthCheck[],
  section: SystemSection
): Extract<HealthStatus, 'warning' | 'critical'> | null {
  const statuses = checks
    .filter((check) => healthCheckSection[check.id] === section)
    .map((check) => check.status);
  if (statuses.includes('critical')) return 'critical';
  if (statuses.includes('warning')) return 'warning';
  return null;
}
