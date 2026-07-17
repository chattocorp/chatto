import frontendPackage from '../../../../package.json';
import compare from 'semver/functions/compare.js';
import valid from 'semver/functions/valid.js';

export const CHATTO_WEB_CLIENT_VERSION = frontendPackage.version;
export const LEGACY_SERVER_WARNING_BEFORE_VERSION = '0.5.0';
export const REALTIME_PROJECTION_CAPABILITY = 'chatto.realtime.projection.v1';

export const REQUIRED_PROTOCOL_CAPABILITIES = [
  'chatto.api.v1',
  REALTIME_PROJECTION_CAPABILITY
] as const;
export const RECOMMENDED_PROTOCOL_CAPABILITIES = ['chatto.realtime.v1'] as const;

export type ServerCompatibilityStatus =
  | 'supported'
  | 'degraded'
  | 'unsupported'
  | 'unknown'
  | 'unreachable';

export type ServerCompatibilityReason =
  | 'capabilities-confirmed'
  | 'missing-required-capabilities'
  | 'missing-recommended-capabilities'
  | 'server-too-old'
  | 'web-client-too-old'
  | 'legacy-server'
  | 'unreachable';

export type ServerCompatibilityResult = {
  status: ServerCompatibilityStatus;
  reason: ServerCompatibilityReason;
  missingCapabilities: string[];
};

export type ServerCompatibilityInput = {
  serverVersion: string;
  protocolCapabilities: readonly string[] | null;
  minimumWebClientVersion: string | null;
  webClientVersion?: string;
  unreachable?: boolean;
};

export function compareReleaseVersions(left: string, right: string): number | null {
  const parsedLeft = valid(left.trim());
  const parsedRight = valid(right.trim());
  if (!parsedLeft || !parsedRight) return null;
  return compare(parsedLeft, parsedRight);
}

export function evaluateServerCompatibility(
  input: ServerCompatibilityInput
): ServerCompatibilityResult {
  if (input.unreachable) {
    return { status: 'unreachable', reason: 'unreachable', missingCapabilities: [] };
  }

  const webClientVersion = input.webClientVersion ?? CHATTO_WEB_CLIENT_VERSION;
  if (
    input.minimumWebClientVersion &&
    compareReleaseVersions(webClientVersion, input.minimumWebClientVersion) === -1
  ) {
    return { status: 'unsupported', reason: 'web-client-too-old', missingCapabilities: [] };
  }

  if (input.protocolCapabilities !== null) {
    const advertised = new Set(input.protocolCapabilities);
    const missingRequired = REQUIRED_PROTOCOL_CAPABILITIES.filter(
      (capability) => !advertised.has(capability)
    );
    if (missingRequired.length > 0) {
      return {
        status: 'unsupported',
        reason: 'missing-required-capabilities',
        missingCapabilities: missingRequired
      };
    }

    const missingRecommended = RECOMMENDED_PROTOCOL_CAPABILITIES.filter(
      (capability) => !advertised.has(capability)
    );
    if (missingRecommended.length > 0) {
      return {
        status: 'degraded',
        reason: 'missing-recommended-capabilities',
        missingCapabilities: missingRecommended
      };
    }

    return {
      status: 'supported',
      reason: 'capabilities-confirmed',
      missingCapabilities: []
    };
  }

  if (compareReleaseVersions(input.serverVersion, LEGACY_SERVER_WARNING_BEFORE_VERSION) === -1) {
    return { status: 'unsupported', reason: 'server-too-old', missingCapabilities: [] };
  }

  return { status: 'unknown', reason: 'legacy-server', missingCapabilities: [] };
}

export function hasProtocolCapability(
  capabilities: readonly string[] | null,
  capability: string
): boolean | null {
  return capabilities === null ? null : capabilities.includes(capability);
}
