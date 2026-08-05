import { CatalogValidationError } from "./errors.js";
import { isCountTranslationKey } from "./conventions.js";
import { translationPlaceholders } from "./format.js";
import {
  pluralCategories,
  type PluralCategory,
  type PluralTranslation,
  type TranslationDocument,
  type TranslationModule,
  type TranslationObject,
} from "./types.js";

const pluralCategorySet = new Set<string>(pluralCategories);

function placeholderSignature(template: string): string {
  return [...new Set(translationPlaceholders(template))].sort().join("\u0000");
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isPluralObject(
  value: Record<string, unknown>,
): value is PluralTranslation {
  const keys = Object.keys(value);
  return (
    keys.length > 0 &&
    keys.every(
      (key) => pluralCategorySet.has(key) && typeof value[key] === "string",
    )
  );
}

function validatePlural(
  value: Record<string, unknown>,
  key: string,
): asserts value is PluralTranslation {
  if (!isPluralObject(value)) {
    throw new CatalogValidationError(
      `Plural translation "${key}" must contain only CLDR plural categories with string values`,
    );
  }
  if (typeof value.other !== "string") {
    throw new CatalogValidationError(
      `Plural translation "${key}" must define an "other" value`,
    );
  }
  const expectedPlaceholders = placeholderSignature(value.other);
  for (const translation of Object.values(value)) {
    if (placeholderSignature(translation) !== expectedPlaceholders) {
      throw new CatalogValidationError(
        `Plural translation "${key}" must use the same placeholders in every category`,
      );
    }
  }
}

function validateNode(value: unknown, key: string): void {
  if (typeof value === "string") {
    if (isCountTranslationKey(key)) {
      throw new CatalogValidationError(
        `Plural translation "${key}" must be an object`,
      );
    }
    return;
  }
  if (!isObject(value)) {
    throw new CatalogValidationError(
      `Translation "${key}" must be a string or object`,
    );
  }
  if (isCountTranslationKey(key)) {
    validatePlural(value, key);
    return;
  }
  if (isPluralObject(value)) {
    throw new CatalogValidationError(
      `Plural translation "${key}" must use a key ending in _count or .count`,
    );
  }
  for (const [childKey, childValue] of Object.entries(value)) {
    validateNode(childValue, `${key}.${childKey}`);
  }
}

function unwrapModule(loaded: TranslationModule): TranslationDocument {
  if ("default" in loaded && isObject(loaded.default)) {
    return loaded.default;
  }
  return loaded;
}

export function extractSection(
  loaded: TranslationModule,
  section: string,
): TranslationObject {
  const document = unwrapModule(loaded);
  const value = document[section];
  if (!isObject(value)) {
    throw new CatalogValidationError(
      `Catalog for section "${section}" must contain a top-level "${section}" object`,
    );
  }
  validateNode(value, section);
  return value as TranslationObject;
}

function assertMatchingPlaceholders(
  base: string,
  translated: string,
  key: string,
): void {
  if (placeholderSignature(base) !== placeholderSignature(translated)) {
    throw new CatalogValidationError(
      `Translation "${key}" must preserve the base locale placeholders`,
    );
  }
}

function validateOverlayNode(
  base: unknown,
  translated: unknown,
  key: string,
): void {
  if (typeof translated === "string") {
    if (typeof base !== "string") {
      throw new CatalogValidationError(
        `Translation "${key}" must have the same value kind as the base locale`,
      );
    }
    assertMatchingPlaceholders(base, translated, key);
    return;
  }

  if (!isObject(translated) || !isObject(base)) {
    throw new CatalogValidationError(
      `Translation "${key}" must have the same value kind as the base locale`,
    );
  }
  if (isCountTranslationKey(key)) {
    if (!isPluralObject(base) || !isPluralObject(translated)) {
      throw new CatalogValidationError(
        `Translation "${key}" must have the same value kind as the base locale`,
      );
    }
    for (const [category, translation] of Object.entries(translated)) {
      assertMatchingPlaceholders(base.other, translation, `${key}.${category}`);
    }
    return;
  }

  for (const [childKey, childValue] of Object.entries(translated)) {
    if (!(childKey in base)) {
      throw new CatalogValidationError(
        `Translation "${key}.${childKey}" does not exist in the base locale`,
      );
    }
    validateOverlayNode(base[childKey], childValue, `${key}.${childKey}`);
  }
}

export function validateTranslationOverlay(
  base: TranslationObject,
  translated: TranslationObject,
  section: string,
): void {
  validateOverlayNode(base, translated, section);
}

export function findTranslation(
  section: TranslationObject | undefined,
  path: readonly string[],
): string | PluralTranslation | undefined {
  let current: unknown = section;
  for (const segment of path) {
    if (!isObject(current) || !(segment in current)) return undefined;
    current = current[segment];
  }
  if (
    typeof current === "string" ||
    (isObject(current) && isPluralObject(current))
  ) {
    return current as string | PluralTranslation;
  }
  return undefined;
}

export function selectPlural(
  plural: PluralTranslation,
  category: PluralCategory,
): string {
  return plural[category] ?? plural.other;
}
