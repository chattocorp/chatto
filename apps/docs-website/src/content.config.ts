import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { defineCollection } from 'astro:content';
import { z } from 'astro/zod';
import type { Loader } from 'astro/loaders';
import { docsLoader } from '@astrojs/starlight/loaders';
import { docsSchema } from '@astrojs/starlight/schema';
import { parseChangelog, readChangelog } from '../../../tools/changelog.mjs';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const changelogFiles = ['CHANGELOG.md', '.sweet.config.yml', '.sweet'].map((file) =>
  join(root, file)
);

/**
 * Loads one entry for each Sweet Release entry in the root `CHANGELOG.md`, and
 * one `Unreleased` entry for the pending notes in `.sweet/`.
 * Each note is rendered to HTML. The entries keep the changelog order, latest first.
 */
function changelogLoader(): Loader {
  return {
    name: 'chatto-changelog',
    async load({ store, parseData, renderMarkdown, watcher }) {
      const sync = async () => {
        const releases = parseChangelog(readChangelog(root));
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
      watcher?.add(changelogFiles);
      const onChange = (path: string) =>
        changelogFiles.some((file) => path.startsWith(file)) && sync();
      for (const event of ['add', 'change', 'unlink']) watcher?.on(event, onChange);
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
