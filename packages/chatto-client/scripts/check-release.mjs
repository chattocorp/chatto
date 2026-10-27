/** Validate release identity and reject versions present in npm history. */
import { readFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';
import semver from 'semver';

/** Check package identity and, when supplied, the release tag.
 * @param {{name: string, version: string}} manifest
 * @param {string} tag
 */
export function assertRelease(manifest, tag = '') {
  if (manifest.name !== '@chatto/client' || semver.valid(manifest.version) !== manifest.version) {
    throw new Error('Expected a valid @chatto/client release version');
  }
  if (tag && tag !== `v${manifest.version}`) {
    throw new Error('Tag must match package version');
  }
}

/** Fail closed on registry errors; a missing package is valid for its first release.
 * @param {string} version
 * @param {typeof fetch} request
 */
export async function assertUnusedVersion(version, request = fetch) {
  const response = await request('https://registry.npmjs.org/@chatto%2Fclient', {
    signal: AbortSignal.timeout(15000)
  });
  if (response.status === 404) return;
  if (!response.ok) throw new Error(`Cannot verify npm version history: HTTP ${response.status}`);
  const metadata = await response.json();
  if (!metadata || typeof metadata !== 'object' || !metadata.versions || !metadata.time) {
    throw new Error('Invalid npm version history');
  }
  if (Object.hasOwn(metadata.versions, version) || Object.hasOwn(metadata.time, version)) {
    throw new Error(`@chatto/client@${version} has already been published. Choose a new version.`);
  }
}

/** Check identity in every mode; only publication requires an unused version.
 * @param {{name: string, version: string}} manifest
 * @param {{tag?: string, verifyOnly?: boolean}} options
 * @param {typeof fetch} request
 */
export async function checkRelease(
  manifest,
  { tag = '', verifyOnly = false } = {},
  request = fetch
) {
  assertRelease(manifest, tag);
  if (!verifyOnly) await assertUnusedVersion(manifest.version, request);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const manifest = JSON.parse(await readFile(new URL('../package.json', import.meta.url), 'utf8'));
  const verifyOnly = process.argv.includes('--verify-only');
  await checkRelease(manifest, { tag: process.env.RELEASE_TAG, verifyOnly });
  console.log(
    verifyOnly
      ? `Verified ${manifest.name}@${manifest.version} release identity.`
      : `${manifest.name}@${manifest.version} is available for publication.`
  );
}
