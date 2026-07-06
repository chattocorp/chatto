import MarkdownIt from 'markdown-it';
import type StateInline from 'markdown-it/lib/rules_inline/state_inline.mjs';
import tlds from 'tlds';
import { classifyMessageBodyChatLink } from '$lib/messageLinks';

type CodeHighlightingModule = typeof import('$lib/codeHighlighting');

/**
 * Disabled markdown-it rules - we only allow a subset of markdown syntax.
 */
const DISABLED_RULES = [
  // Block-level
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
  const runLength = runEnd - start;

  const before = start > 0 ? state.src[start - 1] : '';
  const after = runEnd < state.src.length ? state.src[runEnd] : '';
  const beforeAlnum = ALPHANUMERIC.test(before);
  const afterAlnum = ALPHANUMERIC.test(after);

  // Single-marker intraword runs are definitely literal (`snake_case`,
  // `foo*bar*baz`). Double-marker runs are still allowed so bold can end next
  // to a following word (`**bold**text`).
  const intraword = runLength === 1 && beforeAlnum && afterAlnum;
  // Kaomoji-like: punctuation on both sides AND neither direction crosses an
  // alphanumeric before hitting a same-marker run or the input boundary. The
  // bidirectional check distinguishes a true kaomoji marker (e.g. the trailing
  // `_` in `_(ツ)_/¯` — only punctuation back to the opener and only
  // punctuation forward to end of input) from a closer of a real emphasis
  // run that happens to be followed by punctuation/another emphasis (e.g.
  // the closing `**` in `**foo:** **bar**` — alnum `o` is right behind it).
  let kaomojiLike = false;
  if (!beforeAlnum && !afterAlnum) {
    let forwardOK = true;
    for (let i = runEnd; i < state.posMax; i++) {
      if (state.src.charCodeAt(i) === marker) break;
      if (ALPHANUMERIC.test(state.src[i])) {
        forwardOK = false;
        break;
      }
    }
    if (forwardOK) {
      kaomojiLike = true;
      for (let i = start - 1; i >= 0; i--) {
        if (state.src.charCodeAt(i) === marker) break;
        if (ALPHANUMERIC.test(state.src[i])) {
          kaomojiLike = false;
          break;
        }
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

let md: MarkdownIt | null = null;
let codeHighlighting: CodeHighlightingModule | null = null;

type LowlightText = {
  type: 'text';
  value: string;
};

type LowlightElement = {
  type: 'element';
  tagName: string;
  properties?: Record<string, unknown>;
  children?: LowlightNode[];
};

type LowlightNode =
  | LowlightText
  | LowlightElement
  | {
      type: string;
      children?: LowlightNode[];
    };

function escapeHtml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;');
}

function escapeAttribute(value: string): string {
  return escapeHtml(value).replaceAll("'", '&#39;');
}

function renderClassName(value: unknown): string | null {
  if (Array.isArray(value)) {
    const classes = value.filter((item): item is string => typeof item === 'string');
    return classes.length > 0 ? classes.join(' ') : null;
  }

  return typeof value === 'string' && value.length > 0 ? value : null;
}

function renderElementOpen(node: LowlightElement): string {
  const className = renderClassName(node.properties?.className);
  const classAttribute = className ? ` class="${escapeAttribute(className)}"` : '';
  return `<${node.tagName}${classAttribute}>`;
}

function isLowlightText(node: LowlightNode): node is LowlightText {
  return node.type === 'text';
}

function isLowlightElement(node: LowlightNode): node is LowlightElement {
  return node.type === 'element';
}

function renderLowlightLines(nodes: LowlightNode[]): string[] {
  const lines = [''];

  function append(value: string) {
    lines[lines.length - 1] += value;
  }

  function renderNode(node: LowlightNode, activeOpen: string, activeClose: string) {
    if (isLowlightText(node)) {
      const parts = node.value.replaceAll('\t', '    ').split('\n');

      for (let i = 0; i < parts.length; i++) {
        if (i > 0) {
          append(activeClose);
          lines.push(activeOpen);
        }
        append(escapeHtml(parts[i]));
      }
      return;
    }

    if (isLowlightElement(node)) {
      const open = renderElementOpen(node);
      const close = `</${node.tagName}>`;
      append(open);

      for (const child of node.children ?? []) {
        renderNode(child, `${activeOpen}${open}`, `${close}${activeClose}`);
      }

      append(close);
      return;
    }

    for (const child of 'children' in node ? (node.children ?? []) : []) {
      renderNode(child, activeOpen, activeClose);
    }
  }

  for (const node of nodes) {
    renderNode(node, '', '');
  }

  return lines;
}

function renderPlainCodeLines(code: string): string[] {
  return code.replaceAll('\t', '    ').split('\n').map(escapeHtml);
}

function renderCodeFence(code: string, rawLanguage: string): string {
  const displayLanguage = normalizeCodeLanguage(rawLanguage);
  const resolvedLanguage = codeHighlighting?.resolveCodeLanguage(displayLanguage);
  const displayCode = code.replace(/\r?\n$/, '');
  const lines =
    resolvedLanguage && codeHighlighting?.lowlight.registered(displayLanguage)
      ? renderLowlightLines(
          (
            codeHighlighting.lowlight.highlight(displayLanguage, displayCode) as {
              children: LowlightNode[];
            }
          ).children
        )
      : resolvedLanguage && codeHighlighting?.lowlight.registered(resolvedLanguage)
        ? renderLowlightLines(
            (
              codeHighlighting.lowlight.highlight(resolvedLanguage, displayCode) as {
                children: LowlightNode[];
              }
            ).children
          )
        : renderPlainCodeLines(displayCode);
  const lineHtml = lines.map((line) => `<span class="line">${line}</span>`).join('');

  return `<pre class="hljs" data-language="${escapeAttribute(displayLanguage)}"><code class="language-${escapeAttribute(displayLanguage)}">${lineHtml}</code></pre>`;
}

function normalizeCodeLanguage(language: string | null | undefined): string {
  const token = language
    ?.trim()
    .toLowerCase()
    .match(/[a-z0-9+#_.-]+/)?.[0];
  return token || 'text';
}

function extractFenceLanguages(markdown: string): string[] {
  const languages = new Set<string>();
  const fencePattern = /^[ \t]*(```|~~~)[ \t]*([^\s`~]*)/gm;
  let match: RegExpExecArray | null;

  while ((match = fencePattern.exec(markdown))) {
    languages.add(normalizeCodeLanguage(match[2]));
  }

  return [...languages];
}

async function ensureFenceLanguagesLoaded(languages: string[]): Promise<void> {
  if (languages.length === 0) return;

  codeHighlighting ??= await import('$lib/codeHighlighting');
  await codeHighlighting.ensureCodeLanguagesLoaded(languages);
}

/**
 * Initialize the markdown-it instance.
 * Called once on first render.
 */
function initialize(): void {
  if (md) return;

  md = new MarkdownIt({
    html: false, // Disable HTML tags in source
    linkify: true, // Auto-convert URLs to links
    breaks: true, // Convert \n to <br>
    highlight: renderCodeFence
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

  // Customize link rendering for security
  const defaultLinkRender =
    md.renderer.rules.link_open ||
    function (tokens, idx, options, _env, self) {
      return self.renderToken(tokens, idx, options);
    };

  md.renderer.rules.link_open = function (tokens, idx, options, env, self) {
    const token = tokens[idx];
    const hrefIndex = token.attrIndex('href');
    let allowedSameTabChatLink = false;

    if (hrefIndex >= 0) {
      const href = token.attrs![hrefIndex][1];

      // Only allow http and https URLs
      if (!href.startsWith('http://') && !href.startsWith('https://')) {
        // Replace dangerous URLs with empty href
        token.attrs![hrefIndex][1] = '#';
      } else {
        allowedSameTabChatLink = classifyMessageBodyChatLink(href) !== null;
      }
    }

    // External and non-allow-listed links open out-of-band. Known same-origin
    // chat routes intentionally keep normal same-tab navigation semantics.
    if (!allowedSameTabChatLink) {
      token.attrSet('target', '_blank');
      token.attrSet('rel', 'noopener noreferrer');
    }

    return defaultLinkRender(tokens, idx, options, env, self);
  };
}

/**
 * Returns true if the renderer has been initialized.
 */
export function isRendererReady(): boolean {
  return md !== null;
}

/**
 * Promise that resolves when the markdown renderer is ready.
 * Kept for backwards compatibility - initializes the renderer.
 */
export const rendererReady = Promise.resolve().then(initialize);

/**
 * Renders markdown to HTML.
 */
export async function renderMarkdown(body: string): Promise<string> {
  try {
    await ensureFenceLanguagesLoaded(extractFenceLanguages(body));
    initialize();

    return md!.render(body);
  } catch (err) {
    console.error('[Markdown] renderMarkdown failed:', err, { bodyLength: body.length });
    throw err;
  }
}
