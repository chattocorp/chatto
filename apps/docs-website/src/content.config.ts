import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { defineCollection } from 'astro:content';
import { z } from 'astro/zod';
import type { Loader } from 'astro/loaders';
import { docsLoader } from '@astrojs/starlight/loaders';
import { docsSchema } from '@astrojs/starlight/schema';
import { parseChangelog } from '../../../tools/changelog.mjs';

const changelogPath = fileURLToPath(new URL('../../../CHANGELOG.md', import.meta.url));

/**
 * Loads one entry for each Sweet Release entry in the root `CHANGELOG.md`.
 * Each note is rendered to HTML. The entries keep the changelog order, latest first.
 */
function changelogLoader(): Loader {
  return {
    name: 'chatto-changelog',
    async load({ store, parseData, renderMarkdown, watcher }) {
      const sync = async () => {
        const releases = parseChangelog(await readFile(changelogPath, 'utf8'));
        store.clear();
        for (const [order, { version, sections }] of releases.entries()) {
          const rendered = await Promise.all(
            sections.map(async ({ title, notes }) => ({
              title,
              notes: await Promise.all(notes.map(async (note) => (await renderMarkdown(note)).html))
            }))
          );
          const data = await parseData({
            id: version,
            data: { version, order, sections: rendered }
          });
          store.set({ id: version, data });
        }
      };
      watcher?.add(changelogPath);
      watcher?.on('change', (path) => path === changelogPath && sync());
      await sync();
    }
  };
}

export const collections = {
  docs: defineCollection({ loader: docsLoader(), schema: docsSchema() }),
  changelog: defineCollection({
    loader: changelogLoader(),
    schema: z.object({
      version: z.string(),
      /** Position in the changelog; 0 is the latest release. */
      order: z.number(),
      sections: z.array(z.object({ title: z.string(), notes: z.array(z.string()) }))
    })
  })
};
