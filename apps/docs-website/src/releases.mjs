import { execFileSync } from 'node:child_process';
import {
  UNRELEASED,
  readChangelog,
  parseChangelog,
  readSweetSections
} from '../../../tools/changelog.mjs';

/** Read the release catalog used by the docs collection and Starlight sidebar.
 * @param {string} root Repository root.
 */
export function readReleasePages(root) {
  const entries = parseChangelog(readChangelog(root));
  let target;
  if (entries.some(({ version }) => version === UNRELEASED)) {
    // Sweet owns the bump rules and the active prerelease configuration.
    const plan = JSON.parse(
      execFileSync('sweet', ['status', '--json'], { cwd: root, encoding: 'utf8' })
    );
    target = plan.find((pkg) => pkg.path === '.')?.nextVersion?.split('-')[0];
  }
  return releasePages(entries, readSweetSections(root).values(), target);
}

/**
 * Group Sweet entries into pages ordered by version, newest first. A final
 * entry replaces its entire prerelease cycle. Pending notes can open a page before its first release.
 * @param {import('../../../tools/changelog.mjs').ChangelogRelease[]} entries Latest first.
 * @param {Iterable<string>} sectionOrder Section titles from the Sweet configuration.
 * @param {string} [target] Numbered version that Sweet plans for pending notes.
 */
export function releasePages(entries, sectionOrder, target) {
  const groups = Map.groupBy(entries, ({ version }) => version.split('-')[0]);
  const pending = groups.get(UNRELEASED) ?? [];
  groups.delete(UNRELEASED);
  if (target && pending.length && !groups.has(target)) groups.set(target, []);
  const versions = [...groups]
    .filter(([version]) => /^\d+\.\d+\.\d+$/.test(version))
    .sort(([a], [b]) => b.localeCompare(a, 'en', { numeric: true }));
  const titles = [...sectionOrder];
  return versions.map(([version, cycle], order) => {
    const stable = cycle.find((entry) => entry.version === version);
    const slug = version.replaceAll('.', '-');
    return {
      version,
      slug,
      order,
      released: Boolean(stable),
      link: `/releases/${slug}/`,
      title: `Chatto ${version}`,
      description: `Highlights, upgrade notes, and changes in Chatto ${version}.`,
      sections:
        stable?.sections ??
        previewSections(
          (target ? version === target : order === 0) ? [...pending, ...cycle] : cycle,
          titles
        )
    };
  });
}

/** Combine prerelease notes by section. Keep historical sections that Sweet no longer configures.
 * @param {import('../../../tools/changelog.mjs').ChangelogRelease[]} entries
 * @param {string[]} sectionOrder
 */
function previewSections(entries, sectionOrder) {
  const groups = Map.groupBy(
    entries.flatMap((entry) => entry.sections),
    (section) => section.title
  );
  return [...new Set([...sectionOrder, ...groups.keys()])]
    .filter((title) => groups.has(title))
    .map((title) => ({ title, notes: groups.get(title).flatMap((section) => section.notes) }));
}
