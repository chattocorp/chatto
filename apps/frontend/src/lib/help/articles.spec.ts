import { existsSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { baseLocale, fallbackLocales, selectableLocales } from '$lib/i18n/locales';
import { renderMarkdown } from '$lib/markdown';

const topics = ['conversations', 'formatting', 'bots', 'phone'];
const content = new URL('./content/', import.meta.url);

/** Missing regional documents may inherit their language, but never an unrelated language. */
describe('translated help coverage', () => {
  for (const locale of selectableLocales) {
    for (const topic of topics) {
      it(`${locale}/${topic} has a complete document in its language`, async () => {
        const own = new URL(`${locale}/${topic}.mdx`, content);
        const parent = fallbackLocales[locale as keyof typeof fallbackLocales];
        const path = existsSync(own) ? own : new URL(`${parent}/${topic}.mdx`, content);
        expect(existsSync(path), `Missing translation: ${locale}/${topic}`).toBe(true);
        const source = readFileSync(new URL(`${baseLocale}/${topic}.mdx`, content), 'utf8');
        const translated = readFileSync(path, 'utf8');
        // Translations must retain all sections and working examples, not become summaries.
        expect(translated.match(/^#{2,3}(?= )/gm)).toEqual(source.match(/^#{2,3}(?= )/gm));
        for (const component of ['MarkdownSample', 'FormattingExample']) {
          const pattern = new RegExp(`<${component}\\b`, 'g');
          expect(translated.match(pattern)?.length ?? 0).toBe(source.match(pattern)?.length ?? 0);
        }
        // URLs and permission identifiers are protocol details, not translatable prose.
        for (const pattern of [/\]\((https?:\/\/[^)]+)\)/g, /\b(?:message|bot|room)\.[a-z.-]+/g]) {
          const values = (text: string) => [...new Set(text.match(pattern))].sort();
          expect(values(translated)).toEqual(values(source));
        }
        if (!locale.startsWith('en-')) expect(translated).not.toBe(source);
        if (topic === 'formatting') {
          // Translated words must still demonstrate the promised Markdown syntax.
          // In particular, Chatto rejects emphasis in the middle of a word.
          const example = translated.match(/<MarkdownSample source=\{("(?:\\.|[^"\\])*")\}/);
          expect(example, 'Missing worked formatting example').not.toBeNull();
          const html = await renderMarkdown(JSON.parse(example![1]));
          for (const tag of ['strong', 'em', 'u', 's']) expect(html).toContain(`<${tag}>`);
        }
      });
    }
  }
});
