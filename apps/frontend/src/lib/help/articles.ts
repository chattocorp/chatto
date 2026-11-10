import type { Component } from 'svelte';
import { baseLocale, fallbackLocales, type Locale } from '$lib/i18n/locales';
import type { HelpTopic } from './navigation';

/** An authored section and its build-generated anchor. Example headings are excluded. */
export type HelpHeading = { id: string; title: string; depth: 2 | 3 };

/** Build-time compiled documents. Each import stays behind its article and locale boundary. */
const articles = import.meta.glob<{
  default: Component;
  metadata: { toc: HelpHeading[] };
}>('./content/*/*.mdx');

/** Load one document, with the same regional inheritance as the UI catalogs. */
export async function loadHelpArticle(locale: Locale, topic: HelpTopic) {
  const fallback =
    locale in fallbackLocales
      ? fallbackLocales[locale as keyof typeof fallbackLocales]
      : baseLocale;
  const candidates: Locale[] = [locale, fallback, baseLocale];
  for (const language of candidates) {
    const load = articles[`./content/${language}/${topic}.mdx`];
    if (load) return { ...(await load()), locale: language };
  }
  throw new Error(`Missing help article: ${locale}/${topic}`);
}
