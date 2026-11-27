import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { defineCollection } from 'astro:content';
import { z } from 'astro/zod';
import type { Loader } from 'astro/loaders';
import { docsLoader } from '@astrojs/starlight/loaders';
import { docsSchema } from '@astrojs/starlight/schema';
import { readReleasePages } from './releases.mjs';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const changelogFiles = ['CHANGELOG.md', '.sweet.yml', '.sweet.config.yml', '.sweet'].map((file) =>
  join(root, file)
);

/** Load a single rendered page per version for all release navigation and routes. */
function releaseLoader(): Loader {
  return {
    name: 'chatto-releases',
    async load({ store, parseData, renderMarkdown, watcher }) {
      const sync = async () => {
        const releases = readReleasePages(root);
        store.clear();
        for (const { sections, ...page } of releases) {
          const rendered = await Promise.all(
            sections.map(async ({ title, notes }) => ({
              title,
              notes: await Promise.all(notes.map(async (note) => (await renderMarkdown(note)).html))
            }))
          );
          const data = await parseData({
            id: page.slug,
            data: { ...page, sections: rendered }
          });
          store.set({ id: page.slug, data });
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
  releases: defineCollection({
    loader: releaseLoader(),
    schema: z.object({
      version: z.string(),
      slug: z.string(),
      title: z.string(),
      link: z.string(),
      description: z.string(),
      released: z.boolean(),
      /** Zero identifies the newest version, including prereleases. */
      order: z.number(),
      sections: z.array(z.object({ title: z.string(), notes: z.array(z.string()) }))
    })
  })
};
