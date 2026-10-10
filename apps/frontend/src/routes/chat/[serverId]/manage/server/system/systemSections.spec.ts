import { describe, expect, it } from 'vitest';
import type { AdminSystemInfo } from '$lib/api/adminDiagnostics';
import type { HealthCheck } from './systemHealth';
import { systemSectionStatus, visibleSystemSections } from './systemSections';

function check(id: HealthCheck['id'], status: HealthCheck['status']): HealthCheck {
  return { id, status, detail: { kind: 'unavailable' } };
}

describe('systemSectionStatus', () => {
  it('returns the most severe status of the checks that the section explains', () => {
    const checks = [check('broker', 'warning'), check('storage', 'critical')];
    expect(systemSectionStatus(checks, 'overview')).toBe('critical');
    expect(systemSectionStatus([check('backlog', 'warning')], 'streams')).toBe('warning');
  });

  it('ignores checks of other sections and checks without a problem', () => {
    const checks = [check('projections', 'critical'), check('workers', 'ok')];
    expect(systemSectionStatus(checks, 'workers')).toBeNull();
    expect(systemSectionStatus([check('voice_calls', 'unknown')], 'livekit')).toBeNull();
  });
});

describe('visibleSystemSections', () => {
  const withLiveKit = (connectionState: AdminSystemInfo['livekit']['connectionState']) =>
    ({ livekit: { connectionState } }) as AdminSystemInfo;

  it('shows the LiveKit section only when the server reports LiveKit status', () => {
    expect(visibleSystemSections(withLiveKit('not_configured'))).toContain('livekit');
    expect(visibleSystemSections(withLiveKit('unavailable'))).not.toContain('livekit');
    expect(visibleSystemSections(null)).toEqual(['overview', 'streams', 'projections', 'workers']);
  });
});
