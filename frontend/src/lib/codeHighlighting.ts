import { createLowlight } from 'lowlight';
import { aliasesByLanguage, languageAliases } from 'virtual:chatto-highlight-language-metadata';
import type { LanguageFn } from 'highlight.js';

type HighlightLanguageModule = {
  default: LanguageFn;
};

const languageModules = import.meta.glob<HighlightLanguageModule>(
  [
    '/node_modules/highlight.js/es/languages/*.js',
    '!/node_modules/highlight.js/es/languages/*.js.js'
  ]
);

const languageImporters = new Map<string, () => Promise<HighlightLanguageModule>>();

for (const [path, importer] of Object.entries(languageModules)) {
  const language = path.match(/\/([^/]+)\.js$/)?.[1];
  if (language) languageImporters.set(language, importer);
}

function normalizeLanguageToken(value: string): string | null {
  return value.trim().toLowerCase().match(/[a-z0-9+#_.-]+/)?.[0] ?? null;
}

const languageLoadPromises = new Map<string, Promise<boolean>>();

const preferredCodeLanguageOptions = [
  { value: 'text', label: 'TEXT' },
  { value: 'ts', label: 'TS' },
  { value: 'js', label: 'JS' },
  { value: 'json', label: 'JSON' },
  { value: 'html', label: 'HTML' },
  { value: 'css', label: 'CSS' },
  { value: 'bash', label: 'BASH' },
  { value: 'py', label: 'PY' },
  { value: 'go', label: 'GO' },
  { value: 'rust', label: 'RUST' },
  { value: 'sql', label: 'SQL' },
  { value: 'yaml', label: 'YAML' },
  { value: 'md', label: 'MD' },
  { value: 'graphql', label: 'GRAPHQL' },
  { value: 'dockerfile', label: 'DOCKERFILE' },
  { value: 'ruby', label: 'RUBY' }
];

const preferredCodeLanguageValues = new Set(
  preferredCodeLanguageOptions.map((language) => language.value)
);

export const CODE_LANGUAGE_OPTIONS = [
  ...preferredCodeLanguageOptions,
  ...[...languageImporters.keys()]
    .filter((language) => !preferredCodeLanguageValues.has(language))
    .sort()
    .map((language) => ({
      value: language,
      label: language.toUpperCase()
    }))
];

export const lowlight = createLowlight();

export function normalizeCodeLanguage(language: string | null | undefined): string {
  return normalizeLanguageToken(language ?? '') ?? 'text';
}

export function resolveCodeLanguage(language: string | null | undefined): string | null {
  const normalized = normalizeCodeLanguage(language);
  if (languageImporters.has(normalized)) return normalized;
  return languageAliases[normalized] ?? null;
}

export function canHighlightCodeLanguage(language: string): boolean {
  return resolveCodeLanguage(language) !== null;
}

export function isCodeLanguageLoaded(language: string | null | undefined): boolean {
  const normalized = normalizeCodeLanguage(language);
  const resolved = resolveCodeLanguage(normalized);
  return Boolean(resolved && (lowlight.registered(normalized) || lowlight.registered(resolved)));
}

export async function ensureCodeLanguageLoaded(language: string | null | undefined): Promise<boolean> {
  const resolved = resolveCodeLanguage(language);
  if (!resolved) return false;
  if (lowlight.registered(resolveCodeLanguage(language) ?? resolved)) return false;

  const existing = languageLoadPromises.get(resolved);
  if (existing) return existing;

  const loadPromise = (async () => {
    const importer = languageImporters.get(resolved);
    if (!importer) return false;

    try {
      const module = await importer();
      lowlight.register(resolved, module.default);
      const aliases = aliasesByLanguage[resolved];
      if (aliases) lowlight.registerAlias(resolved, aliases);
      return true;
    } catch (err) {
      languageLoadPromises.delete(resolved);
      console.warn('[CodeHighlighting] Failed to load language:', resolved, err);
      return false;
    }
  })();

  languageLoadPromises.set(resolved, loadPromise);
  return loadPromise;
}

export async function ensureCodeLanguagesLoaded(
  languages: Iterable<string | null | undefined>
): Promise<boolean> {
  const results = await Promise.all([...languages].map((language) => ensureCodeLanguageLoaded(language)));
  return results.some(Boolean);
}
