import {
  extractSection,
  findTranslation,
  selectPlural,
  validateTranslationOverlay
} from './catalog.js';
import { asLocalizedHtml, isCountTranslationKey, isHtmlTranslationKey } from './conventions.js';
import { InterpolationError, TranslattoError, TranslationKindError } from './errors.js';
import { formatTranslation } from './format.js';
import type {
  HtmlTranslationKey,
  InterpolationValues,
  TranslattoOptions,
  TranslattoSnapshot,
  LoaderRegistry,
  LocaleName,
  LocalizedHtml,
  PluralTranslation,
  SectionName,
  TranslationArguments,
  TranslationObject
} from './types.js';

type Listener<Locale extends string, Section extends string> = (
  snapshot: TranslattoSnapshot<Locale, Section>
) => void;

interface ResolvedTranslation {
  readonly locale: string;
  readonly value: string | PluralTranslation;
}

/** Stateful loader and synchronous translator for sectioned JSON catalogs. */
export class Translatto<Registry extends LoaderRegistry> {
  readonly #baseLocale: LocaleName<Registry>;
  readonly #loaders: Registry;
  readonly #onMissingTranslation: NonNullable<TranslattoOptions<Registry>['onMissingTranslation']>;
  readonly #fallbackLocales: Readonly<Partial<Record<string, string>>>;
  readonly #knownLocales: ReadonlySet<string>;
  readonly #fallbackChains = new Map<string, readonly string[]>();
  readonly #loaded = new Map<string, TranslationObject>();
  /** Catalogs from successful transitions, plus explicitly eager base catalogs. */
  readonly #published = new Map<string, TranslationObject>();
  readonly #loading = new Map<string, Promise<TranslationObject>>();
  readonly #pluralRules = new Map<string, Intl.PluralRules>();
  readonly #numberFormats = new Map<string, Intl.NumberFormat>();
  readonly #listeners = new Set<Listener<LocaleName<Registry>, SectionName<Registry>>>();

  #locale: LocaleName<Registry>;
  #activeSections: readonly SectionName<Registry>[] = Object.freeze([]);
  #revision = 0;
  #transition = 0;
  /** Pending setters compose against this target until it succeeds or fails. */
  #target?: { locale: LocaleName<Registry>; sections: readonly SectionName<Registry>[] };

  constructor(options: TranslattoOptions<Registry>) {
    this.#baseLocale = options.baseLocale;
    this.#locale = options.initialLocale ?? options.baseLocale;
    this.#loaders = options.loaders;
    this.#onMissingTranslation = options.onMissingTranslation ?? (({ key }) => `⟦${key}⟧`);
    this.#fallbackLocales = options.fallbackLocales ?? {};
    this.#knownLocales = new Set(
      Object.values(options.loaders).flatMap((locales) => Object.keys(locales))
    );

    for (const locale of this.#knownLocales) {
      try {
        Intl.getCanonicalLocales(locale);
      } catch {
        throw new TranslattoError(`Invalid locale identifier "${locale}"`);
      }
    }

    if (!this.#knownLocales.has(this.#baseLocale)) {
      throw new TranslattoError(`Unknown base locale "${this.#baseLocale}"`);
    }
    if (!this.#knownLocales.has(this.#locale)) {
      throw new TranslattoError(`Unknown initial locale "${this.#locale}"`);
    }
    for (const [locale, fallback] of Object.entries(this.#fallbackLocales) as [string, string][]) {
      if (!this.#knownLocales.has(locale)) {
        throw new TranslattoError(`Unknown fallback locale "${locale}"`);
      }
      if (!this.#knownLocales.has(fallback)) {
        throw new TranslattoError(`Unknown fallback target "${fallback}"`);
      }
      if (locale === fallback) {
        throw new TranslattoError(`Locale "${locale}" cannot fall back to itself`);
      }
    }
    for (const locale of this.#knownLocales) this.#fallbackChain(locale);
    for (const [section, locales] of Object.entries(options.loaders)) {
      if (!(this.#baseLocale in locales)) {
        throw new TranslattoError(
          `Section "${section}" does not define the base locale "${this.#baseLocale}"`
        );
      }
    }
    for (const [section, catalog] of Object.entries(options.initialBaseCatalogs ?? {})) {
      if (!(section in options.loaders) || !catalog) {
        throw new TranslattoError(`Unknown initial catalog section "${section}"`);
      }
      const key = this.#cacheKey(section, this.#baseLocale);
      const value = extractSection(catalog, section);
      this.#loaded.set(key, value);
      this.#published.set(key, value);
    }
  }

  /** The last completely loaded and committed runtime state. */
  get snapshot(): TranslattoSnapshot<LocaleName<Registry>, SectionName<Registry>> {
    return Object.freeze({
      locale: this.#locale,
      activeSections: this.#activeSections,
      revision: this.#revision
    });
  }

  /** Subscribes to committed state, immediately emitting the current snapshot. */
  subscribe(listener: Listener<LocaleName<Registry>, SectionName<Registry>>): () => void {
    this.#listeners.add(listener);
    listener(this.snapshot);
    return () => this.#listeners.delete(listener);
  }

  /** Loads and validates sections without changing the current runtime state. */
  async preload(
    locale: LocaleName<Registry>,
    sections: readonly SectionName<Registry>[]
  ): Promise<void> {
    this.#assertLocale(locale);
    const uniqueSections = this.#normaliseSections(sections);
    await Promise.all(
      uniqueSections.map((section) =>
        // Start all downloads together. Each overlay waits for the base catalog
        // inside #load before validation, without delaying its network request.
        Promise.all(
          // An async callback also captures loaders that throw before returning.
          this.#fallbackChain(locale).map(async (fallbackLocale) =>
            fallbackLocale === this.#baseLocale
              ? this.#load(section, fallbackLocale)
              : this.#loadOptional(section, fallbackLocale)
          )
        )
      )
    );
  }

  /**
   * Loads and publishes sections together. Previously published catalogs remain
   * readable, but only active sections are loaded by subsequent locale changes.
   */
  async setActiveSections(sections: readonly SectionName<Registry>[]): Promise<void> {
    await this.#transitionTo(this.#target?.locale ?? this.#locale, sections);
  }

  /** Loads every active section before atomically publishing a new locale. */
  async setLocale(locale: LocaleName<Registry>): Promise<void> {
    await this.#transitionTo(locale, this.#target?.sections ?? this.#activeSections);
  }

  /** Resolves a plain-text translation synchronously from published catalogs. */
  t<const Key extends string>(
    key: Key extends HtmlTranslationKey ? never : Key,
    ...args: TranslationArguments<Key>
  ): string {
    if (isHtmlTranslationKey(key)) {
      throw new TranslationKindError(`HTML translation "${key}" must use html()`);
    }
    return this.#translate(key, args[0], false);
  }

  /** Resolves untrusted localized markup whose interpolated values are HTML-escaped. */
  html<const Key extends HtmlTranslationKey>(
    key: Key,
    ...args: TranslationArguments<Key>
  ): LocalizedHtml {
    if (!isHtmlTranslationKey(key)) {
      throw new TranslationKindError(`Translation "${key}" is not an HTML translation`);
    }
    return asLocalizedHtml(this.#translate(key, args[0], true));
  }

  async #transitionTo(
    locale: LocaleName<Registry>,
    sections: readonly SectionName<Registry>[]
  ): Promise<void> {
    this.#assertLocale(locale);
    const nextSections = this.#normaliseSections(sections);
    const transition = ++this.#transition;
    this.#target = { locale, sections: nextSections };
    try {
      await this.preload(locale, nextSections);
    } catch (error) {
      if (transition === this.#transition) this.#target = undefined;
      throw error;
    }
    if (transition !== this.#transition) return;

    for (const section of nextSections) {
      for (const fallback of this.#fallbackChain(locale)) {
        const key = this.#cacheKey(section, fallback);
        const catalog = this.#loaded.get(key);
        if (catalog) this.#published.set(key, catalog);
      }
    }
    this.#target = undefined;
    this.#locale = locale;
    this.#activeSections = nextSections;
    this.#revision += 1;
    const snapshot = this.snapshot;
    for (const listener of this.#listeners) listener(snapshot);
  }

  #translate(key: string, values: InterpolationValues | undefined, html: boolean): string {
    const [section, ...path] = key.split('.');
    if (!section || path.length === 0) {
      throw new TranslattoError(`Translation key "${key}" must include a section and message path`);
    }
    const resolved = this.#resolve(section, path);
    if (!resolved) {
      return this.#onMissingTranslation({ key, locale: this.#locale });
    }

    let template: string;
    let interpolationValues = values;
    if (isCountTranslationKey(key)) {
      if (typeof resolved.value === 'string') {
        throw new TranslationKindError(`Plural translation "${key}" is not a plural object`);
      }
      const count = values?.count;
      if (typeof count !== 'number' || !Number.isFinite(count)) {
        throw new InterpolationError(`Plural translation "${key}" requires a finite count`);
      }
      const category = this.#getPluralRules(resolved.locale).select(count);
      template = selectPlural(resolved.value, category);
      interpolationValues = {
        ...values,
        count: this.#getNumberFormat(resolved.locale).format(count)
      };
    } else {
      if (typeof resolved.value !== 'string') {
        throw new TranslationKindError(`Translation "${key}" is unexpectedly plural`);
      }
      template = resolved.value;
    }
    return formatTranslation(template, interpolationValues, html);
  }

  #resolve(section: string, path: readonly string[]): ResolvedTranslation | undefined {
    for (const locale of this.#fallbackChain(this.#locale)) {
      const catalog = this.#published.get(this.#cacheKey(section, locale));
      const value = findTranslation(catalog, path);
      if (value !== undefined) return { locale, value };
    }
    return undefined;
  }

  #getPluralRules(locale: string): Intl.PluralRules {
    let rules = this.#pluralRules.get(locale);
    if (!rules) {
      rules = new Intl.PluralRules(locale);
      this.#pluralRules.set(locale, rules);
    }
    return rules;
  }

  #getNumberFormat(locale: string): Intl.NumberFormat {
    let formatter = this.#numberFormats.get(locale);
    if (!formatter) {
      formatter = new Intl.NumberFormat(locale);
      this.#numberFormats.set(locale, formatter);
    }
    return formatter;
  }

  #normaliseSections(sections: readonly SectionName<Registry>[]): readonly SectionName<Registry>[] {
    const unique = [...new Set(sections)];
    for (const section of unique) {
      if (!(section in this.#loaders)) {
        throw new TranslattoError(`Unknown translation section "${section}"`);
      }
    }
    return Object.freeze(unique);
  }

  #assertLocale(locale: string): void {
    if (!this.#knownLocales.has(locale)) {
      throw new TranslattoError(`Unknown locale "${locale}"`);
    }
  }

  #fallbackChain(locale: string): readonly string[] {
    const cached = this.#fallbackChains.get(locale);
    if (cached) return cached;

    const chain: string[] = [locale];
    const seen = new Set(chain);
    let current = locale;
    while (current !== this.#baseLocale) {
      const fallback = this.#fallbackLocales[current] ?? this.#baseLocale;
      if (seen.has(fallback)) {
        throw new TranslattoError(`Locale fallback cycle includes "${fallback}"`);
      }
      chain.push(fallback);
      seen.add(fallback);
      current = fallback;
    }
    const resolved = Object.freeze(chain);
    this.#fallbackChains.set(locale, resolved);
    return resolved;
  }

  async #loadOptional(section: string, locale: string): Promise<TranslationObject | undefined> {
    if (!this.#loaders[section]?.[locale]) return undefined;
    return this.#load(section, locale);
  }

  #load(section: string, locale: string): Promise<TranslationObject> {
    const cacheKey = this.#cacheKey(section, locale);
    const loaded = this.#loaded.get(cacheKey);
    if (loaded) return Promise.resolve(loaded);

    const loading = this.#loading.get(cacheKey);
    if (loading) return loading;

    const loader = this.#loaders[section]?.[locale];
    if (!loader) {
      return Promise.reject(
        new TranslattoError(`No loader for section "${section}" and locale "${locale}"`)
      );
    }

    const promise = loader()
      .then(async (module) => {
        const catalog = extractSection(module, section);
        if (locale !== this.#baseLocale) {
          const base = await this.#load(section, this.#baseLocale);
          validateTranslationOverlay(base, catalog, section);
        }
        return catalog;
      })
      .then((catalog) => {
        this.#loaded.set(cacheKey, catalog);
        this.#loading.delete(cacheKey);
        return catalog;
      })
      .catch((error: unknown) => {
        this.#loading.delete(cacheKey);
        throw error;
      });
    this.#loading.set(cacheKey, promise);
    return promise;
  }

  #cacheKey(section: string, locale: string): string {
    return `${section}\u0000${locale}`;
  }
}

/** Creates a framework-neutral JSON translation runtime. */
export function createTranslatto<const Registry extends LoaderRegistry>(
  options: TranslattoOptions<Registry>
): Translatto<Registry> {
  return new Translatto(options);
}
