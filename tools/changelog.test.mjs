/** Check the Sweet Release changelog parser against the formats that Sweet writes. */
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { parseChangelog, readChangelog, releaseHighlights } from './changelog.mjs';

const changelog = `# Changelog

Intro text.

## 0.6.0

### Highlights

- Sign in with your **internet handle**.

### Features

- First feature. ([#1](https://example.com/1))
- Second feature, with a second paragraph.

  More about it, with \`code\`. ([#2](https://example.com/2))

### Fixes

- A fix.

## 0.6.0-beta.1

### Fixes

- Another fix.

## [0.5.0](https://github.com/chattocorp/chatto/compare/v0.5.0-rc.5...v0.5.0) (2026-11-09)

### Features

- Old release-please entry.
`;

test('parses releases, sections, and multi-paragraph notes', () => {
  assert.deepEqual(parseChangelog(changelog), [
    {
      version: '0.6.0',
      sections: [
        { title: 'Highlights', notes: ['Sign in with your **internet handle**.'] },
        {
          title: 'Features',
          notes: [
            'First feature. ([#1](https://example.com/1))',
            'Second feature, with a second paragraph.\n\nMore about it, with `code`. ([#2](https://example.com/2))'
          ]
        },
        { title: 'Fixes', notes: ['A fix.'] }
      ]
    },
    { version: '0.6.0-beta.1', sections: [{ title: 'Fixes', notes: ['Another fix.'] }] }
  ]);
});

test('returns only releases with highlights', () => {
  assert.deepEqual(releaseHighlights(changelog), [
    { version: '0.6.0', highlights: ['Sign in with your **internet handle**.'] }
  ]);
});

test('parses the repository changelog', () => {
  const releases = parseChangelog(
    readFileSync(new URL('../CHANGELOG.md', import.meta.url), 'utf8')
  );
  assert.ok(releases.length > 0);
  for (const { version, sections } of releases) {
    assert.match(version, /^\d+\.\d+\.\d+(-[\w.]+)?$/);
    for (const { notes } of sections) for (const note of notes) assert.notEqual(note, '');
  }
});

test('adds pending notes as the first release, in section order', (t) => {
  const root = mkdtempSync(join(tmpdir(), 'chatto-changelog-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  writeFileSync(join(root, 'CHANGELOG.md'), changelog);
  writeFileSync(
    join(root, '.sweet.config.yml'),
    'types:\n  highlight:\n    bump: none\n    section: Highlights\n  fix:\n    bump: patch\n    section: Fixes\n  chore:\n    bump: none\n'
  );
  mkdirSync(join(root, '.sweet'));
  const note = (name, type, body) =>
    writeFileSync(join(root, '.sweet', name), `---\ntype: ${type}\n---\n\n${body}\n`);
  note('a.md', 'fix', 'A pending fix.\n\nWith a second paragraph.');
  note('b.md', 'highlight', 'A pending highlight.');
  note('c.md', 'chore', 'Not in the changelog.');

  const [unreleased, latest] = parseChangelog(readChangelog(root));
  assert.deepEqual(unreleased, {
    version: 'Unreleased',
    sections: [
      { title: 'Highlights', notes: ['A pending highlight.'] },
      { title: 'Fixes', notes: ['A pending fix.\n\nWith a second paragraph.'] }
    ]
  });
  assert.equal(latest.version, '0.6.0');

  rmSync(join(root, '.sweet'), { recursive: true });
  assert.equal(parseChangelog(readChangelog(root))[0].version, '0.6.0');
});
