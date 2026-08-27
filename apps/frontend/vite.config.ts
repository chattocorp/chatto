/// <reference types="vitest/config" />
import { readdirSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, type Plugin, type UserConfig } from 'vite';

import { catalogSections, isPublicCatalogSection } from './src/lib/i18n/catalogSections.js';

// Backend target for dev proxy. Set CHATTO_BACKEND_URL to proxy to a remote
// backend (e.g. "https://dev.chatto.run") instead of a local one.
const backendTarget =
  process.env.CHATTO_BACKEND_URL ||
  `http://localhost:${process.env.CHATTO_WEBSERVER_PORT || '4000'}`;
const tiptapDeps = ['@tiptap/pm/state'];
const highlightLanguageMetadataModule = 'virtual:chatto-highlight-language-metadata';
const resolvedHighlightLanguageMetadataModule = `\0${highlightLanguageMetadataModule}`;

function localeCatalogChunkName(moduleId: string): string | null {
  const match = moduleId.match(/[\\/]messages[\\/]([^\\/]+)[\\/]([^\\/]+)\.json(?:\?.*)?$/);
  if (!match) return null;

  const [, locale, section] = match;
  if (locale === 'en-GB') return null;
  if (!catalogSections.includes(section as (typeof catalogSections)[number])) return null;

  const boundary = isPublicCatalogSection(section as (typeof catalogSections)[number])
    ? 'public'
    : 'chat';
  return `lingua-${locale}-${boundary}`;
}

function normalizeHighlightLanguageToken(value: string): string | null {
  return (
    value
      .trim()
      .toLowerCase()
      .match(/[a-z0-9+#_.-]+/)?.[0] ?? null
  );
}

function highlightLanguageNameCandidates(name: string): string[] {
  const base = name
    .toLowerCase()
    .replace(/\([^)]*\)/g, ' ')
    .replace(/&[a-z]+;/g, ' ')
    .replace(/[^a-z0-9+#_.-]+/g, ' ')
    .trim();
  const parts = base.split(/\s+/).filter(Boolean);
  return [...new Set([base, parts.join(''), parts.join('-')].filter(Boolean))];
}

function parseMarkdownTableCell(value: string): string {
  return value
    .replace(/<[^>]*>/g, '')
    .replace(/\[[^\]]*]\([^)]*\)/g, '')
    .trim();
}

function buildHighlightLanguageAliasMaps(): {
  aliasesByLanguage: Record<string, string[]>;
  languageAliases: Record<string, string>;
} {
  const supportedLanguagesMarkdown = readFileSync(
    new URL('./node_modules/highlight.js/SUPPORTED_LANGUAGES.md', import.meta.url),
    'utf8'
  );
  const bundledLanguages = new Set(
    readdirSync(new URL('./node_modules/highlight.js/es/languages/', import.meta.url))
      .filter((file) => file.endsWith('.js') && !file.endsWith('.js.js'))
      .map((file) => file.replace(/\.js$/, ''))
  );
  const aliasesByLanguage: Record<string, string[]> = {};
  const languageAliases: Record<string, string> = {};

  for (const line of supportedLanguagesMarkdown.split('\n')) {
    if (!line.startsWith('|') || line.includes('---')) continue;

    const cells = line
      .slice(1, -1)
      .split('|')
      .map((cell) => parseMarkdownTableCell(cell));
    const [rawName, rawAliases] = cells;
    if (!rawName || !rawAliases || rawName === 'Language') continue;

    const aliases = rawAliases
      .split(',')
      .map((alias) => normalizeHighlightLanguageToken(alias))
      .filter((alias): alias is string => Boolean(alias));
    const language = [...aliases, ...highlightLanguageNameCandidates(rawName)].find((candidate) =>
      bundledLanguages.has(candidate)
    );
    if (!language) continue;

    for (const alias of aliases) {
      if (alias === language || languageAliases[alias]) continue;
      languageAliases[alias] = language;
      aliasesByLanguage[language] ??= [];
      aliasesByLanguage[language].push(alias);
    }
  }

  return { aliasesByLanguage, languageAliases };
}

function highlightLanguageMetadata(): Plugin {
  let generatedCode: string | null = null;

  return {
    name: 'chatto-highlight-language-metadata',
    resolveId(id) {
      return id === highlightLanguageMetadataModule
        ? resolvedHighlightLanguageMetadataModule
        : null;
    },
    load(id) {
      if (id !== resolvedHighlightLanguageMetadataModule) return null;

      generatedCode ??= (() => {
        const metadata = buildHighlightLanguageAliasMaps();
        return [
          `export const aliasesByLanguage = ${JSON.stringify(metadata.aliasesByLanguage)};`,
          `export const languageAliases = ${JSON.stringify(metadata.languageAliases)};`
        ].join('\n');
      })();

      return generatedCode;
    }
  };
}

async function createTestConfig(): Promise<NonNullable<UserConfig['test']>> {
  const [{ playwright }, { storybookTest }] = await Promise.all([
    import('@vitest/browser-playwright'),
    import('@storybook/addon-vitest/vitest-plugin')
  ]);

  return {
    expect: { requireAssertions: true },
    projects: [
      {
        extends: './vite.config.ts',
        test: {
          name: 'client',
          browser: {
            enabled: true,
            provider: playwright(),
            headless: !process.env.SHOW_BROWSER,
            instances: [{ browser: 'chromium' }]
          },
          include: ['src/**/*.svelte.{test,spec}.{js,ts}'],
          exclude: ['src/lib/server/**'],
          setupFiles: ['./vitest-setup-client.ts'],
          deps: {
            optimizer: {
              web: {
                include: [...tiptapDeps]
              }
            }
          }
        }
      },
      {
        extends: './vite.config.ts',
        test: {
          name: 'server',
          environment: 'node',
          include: ['src/**/*.{test,spec}.{js,ts}'],
          exclude: ['src/**/*.svelte.{test,spec}.{js,ts}'],
          testTimeout: 10000 // CI is slower with Svelte module transforms
        }
      },
      {
        extends: true,
        plugins: [
          storybookTest({
            configDir: fileURLToPath(new URL('./.storybook', import.meta.url))
          })
        ],
        test: {
          name: 'storybook',
          browser: {
            enabled: true,
            provider: playwright(),
            headless: !process.env.SHOW_BROWSER,
            instances: [{ browser: 'chromium' }]
          }
        }
      }
    ]
  };
}

const testConfig = process.env.VITEST ? await createTestConfig() : undefined;

async function createServePlugins() {
  const { default: devtoolsJson } = await import('vite-plugin-devtools-json');
  return [devtoolsJson()];
}

export default defineConfig(async ({ command }) => {
  const servePlugins = command === 'serve' ? await createServePlugins() : [];

  return {
    clearScreen: false,
    plugins: [tailwindcss(), highlightLanguageMetadata(), sveltekit(), ...servePlugins],
    build: {
      reportCompressedSize: false,
      rolldownOptions: {
        output: {
          codeSplitting: {
            groups: [{ name: localeCatalogChunkName }]
          }
        }
      }
    },
    resolve: {
      alias: {
        // The lowlight package root re-exports `all`, which imports every
        // highlight.js grammar. We only need createLowlight, so point bundling
        // at the implementation module to keep language grammars lazy.
        lowlight: fileURLToPath(new URL('./node_modules/lowlight/lib/index.js', import.meta.url))
      }
    },
    ssr: {
      // TipTap is browser-only but imported in Svelte components that are
      // compiled for SSR. Bundle them into the SSR output to avoid
      // "could not be resolved" warnings (the code paths are guarded by
      // $effect which doesn't run during SSR).
      noExternal: [
        '@tiptap/core',
        '@tiptap/extension-code-block-lowlight',
        '@tiptap/extension-placeholder',
        '@tiptap/markdown',
        '@tiptap/starter-kit'
      ]
    },
    optimizeDeps: {
      include: [...tiptapDeps]
    },
    server: {
      // Proxy some URL routes to the Go backend process in development.
      port: process.env.VITE_PORT ? parseInt(process.env.VITE_PORT) : undefined,
      host: true,
      allowedHosts: ['fatso.fritz.box', '.orb.local'],
      // Regional Lingua catalogs live beside the Svelte source tree so the
      // public and chat section imports can remain independently lazy.
      fs: {
        allow: [fileURLToPath(new URL('./messages', import.meta.url))]
      },
      // Bind-mount inotify on macOS (Docker Desktop / OrbStack) drops events
      // during bursty changes. Polling is reliable; cost is negligible at this
      // tree size.
      watch: {
        usePolling: true,
        interval: 300
      },
      proxy: {
        '/oauth': {
          target: backendTarget,
          // Cookie authentication compares the browser Origin with the
          // request target. Preserve Vite's public Host so both values match.
          changeOrigin: false,
          cookieDomainRewrite: { '*': '' }
        },
        '/api': {
          target: backendTarget,
          ws: true,
          changeOrigin: false,
          secure: false,
          cookieDomainRewrite: { '*': '' },
          // Preserve the public browser origin so the backend can accept
          // same-origin cookie authentication on the WebSocket upgrade.
          rewriteWsOrigin: false
        },
        '/auth': {
          target: backendTarget,
          changeOrigin: false,
          cookieDomainRewrite: { '*': '' }
        },
        '/assets': {
          target: backendTarget,
          changeOrigin: true
        },
        '/.well-known/chatto/shields': {
          target: backendTarget,
          changeOrigin: true
        },
        '/webhooks': {
          target: backendTarget,
          changeOrigin: true
        }
      }
    },
    test: testConfig
  };
});
