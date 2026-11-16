// Selects the CI job groups for one workflow run.
//
// The `changes` job in .github/workflows/ci.yml matches the changed paths
// against its path filters, then runs this script to combine the results into
// the job outputs. Keep the combination rules here, so
// tools/ci-selection.test.mjs can test them together with the path filters.

import { appendFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

/** Head branch prefix of the release pull requests that Sweet Release maintains. */
export const RELEASE_BRANCH_PREFIX = 'sweet/release-';

/** Products that one path filter selects, in addition to the `common` filter. */
const PRODUCTS = ['authling', 'translatto', 'runling', 'chattobot', 'botto', 'docs'];

/**
 * Returns true for a release pull request that changes only release files.
 *
 * `main` has already tested the content of such a pull request, and the push
 * to `main` after the merge runs all checks again before a tag is made. The
 * pull request thus runs only the checks that do not depend on job selection.
 * A branch in a fork cannot use the release branch name to skip checks.
 *
 * @param {Record<string, boolean>} changed Path filter results by filter name.
 * @param {{ event: string, headRef?: string, sameRepository?: boolean }} context
 */
export function isReleasePullRequest(changed, { event, headRef = '', sameRepository = false }) {
  return (
    event === 'pull_request' &&
    sameRepository &&
    headRef.startsWith(RELEASE_BRANCH_PREFIX) &&
    !changed['non-release']
  );
}

/**
 * Desktop build platforms. `os` is the runner label and `artifactPath` is the
 * bundle that the build must produce. Platforms with `fullOnly` duplicate an
 * operating system that another platform covers.
 */
const DESKTOP_PLATFORMS = [
  { platform: 'linux', os: 'ubuntu-latest', artifactPath: 'apps/desktop/dist/chatto-desktop' },
  {
    platform: 'linux-arm64',
    os: 'ubuntu-24.04-arm',
    artifactPath: 'apps/desktop/dist/chatto-desktop'
  },
  { platform: 'macos', os: 'macos-15', artifactPath: 'apps/desktop/dist/Chatto.app' },
  {
    platform: 'macos-intel',
    os: 'macos-15-intel',
    artifactPath: 'apps/desktop/dist/Chatto.app',
    fullOnly: true
  },
  {
    platform: 'macos-universal',
    os: 'macos-15',
    artifactPath: 'apps/desktop/dist/Chatto.app',
    fullOnly: true
  },
  { platform: 'windows', os: 'windows-latest', artifactPath: 'apps/desktop/dist/windows' }
];

/**
 * Returns the `include` list for the matrix of the `desktop` job.
 *
 * @param {boolean} full True to build all platforms.
 * @returns {{ platform: string, os: string, 'artifact-path': string }[]}
 */
export function desktopMatrix(full) {
  return DESKTOP_PLATFORMS.filter(({ fullOnly }) => full || !fullOnly).map(
    ({ platform, os, artifactPath }) => ({ platform, os, 'artifact-path': artifactPath })
  );
}

/**
 * Combines path filter results into the boolean outputs of the `changes` job.
 *
 * - `go` and `web` split the Chatto checks by toolchain. `chatto` is true when
 *   one of them is true, and selects the checks that need the complete product.
 * - `desktop-full` adds the macOS Intel and universal builds and the native
 *   capture helper test to the desktop jobs. Pushes always use the full set.
 *
 * @param {Record<string, boolean>} changed Path filter results by filter name.
 * @param {{ event: string, headRef?: string, sameRepository?: boolean }} context
 * @returns {Record<string, boolean>} Job outputs by name.
 */
export function selectJobs(changed, context) {
  const release = isReleasePullRequest(changed, context);
  const all = !release && Boolean(changed.common);
  const on = (name) => !release && (all || Boolean(changed[name]));

  const go = on('go');
  const web = on('web');
  const desktopDirect = on('desktop');

  return {
    go,
    web,
    chatto: go || web,
    desktop: desktopDirect || on('desktop-shared'),
    'desktop-full': desktopDirect || (!release && context.event === 'push'),
    knip: on('knip'),
    ...Object.fromEntries(PRODUCTS.map((name) => [name, on(name)]))
  };
}

/**
 * Returns all outputs of the `changes` job as strings.
 *
 * @param {Record<string, boolean>} changed Path filter results by filter name.
 * @param {{ event: string, headRef?: string, sameRepository?: boolean }} context
 * @returns {Record<string, string>} Job outputs by name.
 */
export function jobOutputs(changed, context) {
  const jobs = selectJobs(changed, context);
  return {
    ...Object.fromEntries(Object.entries(jobs).map(([name, value]) => [name, String(value)])),
    'desktop-matrix': JSON.stringify(desktopMatrix(jobs['desktop-full']))
  };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  // paths-filter reports each filter as the string 'true' or 'false'.
  const results = JSON.parse(process.env.FILTER_RESULTS ?? '{}');
  const changed = Object.fromEntries(
    Object.entries(results).map(([name, value]) => [name, value === 'true'])
  );
  const outputs = jobOutputs(changed, {
    event: process.env.GITHUB_EVENT_NAME ?? '',
    headRef: process.env.GITHUB_HEAD_REF ?? '',
    sameRepository: process.env.HEAD_REPOSITORY === process.env.GITHUB_REPOSITORY
  });
  const lines = Object.entries(outputs).map(([name, value]) => `${name}=${value}`);
  console.log(lines.join('\n'));
  if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, `${lines.join('\n')}\n`);
}
