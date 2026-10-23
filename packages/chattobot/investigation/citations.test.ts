import { mkdtemp, mkdir, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from 'vitest';
import { checkCitations, findCitations, linkCitations } from './citations.ts';

test('finds file references once each, but not paths in URLs', () => {
  expect(
    findCitations(
      'See `cli/core/mentions.go:65-100` and cli/core/mentions.go:12, docs/a.md:3. Again `cli/core/mentions.go:65-100`. Not https://github.com/x/y.go:4 or :12.'
    )
  ).toEqual([
    { path: 'cli/core/mentions.go', start: 65, end: 100 },
    { path: 'cli/core/mentions.go', start: 12, end: 12 },
    { path: 'docs/a.md', start: 3, end: 3 }
  ]);
});

test('checks that references name existing lines inside the directory', async () => {
  const root = await mkdtemp(join(tmpdir(), 'citations-'));
  await mkdir(join(root, 'src'));
  await writeFile(join(root, 'src/a.ts'), 'one\ntwo\nthree\n');
  const outside = await mkdtemp(join(tmpdir(), 'outside-'));
  await writeFile(join(outside, 'secret.txt'), 'x\n');
  await symlink(join(outside, 'secret.txt'), join(root, 'link.txt'));
  const checked = await checkCitations(root, [
    { path: 'src/a.ts', start: 2, end: 3 },
    { path: 'src/a.ts', start: 3, end: 4 },
    { path: 'src/missing.ts', start: 1, end: 1 },
    { path: '../outside.txt', start: 1, end: 1 },
    { path: 'link.txt', start: 1, end: 1 }
  ]);
  expect(checked.map((citation) => citation.problem)).toEqual([
    undefined,
    'src/a.ts has 3 lines; lines 3-4 do not exist',
    'src/missing.ts does not exist',
    '../outside.txt is not a relative path',
    'link.txt is outside the directory'
  ]);
});

test('links valid references to their lines at the commit', () => {
  const text = 'In `src/a.ts:2-3` and src/b.ts:7, not `src/c.ts:1` or `run src/a.ts:2-3 here`.';
  expect(
    linkCitations(
      text,
      [
        { path: 'src/a.ts', start: 2, end: 3 },
        { path: 'src/b.ts', start: 7, end: 7 }
      ],
      { repository: 'org/repo', commit: 'abc' }
    )
  ).toBe(
    'In [`src/a.ts:2-3`](https://github.com/org/repo/blob/abc/src/a.ts#L2-L3) and [`src/b.ts:7`](https://github.com/org/repo/blob/abc/src/b.ts#L7), not `src/c.ts:1` or `run src/a.ts:2-3 here`.'
  );
});
