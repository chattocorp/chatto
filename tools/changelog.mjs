/**
 * Read the release notes that Sweet Release writes to `CHANGELOG.md`. The docs
 * website and the bundled frontend use this parser at build time.
 *
 * Sweet writes one `## <version>` heading for each release, one `### <section>`
 * heading for each change type, and one `- ` list item for each note. A note
 * can continue on lines indented by two spaces, including more paragraphs.
 *
 * Entries through 0.5.0 have the release-please format, with a linked version
 * heading such as `## [0.5.0](...)`. The parser stops at the first of these.
 */

/** The section title of the `highlight` type in `.sweet.config.yml`. */
export const HIGHLIGHTS = 'Highlights';

/**
 * @typedef {object} ChangelogSection
 * @property {string} title The section heading, such as `Features`.
 * @property {string[]} notes The Markdown of each note, without the list marker.
 */

/**
 * @typedef {object} ChangelogRelease
 * @property {string} version The version, without the tag prefix, such as `0.6.0`.
 * @property {ChangelogSection[]} sections The sections in changelog order.
 */

/**
 * Parse the Sweet Release entries of a changelog, latest release first.
 *
 * @param {string} markdown The text of `CHANGELOG.md`.
 * @returns {ChangelogRelease[]}
 */
export function parseChangelog(markdown) {
  /** @type {ChangelogRelease[]} */
  const releases = [];
  /** @type {string[] | undefined} The lines of the current note. */
  let note;

  const finishNote = () => {
    const notes = releases.at(-1)?.sections.at(-1)?.notes;
    if (note && notes) notes.push(note.join('\n').trim());
    note = undefined;
  };

  for (const line of markdown.split(/\r?\n/)) {
    if (line.startsWith('## ')) {
      finishNote();
      if (line.startsWith('## [')) break;
      releases.push({ version: line.slice(3).trim(), sections: [] });
    } else if (line.startsWith('### ')) {
      finishNote();
      releases.at(-1)?.sections.push({ title: line.slice(4).trim(), notes: [] });
    } else if (line.startsWith('- ')) {
      finishNote();
      note = [line.slice(2)];
    } else if (note && (line.startsWith('  ') || line.trim() === '')) {
      note.push(line.slice(2));
    } else {
      finishNote();
    }
  }
  finishNote();
  return releases;
}

/**
 * Return the highlights of each release that has highlights, latest first.
 *
 * @param {string} markdown The text of `CHANGELOG.md`.
 * @returns {{ version: string, highlights: string[] }[]}
 */
export function releaseHighlights(markdown) {
  return parseChangelog(markdown).flatMap(({ version, sections }) => {
    const highlights = sections.find(({ title }) => title === HIGHLIGHTS)?.notes ?? [];
    return highlights.length ? [{ version, highlights }] : [];
  });
}

/**
 * Return the stable HTML ID of a release on the docs website changelog page,
 * such as `v0-6-0-beta-4` for `0.6.0-beta.4`.
 *
 * @param {string} version A version from {@link parseChangelog}.
 */
export function releaseAnchor(version) {
  return `v${version.replace(/[^\w-]+/g, '-')}`;
}
