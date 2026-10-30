import compare from 'semver/functions/compare.js';
import valid from 'semver/functions/valid.js';
import type { PublicServerFeature } from '../api/server.js';

/**
 * Features introduced after a bundled frontend release. Unknown feature IDs
 * are expected. Ignore incomplete entries, unknown versions, and duplicate IDs.
 * Third-party clients must use their own feature policy instead of this helper.
 */
export function limitedFrontendFeatures(
  catalog: readonly PublicServerFeature[],
  frontendVersion: string
): PublicServerFeature[] {
  const seen = new Set<string>();
  return catalog.filter((feature) => {
    if (!feature.id.trim() || !feature.name.trim() || seen.has(feature.id)) return false;
    seen.add(feature.id);
    return compareReleaseVersions(frontendVersion, feature.introducedVersion) === -1;
  });
}

/**
 * Minimum releases for individual client capabilities. Add an entry when a
 * server change affects a feature without preventing the client from connecting.
 * These requirements describe version support, not permissions or configuration.
 */
export const SERVER_FEATURE_REQUIREMENTS = {
  /** Root-author filtering for separate followed and own thread lists (#2983). */
  threadViews: '0.5.0-beta.16',
  /** Separate channel and per-thread cooldowns, with legacy field-presence fallback (#3036). */
  independentSlowMode: '0.5.0-beta.18'
} as const;

/** A capability whose server requirement the client knows. */
export type ServerFeature = keyof typeof SERVER_FEATURE_REQUIREMENTS;

/** Unknown versions remain usable; they are not evidence of missing support. */
export type ServerFeatureSupport = 'supported' | 'limited' | 'unknown';

/**
 * Oldest release allowed to connect. This was the full-support baseline before
 * beta.16 added the thread views; its authentication and realtime contracts
 * remain compatible. Do not lower it without verifying the older contracts.
 */
export const MINIMUM_CONNECTABLE_SERVER_VERSION = '0.5.0-beta.9';

/** Full support is derived from the connection floor and feature requirements. */
export const MINIMUM_SUPPORTED_SERVER_VERSION = Object.values(
  SERVER_FEATURE_REQUIREMENTS
).reduce<string>(
  (minimum, required) => (compare(minimum, required) < 0 ? required : minimum),
  MINIMUM_CONNECTABLE_SERVER_VERSION
);

/** Check one feature using the same custom-build rules as connection discovery. */
export function serverFeatureSupport(
  serverVersion: string,
  feature: ServerFeature
): ServerFeatureSupport {
  const comparison = compareReleaseVersions(serverVersion, SERVER_FEATURE_REQUIREMENTS[feature]);
  return comparison === null ? 'unknown' : comparison < 0 ? 'limited' : 'supported';
}

/** List only confirmed version limitations, in the order of the requirements map. */
export function limitedServerFeatures(serverVersion: string): ServerFeature[] {
  return (Object.keys(SERVER_FEATURE_REQUIREMENTS) as ServerFeature[]).filter(
    (feature) => serverFeatureSupport(serverVersion, feature) === 'limited'
  );
}

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
