import compare from 'semver/functions/compare.js';
import valid from 'semver/functions/valid.js';

/**
 * Oldest server release this client supports. Every feature the client uses
 * exists in this release, so the client does not gate individual features by
 * server version. Raise it when the client starts to depend on a newer server.
 */
export const MINIMUM_SUPPORTED_SERVER_VERSION = '0.5.0-beta.9';

export type ServerCompatibilityStatus = 'supported' | 'unsupported' | 'unknown' | 'unreachable';

export type ServerCompatibilityReason =
  'version-confirmed' | 'server-too-old' | 'server-version-unknown' | 'unreachable';

/** A reason that prevents this client from using a server. */
export type ServerCompatibilityProblem = Exclude<ServerCompatibilityReason, 'version-confirmed'>;

export type ServerCompatibilityResult = {
  status: ServerCompatibilityStatus;
  reason: ServerCompatibilityReason;
};

export type ServerCompatibilityInput = {
  serverVersion: string;
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
    return { status: 'unreachable', reason: 'unreachable' };
  }

  const comparison = compareReleaseVersions(input.serverVersion, MINIMUM_SUPPORTED_SERVER_VERSION);
  if (comparison === null) {
    return { status: 'unknown', reason: 'server-version-unknown' };
  }
  if (comparison === -1) {
    return { status: 'unsupported', reason: 'server-too-old' };
  }

  return { status: 'supported', reason: 'version-confirmed' };
}

/** Whether a server reports a parseable release at or above the supported minimum. */
export function isSupportedServerVersion(serverVersion: string): boolean {
  const comparison = compareReleaseVersions(serverVersion, MINIMUM_SUPPORTED_SERVER_VERSION);
  return comparison !== null && comparison >= 0;
}
