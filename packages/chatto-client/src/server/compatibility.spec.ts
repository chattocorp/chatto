import { describe, expect, it } from 'vitest';
import {
  compareReleaseVersions,
  serverFeatureSupport,
  limitedServerFeatures,
  limitedFrontendFeatures,
  SERVER_FEATURE_REQUIREMENTS,
  evaluateServerCompatibility,
  isSupportedServerVersion,
  isConnectableServerVersion,
  normalizeServerVersion,
  MINIMUM_CONNECTABLE_SERVER_VERSION,
  MINIMUM_SUPPORTED_SERVER_VERSION
} from './compatibility.js';

describe('bundled frontend feature guidance', () => {
  const future = {
    id: 'unknown-future-feature',
    name: 'Future feature',
    description: 'A feature the client does not know.',
    introducedVersion: '0.6.0-beta.2'
  };

  it.each([
    ['0.5.0', true],
    ['0.6.0-beta.1', true],
    ['0.6.0-beta.2', false],
    ['0.6.0-beta.2-custom+build.1', false],
    ['0.6.0', false],
    ['dev', false],
    ['', false]
  ])('compares frontend %s without requiring a known feature ID', (version, missing) => {
    expect(limitedFrontendFeatures([future], version)).toEqual(missing ? [future] : []);
  });

  it('ignores invalid entries and duplicate IDs', () => {
    expect(
      limitedFrontendFeatures(
        [
          { ...future, id: '' },
          { ...future, id: 'empty-name', name: ' ' },
          { ...future, id: 'unknown-version', introducedVersion: 'dev' },
          future,
          future
        ],
        '0.5.0'
      )
    ).toEqual([future]);
    expect(limitedFrontendFeatures([], '0.5.0')).toEqual([]);
  });
});

describe('server compatibility evaluation', () => {
  it.each([
    ['0.5.0-beta.11', 'limited', ['threadViews', 'independentSlowMode']],
    ['0.5.0-beta.15-custom', 'limited', ['threadViews', 'independentSlowMode']],
    ['0.5.0-beta.16', 'supported', ['independentSlowMode']],
    ['0.5.0-beta.16-custom+build.2', 'supported', ['independentSlowMode']],
    ['0.5.0', 'supported', []],
    ['dev', 'unknown', []],
    ['', 'unknown', []]
  ] as const)('reports feature support for %s', (version, support, limited) => {
    expect(serverFeatureSupport(version, 'threadViews')).toBe(support);
    expect(limitedServerFeatures(version)).toEqual(limited);
  });

  it('derives full support from the feature requirements', () => {
    for (const required of Object.values(SERVER_FEATURE_REQUIREMENTS)) {
      expect(
        compareReleaseVersions(MINIMUM_SUPPORTED_SERVER_VERSION, required)
      ).toBeGreaterThanOrEqual(0);
    }
    expect(limitedServerFeatures(MINIMUM_SUPPORTED_SERVER_VERSION)).toEqual([]);
  });

  it('reports independent slow mode without blocking older compatible servers', () => {
    expect(serverFeatureSupport('0.5.0-beta.17', 'independentSlowMode')).toBe('limited');
    expect(serverFeatureSupport('0.5.0-beta.18', 'independentSlowMode')).toBe('supported');
    expect(serverFeatureSupport('dev', 'independentSlowMode')).toBe('unknown');
    expect(limitedServerFeatures('0.5.0-beta.18')).toEqual([]);
    expect(isConnectableServerVersion('0.5.0-beta.17')).toBe(true);
    expect(MINIMUM_CONNECTABLE_SERVER_VERSION).toBe('0.5.0-beta.9');
    expect(MINIMUM_SUPPORTED_SERVER_VERSION).toBe('0.5.0-beta.18');
  });

  it.each([
    ['0.6.1-my-customization', '0.6.1'],
    ['0.6.1+my-customization', '0.6.1'],
    ['0.6.1-beta.2-my-customization', '0.6.1-beta.2'],
    ['0.6.1-beta.2', '0.6.1-beta.2'],
    [' v0.6.1-rc.3-custom+build.4 ', '0.6.1-rc.3'],
    ['0.6.1-alpha.2-custom', '0.6.1-alpha.2'],
    ['dev', null],
    ['0.6', null],
    ['0.6.1-beta.02', null],
    ['0.6.1-custom garbage', null]
  ])('normalizes %s to %s', (input, expected) => {
    expect(normalizeServerVersion(input!)).toBe(expected);
  });

  it('separates the connection floor from full support', () => {
    expect(
      compareReleaseVersions(MINIMUM_CONNECTABLE_SERVER_VERSION, MINIMUM_SUPPORTED_SERVER_VERSION)
    ).toBe(-1);
    expect(isConnectableServerVersion('0.5.0-beta.8-custom')).toBe(false);
    expect(isConnectableServerVersion(MINIMUM_CONNECTABLE_SERVER_VERSION)).toBe(true);
    expect(isConnectableServerVersion('dev')).toBe(true);
    expect(
      evaluateServerCompatibility({ serverVersion: MINIMUM_CONNECTABLE_SERVER_VERSION })
    ).toEqual({ status: 'limited', reason: 'server-too-old' });
    expect(evaluateServerCompatibility({ serverVersion: '0.5.0-beta.15' }).status).toBe('limited');
    expect(
      evaluateServerCompatibility({ serverVersion: MINIMUM_SUPPORTED_SERVER_VERSION }).status
    ).toBe('supported');
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

  it('rejects prereleases older than the supported minimum', () => {
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0-beta.8'
      })
    ).toEqual({ status: 'unsupported', reason: 'server-too-old' });
  });

  it('reports supported versions only for parseable releases at or above the minimum', () => {
    expect(isSupportedServerVersion('0.5.0-beta.18')).toBe(true);
    expect(isSupportedServerVersion('0.5.0-beta.17')).toBe(false);
    expect(isSupportedServerVersion('0.5.0-beta.15')).toBe(false);
    expect(isSupportedServerVersion('0.5.0-dev')).toBe(true);
    expect(isSupportedServerVersion('0.5.0')).toBe(true);
    expect(isSupportedServerVersion('0.5.0-beta.8')).toBe(false);
    expect(isSupportedServerVersion('0.4.19')).toBe(false);
    expect(isSupportedServerVersion('custom-build')).toBe(false);
    expect(isSupportedServerVersion('')).toBe(false);
  });
});
