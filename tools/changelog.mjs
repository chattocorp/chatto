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
 *
 * {@link readChangelog} adds the pending notes in `.sweet/` as an `Unreleased`
 * release, so development builds show them. A release consumes these notes,
 * so release builds have no `Unreleased` entry.
 */

import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

/** The version heading of the pending notes in `.sweet/`. */
export const UNRELEASED = 'Unreleased';

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
 * Return the releases for the in-app guide, latest first. Once the stable
 * release contains the highlights from its prereleases, omit those entries.
 * Include pending notes only when there are pending highlights.
 *
 * @param {string} markdown The text of `CHANGELOG.md`.
 * @returns {{ version: string, highlights: string[] }[]}
 */
export function releaseHighlights(markdown) {
  const releases = parseChangelog(markdown);
  const latest = releases.find(({ version }) => version !== UNRELEASED);
  const currentCyclePrefix = latest?.version.match(/^\d+\.\d+\.\d+-/)?.[0];
  return releases.flatMap(({ version, sections }) => {
    if (version.includes('-') && !(currentCyclePrefix && version.startsWith(currentCyclePrefix)))
      return [];
    const highlights = sections.find(({ title }) => title === HIGHLIGHTS)?.notes ?? [];
    return version !== UNRELEASED || highlights.length ? [{ version, highlights }] : [];
  });
}

/**
 * Read `CHANGELOG.md` from the repository root, with the pending notes in
 * `.sweet/` added as the first release, named {@link UNRELEASED}.
 *
 * @param {string} root The repository root directory.
 * @returns {string} The changelog Markdown.
 */
export function readChangelog(root) {
  const changelog = readFileSync(join(root, 'CHANGELOG.md'), 'utf8');
  const unreleased = unreleasedChangelog(root);
  const at = changelog.search(/^## /m);
  return at < 0
    ? changelog + unreleased
    : changelog.slice(0, at) + unreleased + changelog.slice(at);
}

/**
 * Render the pending notes in `.sweet/` as one changelog entry, in the section
 * order of `.sweet.config.yml`. Return an empty string when there are none.
 * A release consumes all notes, and Git then drops the empty directory, so a
 * missing `.sweet/` means that there are no pending notes.
 *
 * @param {string} root The repository root directory.
 */
function unreleasedChangelog(root) {
  const directory = join(root, '.sweet');
  if (!existsSync(directory)) return '';
  const sections = readSweetSections(root);
  /** @type {Map<string, string[]>} */
  const notes = new Map([...new Set(sections.values())].map((title) => [title, []]));
  const files = readdirSync(directory)
    .filter((file) => file.endsWith('.md'))
    .sort();
  for (const file of files) {
    const match = /^---\r?\n([\s\S]*?)\r?\n---\r?\n([\s\S]*)$/.exec(
      readFileSync(join(directory, file), 'utf8')
    );
    const type = match && /^type:\s*(\S+)\s*$/m.exec(match[1])?.[1];
    const title = type && sections.get(type);
    const body = match?.[2].trim();
    if (title && body) notes.get(title)?.push(`- ${body.replace(/\n(?=.)/g, '\n  ')}`);
  }
  const groups = [...notes].filter(([, list]) => list.length);
  if (!groups.length) return '';
  return [
    `## ${UNRELEASED}`,
    ...groups.map(([title, list]) => `### ${title}\n\n${list.join('\n')}`)
  ]
    .join('\n\n')
    .concat('\n\n');
}

/**
 * Read the changelog section of each type from `.sweet.config.yml`, in file
 * order. Types without a section are left out. This reads only the simple
 * `types:` mapping that the file uses, so that no YAML parser is needed.
 *
 * @param {string} root The repository root directory.
 * @returns {Map<string, string>} The section title of each type.
 */
export function readSweetSections(root) {
  const config = readFileSync(join(root, '.sweet.config.yml'), 'utf8');
  /** @type {Map<string, string>} */
  const sections = new Map();
  let type;
  for (const line of config.split(/\r?\n/)) {
    const key = /^ {2}([\w-]+):\s*$/.exec(line);
    if (key) type = key[1];
    const section = /^ {4}section:\s*(.+?)\s*$/.exec(line);
    if (type && section) sections.set(type, section[1]);
  }
  return sections;
}
