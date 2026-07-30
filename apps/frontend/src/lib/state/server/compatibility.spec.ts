import { version as configuredWebClientVersion } from '$app/environment';
import { describe, expect, it } from 'vitest';
import {
  CHATTO_WEB_CLIENT_VERSION,
  compareReleaseVersions,
  evaluateServerCompatibility,
  supportsServerFeature
} from './compatibility';

describe('server compatibility evaluation', () => {
  it('uses the configured SvelteKit build version by default', () => {
    expect(CHATTO_WEB_CLIENT_VERSION).toBe(configuredWebClientVersion);
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0',
        minimumWebClientVersion: CHATTO_WEB_CLIENT_VERSION
      })
    ).toEqual({
      status: 'supported',
      reason: 'version-confirmed'
    });
  });

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
        serverVersion: '0.5.0',
        minimumWebClientVersion: null,
        webClientVersion: '0.5.0'
      })
    ).toEqual({
      status: 'supported',
      reason: 'version-confirmed'
    });

    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0-dev',
        minimumWebClientVersion: null,
        webClientVersion: '0.5.0'
      })
    ).toEqual({
      status: 'supported',
      reason: 'version-confirmed'
    });

    expect(
      evaluateServerCompatibility({
        serverVersion: '0.6.0',
        minimumWebClientVersion: null,
        webClientVersion: '0.5.0'
      })
    ).toEqual({
      status: 'supported',
      reason: 'version-confirmed'
    });
  });

  it('rejects pre-0.5 servers and preserves unknown custom versions', () => {
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.4.19',
        minimumWebClientVersion: null,
        webClientVersion: '0.5.0'
      })
    ).toEqual({ status: 'unsupported', reason: 'server-too-old' });

    expect(
      evaluateServerCompatibility({
        serverVersion: 'custom-build',
        minimumWebClientVersion: null,
        webClientVersion: '0.5.0'
      })
    ).toEqual({ status: 'unknown', reason: 'server-version-unknown' });
  });

  it('honours a server-declared minimum bundled web-client version', () => {
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.6.0',
        minimumWebClientVersion: '0.6.0',
        webClientVersion: '0.5.0'
      })
    ).toEqual({ status: 'unsupported', reason: 'web-client-too-old' });

    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0-beta.3',
        minimumWebClientVersion: '0.5.0-beta.3',
        webClientVersion: '0.5.0-beta.1'
      })
    ).toEqual({ status: 'unsupported', reason: 'web-client-too-old' });
  });

  it('reports unreachable servers separately from compatibility', () => {
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0',
        minimumWebClientVersion: null,
        unreachable: true
      })
    ).toEqual({ status: 'unreachable', reason: 'unreachable' });
  });

  it('derives feature support from the server release that introduced it', () => {
    expect(supportsServerFeature('0.5.0-beta.1', 'realtimeProjection')).toBe(true);
    expect(supportsServerFeature('0.5.0', 'messageSearch')).toBe(true);
    expect(supportsServerFeature('0.5.0', 'roomManagement')).toBe(true);
    expect(supportsServerFeature('0.4.19', 'messageSearch')).toBe(false);
    expect(supportsServerFeature('custom-build', 'adminApi')).toBe(false);
  });
});
