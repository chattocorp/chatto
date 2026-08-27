import type { SectionLoader, TranslationDocument, TranslationModule } from '@chatto/lingua';

import { catalogSections, publicCatalogSections } from './catalogSections.js';
import { baseLocale, isLocale, type Locale } from './locales';

export { catalogSections, publicCatalogSections };
export type CatalogSection = (typeof catalogSections)[number];

const baseCatalogModules = import.meta.glob<TranslationDocument>('../../../messages/en-GB/*.json', {
  eager: true,
  import: 'default'
});
const translatedCatalogLoaders = import.meta.glob<TranslationDocument>(
  ['../../../messages/*/*.json', '!../../../messages/en-GB/*.json'],
  { import: 'default' }
);

export const initialBaseCatalogs: Partial<Record<CatalogSection, TranslationModule>> = {};
export const catalogLoaders: Record<string, Record<string, SectionLoader>> = {};

for (const [path, catalog] of Object.entries(baseCatalogModules)) {
  const { section } = catalogCoordinates(path);
  initialBaseCatalogs[section] = catalog;
  catalogLoaders[section] = {
    [baseLocale]: async () => catalog
  };
}

for (const [path, loader] of Object.entries(translatedCatalogLoaders)) {
  const { locale, section } = catalogCoordinates(path);
  catalogLoaders[section] ??= {};
  catalogLoaders[section][locale] = loader;
}

for (const section of catalogSections) {
  if (!initialBaseCatalogs[section] || !catalogLoaders[section]?.[baseLocale]) {
    throw new Error(`Missing ${baseLocale} catalog section "${section}"`);
  }
}

function catalogCoordinates(path: string): { locale: Locale; section: CatalogSection } {
  const match = path.match(/\/messages\/([^/]+)\/([^/]+)\.json$/);
  if (!match) throw new Error(`Unexpected translation catalog path: ${path}`);
  const [, locale, section] = match;
  if (!isLocale(locale)) throw new Error(`Unexpected catalog locale "${locale}" in ${path}`);
  if (!catalogSections.includes(section as CatalogSection)) {
    throw new Error(`Unexpected catalog section "${section}" in ${path}`);
  }
  return {
    locale,
    section: section as CatalogSection
  };
}
