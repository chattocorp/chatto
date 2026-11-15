import type { Component } from 'svelte';
import { baseLocale, fallbackLocales, type Locale } from '$lib/i18n/locales';
import type { HighlightSurface } from './catalog';

const documents = import.meta.glob<{ default: Component }>('./content/*/*/*.mdx');

/** Compiled application content only. Never import or compile code supplied by a server. */
export async function loadHighlight(locale: Locale, surface: HighlightSurface, id: string) {
  const parent =
    locale in fallbackLocales
      ? fallbackLocales[locale as keyof typeof fallbackLocales]
      : baseLocale;
  const candidates: Locale[] = [locale, parent, baseLocale];
  for (const language of candidates) {
    const load = documents[`./content/${language}/${surface}/${id}.mdx`];
    if (load) return { ...(await load()), locale: language };
  }
  throw new Error(`Missing release highlight: ${surface}/${id}`);
}
