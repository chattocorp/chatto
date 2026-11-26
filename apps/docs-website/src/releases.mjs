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
  return releasePages(parseChangelog(readChangelog(root)), readSweetSections(root).values());
}

/**
 * Group Sweet entries into pages ordered by version, newest first. A final
 * entry replaces its entire prerelease cycle. Only the newest cycle can
 * include pending notes, and only while that cycle is in prerelease.
 * @param {import('../../../tools/changelog.mjs').ChangelogRelease[]} entries Latest first.
 * @param {Iterable<string>} sectionOrder Section titles from the Sweet configuration.
 */
export function releasePages(entries, sectionOrder) {
  const groups = Map.groupBy(entries, ({ version }) => version.split('-')[0]);
  const pending = groups.get(UNRELEASED) ?? [];
  groups.delete(UNRELEASED);
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
        stable?.sections ?? previewSections(order === 0 ? [...pending, ...cycle] : cycle, titles)
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
