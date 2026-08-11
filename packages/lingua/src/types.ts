export const pluralCategories = [
  "zero",
  "one",
  "two",
  "few",
  "many",
  "other",
] as const;

export type PluralCategory = (typeof pluralCategories)[number];

export type PluralTranslation = Readonly<
  Partial<Record<PluralCategory, string>> & { other: string }
>;

export type TranslationNode = string | PluralTranslation | TranslationObject;

export interface TranslationObject {
  readonly [key: string]: TranslationNode;
}

export type TranslationDocument = Readonly<Record<string, unknown>>;

export type TranslationModule =
  TranslationDocument | Readonly<{ default: TranslationDocument }>;

export type SectionLoader = () => Promise<TranslationModule>;

/** Locale loaders grouped by independently loadable catalog section. */
export type LoaderRegistry = Readonly<
  Record<string, Readonly<Record<string, SectionLoader>>>
>;

export type SectionName<Registry extends LoaderRegistry> = Extract<
  keyof Registry,
  string
>;

export type LocaleName<Registry extends LoaderRegistry> = {
  [Section in keyof Registry]: Extract<keyof Registry[Section], string>;
}[keyof Registry];

export type InterpolationValue = string | number;
export type InterpolationValues = Readonly<Record<string, InterpolationValue>>;

export type CountTranslationKey =
  | `${string}_count`
  | `${string}.count`
  | `${string}_count_html`
  | `${string}.count_html`
  | `${string}.count.html`;

export type HtmlTranslationKey = `${string}_html` | `${string}.html`;

/** Arguments inferred from a literal translation key's naming convention. */
export type TranslationArguments<Key extends string> =
  Key extends CountTranslationKey
    ? [values: InterpolationValues & { count: number }]
    : [values?: InterpolationValues];

declare const localizedHtmlBrand: unique symbol;

/**
 * Untrusted localized markup. It must be sanitized by the rendering application.
 */
export type LocalizedHtml = string & { readonly [localizedHtmlBrand]: true };

export interface MissingTranslation {
  readonly key: string;
  readonly locale: string;
}

export interface LinguaSnapshot<Locale extends string, Section extends string> {
  readonly locale: Locale;
  readonly activeSections: readonly Section[];
  readonly revision: number;
}

/** Configuration for a Lingua runtime backed by lazy JSON section loaders. */
export interface LinguaOptions<Registry extends LoaderRegistry> {
  readonly baseLocale: LocaleName<Registry>;
  readonly initialLocale?: LocaleName<Registry>;
  /**
   * Maps a locale to its next message fallback. Fallbacks are followed until
   * the base locale, allowing regional catalogs to contain only overrides.
   */
  readonly fallbackLocales?: Readonly<
    Partial<Record<LocaleName<Registry>, LocaleName<Registry>>>
  >;
  readonly initialBaseCatalogs?: Readonly<
    Partial<Record<SectionName<Registry>, TranslationModule>>
  >;
  readonly loaders: Registry;
  readonly onMissingTranslation?: (missing: MissingTranslation) => string;
}
