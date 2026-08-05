import {
  extractSection,
  findTranslation,
  selectPlural,
  validateTranslationOverlay,
} from "./catalog.js";
import {
  asLocalizedHtml,
  isCountTranslationKey,
  isHtmlTranslationKey,
} from "./conventions.js";
import {
  InterpolationError,
  LinguaError,
  TranslationKindError,
} from "./errors.js";
import { formatTranslation } from "./format.js";
import type {
  HtmlTranslationKey,
  InterpolationValues,
  LinguaOptions,
  LinguaSnapshot,
  LoaderRegistry,
  LocaleName,
  LocalizedHtml,
  PluralTranslation,
  SectionName,
  TranslationArguments,
  TranslationObject,
} from "./types.js";

type Listener<Locale extends string, Section extends string> = (
  snapshot: LinguaSnapshot<Locale, Section>,
) => void;

interface ResolvedTranslation {
  readonly locale: string;
  readonly value: string | PluralTranslation;
}

/** Stateful loader and synchronous translator for sectioned JSON catalogs. */
export class Lingua<Registry extends LoaderRegistry> {
  readonly #baseLocale: LocaleName<Registry>;
  readonly #loaders: Registry;
  readonly #onMissingTranslation: NonNullable<
    LinguaOptions<Registry>["onMissingTranslation"]
  >;
  readonly #knownLocales: ReadonlySet<string>;
  readonly #loaded = new Map<string, TranslationObject>();
  readonly #loading = new Map<string, Promise<TranslationObject>>();
  readonly #validatedOverlays = new Set<string>();
  readonly #pluralRules = new Map<string, Intl.PluralRules>();
  readonly #numberFormats = new Map<string, Intl.NumberFormat>();
  readonly #listeners = new Set<
    Listener<LocaleName<Registry>, SectionName<Registry>>
  >();

  #locale: LocaleName<Registry>;
  #activeSections: readonly SectionName<Registry>[] = [];
  #revision = 0;
  #transition = 0;

  constructor(options: LinguaOptions<Registry>) {
    this.#baseLocale = options.baseLocale;
    this.#locale = options.initialLocale ?? options.baseLocale;
    this.#loaders = options.loaders;
    this.#onMissingTranslation =
      options.onMissingTranslation ?? (({ key }) => `⟦${key}⟧`);
    this.#knownLocales = new Set(
      Object.values(options.loaders).flatMap((locales) => Object.keys(locales)),
    );

    for (const locale of this.#knownLocales) {
      try {
        Intl.getCanonicalLocales(locale);
      } catch {
        throw new LinguaError(`Invalid locale identifier "${locale}"`);
      }
    }

    if (!this.#knownLocales.has(this.#baseLocale)) {
      throw new LinguaError(`Unknown base locale "${this.#baseLocale}"`);
    }
    if (!this.#knownLocales.has(this.#locale)) {
      throw new LinguaError(`Unknown initial locale "${this.#locale}"`);
    }
    for (const [section, locales] of Object.entries(options.loaders)) {
      if (!(this.#baseLocale in locales)) {
        throw new LinguaError(
          `Section "${section}" does not define the base locale "${this.#baseLocale}"`,
        );
      }
    }
    for (const [section, catalog] of Object.entries(
      options.initialBaseCatalogs ?? {},
    )) {
      if (!(section in options.loaders) || !catalog) {
        throw new LinguaError(`Unknown initial catalog section "${section}"`);
      }
      this.#loaded.set(
        this.#cacheKey(section, this.#baseLocale),
        extractSection(catalog, section),
      );
    }
  }

  /** The last completely loaded and committed runtime state. */
  get snapshot(): LinguaSnapshot<LocaleName<Registry>, SectionName<Registry>> {
    return Object.freeze({
      locale: this.#locale,
      activeSections: this.#activeSections,
      revision: this.#revision,
    });
  }

  /** Subscribes to committed state, immediately emitting the current snapshot. */
  subscribe(
    listener: Listener<LocaleName<Registry>, SectionName<Registry>>,
  ): () => void {
    this.#listeners.add(listener);
    listener(this.snapshot);
    return () => this.#listeners.delete(listener);
  }

  /** Loads and validates sections without changing the current runtime state. */
  async preload(
    locale: LocaleName<Registry>,
    sections: readonly SectionName<Registry>[],
  ): Promise<void> {
    this.#assertLocale(locale);
    const uniqueSections = this.#normaliseSections(sections);
    await Promise.all(
      uniqueSections.map(async (section) => {
        const base = await this.#load(section, this.#baseLocale);
        if (locale === this.#baseLocale) return;

        const translated = await this.#loadOptional(section, locale);
        if (!translated) return;

        const overlayKey = this.#cacheKey(section, locale);
        if (!this.#validatedOverlays.has(overlayKey)) {
          validateTranslationOverlay(base, translated, section);
          this.#validatedOverlays.add(overlayKey);
        }
      }),
    );
  }

  /** Loads and atomically replaces the sections available to synchronous lookups. */
  async setActiveSections(
    sections: readonly SectionName<Registry>[],
  ): Promise<void> {
    await this.#transitionTo(this.#locale, sections);
  }

  /** Loads every active section before atomically publishing a new locale. */
  async setLocale(locale: LocaleName<Registry>): Promise<void> {
    await this.#transitionTo(locale, this.#activeSections);
  }

  /** Resolves a plain-text translation synchronously from the active sections. */
  t<const Key extends string>(
    key: Key extends HtmlTranslationKey ? never : Key,
    ...args: TranslationArguments<Key>
  ): string {
    if (isHtmlTranslationKey(key)) {
      throw new TranslationKindError(
        `HTML translation "${key}" must use html()`,
      );
    }
    return this.#translate(key, args[0], false);
  }

  /** Resolves untrusted localized markup whose interpolated values are HTML-escaped. */
  html<const Key extends HtmlTranslationKey>(
    key: Key,
    ...args: TranslationArguments<Key>
  ): LocalizedHtml {
    if (!isHtmlTranslationKey(key)) {
      throw new TranslationKindError(
        `Translation "${key}" is not an HTML translation`,
      );
    }
    return asLocalizedHtml(this.#translate(key, args[0], true));
  }

  async #transitionTo(
    locale: LocaleName<Registry>,
    sections: readonly SectionName<Registry>[],
  ): Promise<void> {
    this.#assertLocale(locale);
    const transition = ++this.#transition;
    const nextSections = this.#normaliseSections(sections);
    await this.preload(locale, nextSections);
    if (transition !== this.#transition) return;

    this.#locale = locale;
    this.#activeSections = nextSections;
    this.#revision += 1;
    const snapshot = this.snapshot;
    for (const listener of this.#listeners) listener(snapshot);
  }

  #translate(
    key: string,
    values: InterpolationValues | undefined,
    html: boolean,
  ): string {
    const [section, ...path] = key.split(".");
    if (!section || path.length === 0) {
      throw new LinguaError(
        `Translation key "${key}" must include a section and message path`,
      );
    }
    const resolved = this.#resolve(section, path);
    if (!resolved) {
      return this.#onMissingTranslation({ key, locale: this.#locale });
    }

    let template: string;
    let interpolationValues = values;
    if (isCountTranslationKey(key)) {
      if (typeof resolved.value === "string") {
        throw new TranslationKindError(
          `Plural translation "${key}" is not a plural object`,
        );
      }
      const count = values?.count;
      if (typeof count !== "number" || !Number.isFinite(count)) {
        throw new InterpolationError(
          `Plural translation "${key}" requires a finite count`,
        );
      }
      const category = this.#getPluralRules(resolved.locale).select(count);
      template = selectPlural(resolved.value, category);
      interpolationValues = {
        ...values,
        count: this.#getNumberFormat(resolved.locale).format(count),
      };
    } else {
      if (typeof resolved.value !== "string") {
        throw new TranslationKindError(
          `Translation "${key}" is unexpectedly plural`,
        );
      }
      template = resolved.value;
    }
    return formatTranslation(template, interpolationValues, html);
  }

  #resolve(
    section: string,
    path: readonly string[],
  ): ResolvedTranslation | undefined {
    const selected = this.#loaded.get(this.#cacheKey(section, this.#locale));
    const selectedValue = findTranslation(selected, path);
    if (selectedValue !== undefined) {
      return { locale: this.#locale, value: selectedValue };
    }

    const base = this.#loaded.get(this.#cacheKey(section, this.#baseLocale));
    const baseValue = findTranslation(base, path);
    return baseValue === undefined
      ? undefined
      : { locale: this.#baseLocale, value: baseValue };
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

  #normaliseSections(
    sections: readonly SectionName<Registry>[],
  ): readonly SectionName<Registry>[] {
    const unique = [...new Set(sections)];
    for (const section of unique) {
      if (!(section in this.#loaders)) {
        throw new LinguaError(`Unknown translation section "${section}"`);
      }
    }
    return Object.freeze(unique);
  }

  #assertLocale(locale: string): void {
    if (!this.#knownLocales.has(locale)) {
      throw new LinguaError(`Unknown locale "${locale}"`);
    }
  }

  async #loadOptional(
    section: string,
    locale: string,
  ): Promise<TranslationObject | undefined> {
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
        new LinguaError(
          `No loader for section "${section}" and locale "${locale}"`,
        ),
      );
    }

    const promise = loader()
      .then((module) => extractSection(module, section))
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
export function createLingua<const Registry extends LoaderRegistry>(
  options: LinguaOptions<Registry>,
): Lingua<Registry> {
  return new Lingua(options);
}
