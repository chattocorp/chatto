import { createLowlight } from 'lowlight';
import bash from 'highlight.js/lib/languages/bash';
import css from 'highlight.js/lib/languages/css';
import dockerfile from 'highlight.js/lib/languages/dockerfile';
import go from 'highlight.js/lib/languages/go';
import graphql from 'highlight.js/lib/languages/graphql';
import javascript from 'highlight.js/lib/languages/javascript';
import json from 'highlight.js/lib/languages/json';
import markdown from 'highlight.js/lib/languages/markdown';
import plaintext from 'highlight.js/lib/languages/plaintext';
import python from 'highlight.js/lib/languages/python';
import ruby from 'highlight.js/lib/languages/ruby';
import rust from 'highlight.js/lib/languages/rust';
import shell from 'highlight.js/lib/languages/shell';
import sql from 'highlight.js/lib/languages/sql';
import typescript from 'highlight.js/lib/languages/typescript';
import xml from 'highlight.js/lib/languages/xml';
import yaml from 'highlight.js/lib/languages/yaml';

const languages = {
  bash,
  css,
  dockerfile,
  go,
  graphql,
  javascript,
  json,
  markdown,
  plaintext,
  python,
  ruby,
  rust,
  shell,
  sql,
  typescript,
  xml,
  yaml
};

const languageAliases: Record<string, string[]> = {
  bash: ['sh'],
  javascript: ['js', 'jsx'],
  markdown: ['md'],
  plaintext: ['text', 'txt'],
  python: ['py'],
  shell: ['shellscript'],
  typescript: ['ts', 'tsx'],
  xml: ['html', 'svelte']
};

const aliasToLanguage = new Map<string, string>();
const registeredLanguages = new Set(Object.keys(languages));

for (const [language, aliases] of Object.entries(languageAliases)) {
  for (const alias of aliases) {
    aliasToLanguage.set(alias, language);
  }
}

export const CODE_LANGUAGE_OPTIONS = [
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
] as const;

export function createChattoLowlight() {
  const lowlight = createLowlight(languages);
  lowlight.registerAlias(languageAliases);
  return lowlight;
}

export function normalizeCodeLanguage(language: string | null | undefined): string {
  const token = language?.trim().toLowerCase().match(/[a-z0-9_-]+/)?.[0];
  return token || 'text';
}

export function canHighlightCodeLanguage(language: string): boolean {
  const normalized = normalizeCodeLanguage(language);
  return registeredLanguages.has(normalized) || aliasToLanguage.has(normalized);
}
