import MarkdownIt from 'markdown-it';
import type StateInline from 'markdown-it/lib/rules_inline/state_inline.mjs';
import tlds from 'tlds';
import { createHighlighterCore, type HighlighterCore } from 'shiki/core';
import { createJavaScriptRegexEngine } from 'shiki/engine/javascript';
import { fromHighlighter } from '@shikijs/markdown-it/core';

/**
 * Map of supported languages to their dynamic imports.
 * Only these languages can be loaded - keeps bundle minimal.
 * Add new languages here as needed.
 */
const LANGUAGE_IMPORTS: Record<string, () => Promise<unknown>> = {
  javascript: () => import('@shikijs/langs/javascript'),
  typescript: () => import('@shikijs/langs/typescript'),
  json: () => import('@shikijs/langs/json'),
  html: () => import('@shikijs/langs/html'),
  css: () => import('@shikijs/langs/css'),
  markdown: () => import('@shikijs/langs/markdown'),
  bash: () => import('@shikijs/langs/bash'),
  shellscript: () => import('@shikijs/langs/shellscript'),
  shell: () => import('@shikijs/langs/shellscript'),
  python: () => import('@shikijs/langs/python'),
  go: () => import('@shikijs/langs/go'),
  rust: () => import('@shikijs/langs/rust'),
  sql: () => import('@shikijs/langs/sql'),
  yaml: () => import('@shikijs/langs/yaml'),
  toml: () => import('@shikijs/langs/toml'),
  dockerfile: () => import('@shikijs/langs/dockerfile'),
  graphql: () => import('@shikijs/langs/graphql'),
  svelte: () => import('@shikijs/langs/svelte'),
  jsx: () => import('@shikijs/langs/jsx'),
  tsx: () => import('@shikijs/langs/tsx'),
  // Aliases
  js: () => import('@shikijs/langs/javascript'),
  ts: () => import('@shikijs/langs/typescript'),
  sh: () => import('@shikijs/langs/shellscript'),
  py: () => import('@shikijs/langs/python'),
  rb: () => import('@shikijs/langs/ruby'),
  ruby: () => import('@shikijs/langs/ruby')
};

/**
 * Disabled markdown-it rules - we only allow a subset of markdown syntax.
 */
const DISABLED_RULES = [
  // Block-level
  'heading',
  'lheading',
  'hr',
  'table',
  'reference',
  // Inline
  'image',
  'html_inline',
  // Backslash escapes turn `\_` into a literal `_`, which eats the arms of
  // common kaomoji like ¯\_(ツ)_/¯. Chat users type literal backslashes far
  // more often than they need CommonMark escapes; code spans still work for
  // escaping markdown chars when needed.
  'escape'
] as const;

const ALPHANUMERIC = /[a-zA-Z0-9]/;

/**
 * Inline rule that consumes `*` or `_` marker runs as literal text when they
 * are not at a word boundary. A word boundary requires exactly one side of
 * the run to be alphanumeric. This neuters intraword emphasis like
 * `foo*bar*baz` and punctuation-flanked markers like `_(ツ)_`, while
 * preserving normal `*italic*`, `_italic_`, and `**bold**`.
 */
function wordBoundaryEmphasis(state: StateInline, silent: boolean): boolean {
  const start = state.pos;
  const marker = state.src.charCodeAt(start);
  if (marker !== 0x2a /* * */ && marker !== 0x5f /* _ */) return false;

  let runEnd = start + 1;
  while (runEnd < state.posMax && state.src.charCodeAt(runEnd) === marker) {
    runEnd++;
  }

  const before = start > 0 ? state.src[start - 1] : '';
  const after = runEnd < state.src.length ? state.src[runEnd] : '';
  const beforeAlnum = ALPHANUMERIC.test(before);
  const afterAlnum = ALPHANUMERIC.test(after);

  // Intraword: definitely literal (`snake_case`, `foo*bar*baz`).
  const intraword = beforeAlnum && afterAlnum;
  // Kaomoji-like: punctuation on both sides AND no alphanumeric between this
  // run and the next same-marker (so `_(ツ)_` suppresses, `_...moo_` doesn't).
  let kaomojiLike = false;
  if (!beforeAlnum && !afterAlnum) {
    kaomojiLike = true;
    for (let i = runEnd; i < state.posMax; i++) {
      if (state.src.charCodeAt(i) === marker) break;
      if (ALPHANUMERIC.test(state.src[i])) {
        kaomojiLike = false;
        break;
      }
    }
  }
  if (intraword || kaomojiLike) {
    if (!silent) state.pending += state.src.slice(start, runEnd);
    state.pos = runEnd;
    return true;
  }

  return false;
}

// Singleton highlighter and markdown-it instances
let highlighter: HighlighterCore | null = null;
let md: MarkdownIt | null = null;

/**
 * Extract language names from fenced code blocks in markdown.
 */
function extractLanguages(markdown: string): string[] {
  const matches = markdown.matchAll(/```(\w+)/g);
  return [...new Set([...matches].map((m) => m[1].toLowerCase()))];
}

/**
 * Load any supported languages that haven't been loaded yet.
 * Only loads languages from LANGUAGE_IMPORTS.
 * Failures are logged but don't block rendering - code will render without highlighting.
 */
async function ensureLanguagesLoaded(langs: string[]): Promise<void> {
  if (!highlighter) return;

  const loaded = new Set(highlighter.getLoadedLanguages());
  const toLoad = langs.filter((l) => l in LANGUAGE_IMPORTS && !loaded.has(l));

  if (toLoad.length > 0) {
    try {
      console.log('[Shiki] Loading languages:', toLoad);
      // Load languages in parallel for better performance
      const modules = await Promise.all(toLoad.map((lang) => LANGUAGE_IMPORTS[lang]()));
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      await highlighter.loadLanguage(...(modules as any));
    } catch (err) {
      // Log but don't throw - code blocks will render without syntax highlighting
      console.warn('[Shiki] Failed to load languages:', toLoad, err);
    }
  }
}

// Common languages to preload during initialization
// These are loaded eagerly to avoid race conditions with the markdown-it plugin
const PRELOADED_LANGUAGES = [
  import('@shikijs/langs/javascript'),
  import('@shikijs/langs/typescript'),
  import('@shikijs/langs/json'),
  import('@shikijs/langs/bash'),
  import('@shikijs/langs/python'),
  import('@shikijs/langs/go')
];

/**
 * Initialize the highlighter and markdown-it instances.
 * Called once on first render.
 */
async function initialize(): Promise<void> {
  if (highlighter && md) return;

  // Create highlighter with core module
  // Uses JavaScript regex engine (smaller than WASM, ~40KB vs 622KB)
  // Preload common languages to avoid race conditions with the markdown-it plugin
  highlighter = await createHighlighterCore({
    themes: [import('@shikijs/themes/github-light'), import('@shikijs/themes/nord')],
    langs: PRELOADED_LANGUAGES,
    engine: createJavaScriptRegexEngine()
  });

  // Create markdown-it instance
  md = new MarkdownIt({
    html: false, // Disable HTML tags in source
    linkify: true, // Auto-convert URLs to links
    breaks: true // Convert \n to <br>
  });

  // Update linkify-it's TLD list so bare-domain URLs with newer TLDs
  // (.dev, .app, .io, etc.) are auto-linked
  md.linkify.tlds(tlds);

  // Disable unwanted syntax - only keep what we explicitly want
  md.disable([...DISABLED_RULES]);

  // Restrict `*` and `_` emphasis to word boundaries. Prevents intraword
  // emphasis (e.g. `snake_case`, `foo*bar*baz`) and emphasis between
  // punctuation (e.g. the underscores in `¯\_(ツ)_/¯`) from being parsed
  // as italics. Inserted before the `emphasis` rule so non-boundary marker
  // runs are consumed as literal text.
  md.inline.ruler.before('emphasis', 'word_boundary_emphasis', wordBoundaryEmphasis);

  // Add Shiki syntax highlighting with dual themes
  md.use(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    fromHighlighter(highlighter as any, {
      themes: {
        light: 'github-light',
        dark: 'nord'
      },
      // Fallback to plain text for unknown/unloaded languages
      // This prevents errors when a language isn't loaded yet or doesn't exist
      // Note: 'text' is valid at runtime but not in TS types (shiki special value)
      defaultLanguage: 'text' as unknown as undefined,
      fallbackLanguage: 'text' as unknown as undefined,
      // Add data-language attribute to pre element
      // Convert tabs to spaces for consistent rendering
      transformers: [
        {
          pre(node) {
            // this.options.lang contains the language from the code fence
            node.properties['data-language'] = this.options.lang || 'text';
          },
          code(node) {
            // Replace tab characters with spaces for consistent rendering.
            // Tabs interact poorly with the ::before pseudo-element used
            // for line numbers, causing inconsistent indentation.
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            function replaceTabs(n: any) {
              if (n.type === 'text') {
                n.value = n.value.replace(/\t/g, '    ');
              }
              if (n.children) {
                for (const child of n.children) replaceTabs(child);
              }
            }
            replaceTabs(node);
          }
        }
      ]
    })
  );

  // Customize link rendering for security
  const defaultLinkRender =
    md.renderer.rules.link_open ||
    function (tokens, idx, options, _env, self) {
      return self.renderToken(tokens, idx, options);
    };

  md.renderer.rules.link_open = function (tokens, idx, options, env, self) {
    const token = tokens[idx];
    const hrefIndex = token.attrIndex('href');

    if (hrefIndex >= 0) {
      const href = token.attrs![hrefIndex][1];

      // Only allow http and https URLs
      if (!href.startsWith('http://') && !href.startsWith('https://')) {
        // Replace dangerous URLs with empty href
        token.attrs![hrefIndex][1] = '#';
      }
    }

    // Add security attributes for external links
    token.attrSet('target', '_blank');
    token.attrSet('rel', 'noopener noreferrer');

    return defaultLinkRender(tokens, idx, options, env, self);
  };
}

/**
 * Returns true if the renderer has been initialized.
 */
export function isRendererReady(): boolean {
  return highlighter !== null && md !== null;
}

/**
 * Promise that resolves when the markdown renderer is ready.
 * Kept for backwards compatibility - initializes the renderer.
 */
export const rendererReady = initialize();

/**
 * Renders markdown to HTML.
 * Lazily loads syntax highlighting for any languages encountered.
 */
export async function renderMarkdown(body: string): Promise<string> {
  try {
    await initialize();

    // Load any languages we haven't seen before
    const languages = extractLanguages(body);
    if (languages.length > 0) {
      await ensureLanguagesLoaded(languages);
    }

    return md!.render(body);
  } catch (err) {
    console.error('[Markdown] renderMarkdown failed:', err, 'Body:', body.slice(0, 100));
    throw err;
  }
}
