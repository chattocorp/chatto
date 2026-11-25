import type { AdminSystemInfo } from '$lib/api/adminDiagnostics';
import type { HealthCheck, HealthCheckId, HealthStatus } from './systemHealth';

/** Identifies one tab of the System page. Each section has its own route. */
export type SystemSection =
  'overview' | 'streams' | 'projections' | 'workers' | 'livekit' | 'attachmentUploads';

const base = '/chat/[serverId]/manage/server/system' as const;

/** Route ID of each section, for `resolve()` with the `serverId` parameter. */
export const systemSectionRoutes = {
  overview: base,
  streams: `${base}/streams`,
  projections: `${base}/projections`,
  workers: `${base}/workers`,
  livekit: `${base}/livekit`,
  attachmentUploads: `${base}/attachment-uploads`
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
  voice_calls: 'livekit'
};

/**
 * Returns the sections that the System page shows as tabs, in tab order. The
 * Optional sections need server support unless they are the current section,
 * so a direct visit always has a tab.
 */
export function visibleSystemSections(
  info: AdminSystemInfo | null,
  current: SystemSection | null = null
): SystemSection[] {
  const sections: SystemSection[] = ['overview', 'streams', 'projections', 'workers'];
  if ((info && info.livekit.connectionState !== 'unavailable') || current === 'livekit') {
    sections.push('livekit');
  }
  if (info?.attachmentUploadsAvailable || current === 'attachmentUploads') {
    sections.push('attachmentUploads');
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
