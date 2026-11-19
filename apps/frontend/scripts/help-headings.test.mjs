import assert from 'node:assert/strict';
import test from 'node:test';
import { compile } from 'mdsvex';
import { helpHeadings } from './help-headings.js';

test('compiled contents match anchors, preserve inline text, and disambiguate headings', async () => {
  const result = await compile(
    '## **Start** here\n\n### Start here\n\n## Start here-2\n\n## 日本語\n\n> ## Quoted heading\n\n```md\n## Code heading\n```',
    {
      remarkPlugins: [helpHeadings]
    }
  );
  assert.ok(result);
  const toc = result.data.fm.toc;
  assert.deepEqual(toc, [
    { id: 'section-start-here', title: 'Start here', depth: 2 },
    { id: 'section-start-here-2', title: 'Start here', depth: 3 },
    { id: 'section-start-here-2-2', title: 'Start here-2', depth: 2 },
    { id: 'section-日本語', title: '日本語', depth: 2 }
  ]);
  for (const heading of toc) assert.ok(result.code.includes(`id="${heading.id}"`));
  assert.ok(result.code.includes('export const metadata'));
});
