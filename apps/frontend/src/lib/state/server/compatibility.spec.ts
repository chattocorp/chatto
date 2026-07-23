import { describe, expect, it } from 'vitest';
import {
  compareReleaseVersions,
  evaluateServerCompatibility,
  hasProtocolCapability,
  ROOM_MANAGER_MEMBER_READS_CAPABILITY,
  supportsRoomManagerMemberReads
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

  it('accepts a server that advertises the required and recommended protocols', () => {
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0',
        protocolCapabilities: [
          'chatto.api.v1',
          'chatto.realtime.v1',
          'chatto.realtime.projection.v1'
        ],
        minimumWebClientVersion: null,
        webClientVersion: '0.5.0'
      })
    ).toEqual({
      status: 'supported',
      reason: 'capabilities-confirmed',
      missingCapabilities: []
    });
  });

  it('degrades when optional realtime support is unavailable', () => {
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0',
        protocolCapabilities: ['chatto.api.v1', 'chatto.realtime.projection.v1'],
        minimumWebClientVersion: null,
        webClientVersion: '0.5.0'
      })
    ).toMatchObject({
      status: 'degraded',
      reason: 'missing-recommended-capabilities',
      missingCapabilities: ['chatto.realtime.v1']
    });
  });

  it('rejects advertised metadata without the required ConnectRPC API', () => {
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0',
        protocolCapabilities: ['chatto.discovery.v1'],
        minimumWebClientVersion: null,
        webClientVersion: '0.5.0'
      })
    ).toMatchObject({ status: 'unsupported', reason: 'missing-required-capabilities' });
  });

  it('rejects a server without the required projection stream', () => {
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0',
        protocolCapabilities: ['chatto.api.v1', 'chatto.realtime.v1'],
        minimumWebClientVersion: null,
        webClientVersion: '0.5.0'
      })
    ).toMatchObject({
      status: 'unsupported',
      reason: 'missing-required-capabilities',
      missingCapabilities: ['chatto.realtime.projection.v1']
    });
  });

  it('uses the server version only for legacy discovery responses', () => {
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.4.12',
        protocolCapabilities: null,
        minimumWebClientVersion: null,
        webClientVersion: '0.5.0'
      })
    ).toMatchObject({ status: 'unsupported', reason: 'server-too-old' });

    expect(
      evaluateServerCompatibility({
        serverVersion: 'custom-build',
        protocolCapabilities: null,
        minimumWebClientVersion: null,
        webClientVersion: '0.5.0'
      })
    ).toMatchObject({ status: 'unknown', reason: 'legacy-server' });
  });

  it('honours a server-declared minimum bundled web-client version', () => {
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.6.0',
        protocolCapabilities: [
          'chatto.api.v1',
          'chatto.realtime.v1',
          'chatto.realtime.projection.v1'
        ],
        minimumWebClientVersion: '0.6.0',
        webClientVersion: '0.5.0'
      })
    ).toMatchObject({ status: 'unsupported', reason: 'web-client-too-old' });

    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0-beta.3',
        protocolCapabilities: [
          'chatto.api.v1',
          'chatto.realtime.v1',
          'chatto.realtime.projection.v1'
        ],
        minimumWebClientVersion: '0.5.0-beta.3',
        webClientVersion: '0.5.0-beta.1'
      })
    ).toMatchObject({ status: 'unsupported', reason: 'web-client-too-old' });

    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0',
        protocolCapabilities: [
          'chatto.api.v1',
          'chatto.realtime.v1',
          'chatto.realtime.projection.v1'
        ],
        minimumWebClientVersion: '0.5.0',
        webClientVersion: '0.5.0-rc.1'
      })
    ).toMatchObject({ status: 'unsupported', reason: 'web-client-too-old' });
  });

  it('reports unreachable servers separately from compatibility', () => {
    expect(
      evaluateServerCompatibility({
        serverVersion: '0.5.0',
        protocolCapabilities: [
          'chatto.api.v1',
          'chatto.realtime.v1',
          'chatto.realtime.projection.v1'
        ],
        minimumWebClientVersion: null,
        unreachable: true
      })
    ).toMatchObject({ status: 'unreachable', reason: 'unreachable' });
  });

  it('distinguishes absent capability metadata from a missing capability', () => {
    expect(hasProtocolCapability(null, 'chatto.realtime.v1')).toBeNull();
    expect(hasProtocolCapability([], 'chatto.realtime.v1')).toBe(false);
    expect(hasProtocolCapability(['chatto.realtime.v1'], 'chatto.realtime.v1')).toBe(true);
  });

  it('gates manager member reads by capability with a legacy version fallback', () => {
    expect(supportsRoomManagerMemberReads([ROOM_MANAGER_MEMBER_READS_CAPABILITY], '0.4.0')).toBe(
      true
    );
    expect(supportsRoomManagerMemberReads(['chatto.api.v1'], '0.5.0')).toBe(false);
    expect(supportsRoomManagerMemberReads(null, '0.5.0')).toBe(true);
    expect(supportsRoomManagerMemberReads(null, '0.4.12')).toBe(false);
    expect(supportsRoomManagerMemberReads(null, 'custom-build')).toBe(false);
  });
});
