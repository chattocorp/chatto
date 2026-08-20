import { describe, expect, it } from 'vitest';
import {
  compareReleaseVersions,
  evaluateServerCompatibility,
  supportsServerFeature
} from './compatibility';

describe('server compatibility evaluation', () => {
  it('uses full SemVer prerelease precedence', () => {
    expect(compareReleaseVersions('v0.5.0', '0.4.12')).toBe(1);
    expect(compareReleaseVersions('0.5.0-beta.1', '0.5.0-beta.2')).toBe(-1);
    expect(compareReleaseVersions('0.5.0-beta.2', '0.5.0-beta.10')).toBe(-1);
    expect(compareReleaseVersions('0.5.0-beta.10', '0.5.0-rc.1')).toBe(-1);
    expect(compareReleaseVersions('0.5.0-rc.1', '0.5.0')).toBe(-1);
  });

  it('ignores build metadata and rejects malformed versions', () => {
    expect(compareReleaseVersions('0.5.0+build.1', '0.5.0+build.2')).toBe(0);
    expect(compareReleaseVersions('0.5.0-beta.1+build.1', '0.5.0-beta.1+build.2')).toBe(0);
    expect(compareReleaseVersions('unknown', '0.5.0')).toBeNull();
  });

  it('accepts servers at or above the 0.5 compatibility baseline', () => {
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0'
      })
    ).toEqual({
      status: 'supported',
      reason: 'version-confirmed'
    });

    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0-dev'
      })
    ).toEqual({
      status: 'supported',
      reason: 'version-confirmed'
    });

    expect(
      evaluateServerCompatibility({
        serverVersion: '0.6.0'
      })
    ).toEqual({
      status: 'supported',
      reason: 'version-confirmed'
    });
  });

  it('gates author-created threads to 0.5 servers', () => {
    expect(supportsServerFeature('0.5.0', 'threadCreation')).toBe(true);
    expect(supportsServerFeature('0.4.9', 'threadCreation')).toBe(false);
  });

  it('rejects pre-0.5 servers and preserves unknown custom versions', () => {
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.4.19'
      })
    ).toEqual({ status: 'unsupported', reason: 'server-too-old' });

    expect(
      evaluateServerCompatibility({
        serverVersion: 'custom-build'
      })
    ).toEqual({ status: 'unknown', reason: 'server-version-unknown' });
  });

  it('reports unreachable servers separately from compatibility', () => {
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0',
        unreachable: true
      })
    ).toEqual({ status: 'unreachable', reason: 'unreachable' });
  });

  it('derives feature support from the server release that introduced it', () => {
    expect(supportsServerFeature('0.5.0-beta.1', 'realtimeProjection')).toBe(true);
    expect(supportsServerFeature('0.5.0', 'messageSearch')).toBe(true);
    expect(supportsServerFeature('0.5.0', 'roomManagement')).toBe(true);
    expect(supportsServerFeature('0.5.0', 'serverInvitations')).toBe(true);
    expect(supportsServerFeature('0.4.19', 'messageSearch')).toBe(false);
    expect(supportsServerFeature('custom-build', 'adminApi')).toBe(false);
  });
});
