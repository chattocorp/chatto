import compare from 'semver/functions/compare.js';
import valid from 'semver/functions/valid.js';

/**
 * Oldest server release with full client feature support. Older connectable
 * releases show a warning; individual features do not have version gates.
 */
export const MINIMUM_SUPPORTED_SERVER_VERSION = '0.5.0-beta.16';

/**
 * Oldest release allowed to connect. This was the full-support baseline before
 * beta.16 added the thread views; its authentication and realtime contracts
 * remain compatible. Do not lower it without verifying the older contracts.
 */
export const MINIMUM_CONNECTABLE_SERVER_VERSION = '0.5.0-beta.9';

export type ServerCompatibilityStatus =
  'supported' | 'limited' | 'unsupported' | 'unknown' | 'unreachable';

export type ServerCompatibilityReason =
  'version-confirmed' | 'server-too-old' | 'server-version-unknown' | 'unreachable';

/** A compatibility warning or connection failure; not every reason blocks use. */
export type ServerCompatibilityProblem = Exclude<ServerCompatibilityReason, 'version-confirmed'>;

export type ServerCompatibilityResult = {
  status: ServerCompatibilityStatus;
  reason: ServerCompatibilityReason;
};

export type ServerCompatibilityInput = {
  serverVersion: string;
  unreachable?: boolean;
};

/**
 * Read a release version from a custom build. Preserve alpha, beta, and rc
 * stages (and their numeric identifiers), but ignore trailing customization
 * labels and build metadata. Malformed SemVer remains unknown.
 */
export function normalizeServerVersion(version: string): string | null {
  const parsed = valid(version.trim());
  if (!parsed) return null;
  const match = /^(\d+\.\d+\.\d+)(?:-(alpha|beta|rc)((?:\.\d+)*)(?=[.+-]|$))?/.exec(parsed);
  if (!match) return null;
  return valid(`${match[1]}${match[2] ? `-${match[2]}${match[3]}` : ''}`);
}

/** Compare normalized server releases; null means at least one is unknown. */
export function compareReleaseVersions(left: string, right: string): number | null {
  const parsedLeft = normalizeServerVersion(left);
  const parsedRight = normalizeServerVersion(right);
  if (!parsedLeft || !parsedRight) return null;
  return compare(parsedLeft, parsedRight);
}

export function evaluateServerCompatibility(
  input: ServerCompatibilityInput
): ServerCompatibilityResult {
  if (input.unreachable) {
    return { status: 'unreachable', reason: 'unreachable' };
  }

  const comparison = compareReleaseVersions(input.serverVersion, MINIMUM_SUPPORTED_SERVER_VERSION);
  if (comparison === null) {
    return { status: 'unknown', reason: 'server-version-unknown' };
  }
  if (comparison === -1) {
    return {
      status: isConnectableServerVersion(input.serverVersion) ? 'limited' : 'unsupported',
      reason: 'server-too-old'
    };
  }

  return { status: 'supported', reason: 'version-confirmed' };
}

/** Whether a server reports a parseable release at or above the supported minimum. */
export function isSupportedServerVersion(serverVersion: string): boolean {
  const comparison = compareReleaseVersions(serverVersion, MINIMUM_SUPPORTED_SERVER_VERSION);
  return comparison !== null && comparison >= 0;
}

/** Unknown custom builds may try; known releases below the floor may not. */
export function isConnectableServerVersion(serverVersion: string): boolean {
  const comparison = compareReleaseVersions(serverVersion, MINIMUM_CONNECTABLE_SERVER_VERSION);
  return comparison === null || comparison >= 0;
}
