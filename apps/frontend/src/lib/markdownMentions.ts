import type MarkdownIt from 'markdown-it';
import type StateCore from 'markdown-it/lib/rules_core/state_core.mjs';
import type Token from 'markdown-it/lib/token.mjs';

/**
 * Token type of an `@handle` mention in the markdown-it token stream. The
 * token `content` holds the handle without the `@`.
 */
export const MENTION_TOKEN_TYPE = 'mention';

/**
 * Attribute that marks an unresolved mention candidate in rendered HTML. Only
 * the mention renderer emits it; the markdown renderer disables source HTML.
 */
export const MENTION_HANDLE_ATTRIBUTE = 'data-mention-handle';

/**
 * markdown-it `env` key that turns mention tokenization on for one parse or
 * render. Only message bodies have mentions; other Markdown surfaces, such as
 * the MOTD and the server welcome message, leave `@handle` as plain text.
 */
export const MENTIONS_ENV_KEY = 'mentions';

function isAlphanumeric(value: string): boolean {
  return /^[a-zA-Z0-9]$/.test(value);
}

function isHandleChar(value: string): boolean {
  return /^[a-zA-Z0-9_-]$/.test(value);
}

/**
 * Reads the handle that starts at `start`, directly after an `@`. A handle
 * contains letters, digits, `_`, and `-`; a `.` is allowed only between two
 * handle characters, so a trailing sentence dot is not part of the handle.
 */
function readHandle(text: string, start: number): string {
  let stop = start;
  while (stop < text.length && isHandleChar(text[stop])) stop++;
  if (stop === start) return '';

  while (text[stop] === '.' && stop + 1 < text.length && isHandleChar(text[stop + 1])) {
    stop += 2;
    while (stop < text.length && isHandleChar(text[stop])) stop++;
  }
  return text.slice(start, stop);
}

/**
 * Splits one text token into text and mention tokens. `previous` is the source
 * character before the token; an `@` directly after a letter or digit (as in
 * `user@example.com`) does not start a mention.
 */
function splitTextToken(state: StateCore, token: Token, previous: string): Token[] | null {
  const text = token.content;
  const parts: Token[] = [];
  let textStart = 0;
  let at = text.indexOf('@');

  while (at !== -1) {
    const before = at > 0 ? text[at - 1] : previous;
    const handle = isAlphanumeric(before) ? '' : readHandle(text, at + 1);
    if (!handle) {
      at = text.indexOf('@', at + 1);
      continue;
    }

    if (at > textStart) {
      const leading = new state.Token('text', '', 0);
      leading.content = text.slice(textStart, at);
      leading.level = token.level;
      parts.push(leading);
    }
    const mention = new state.Token(MENTION_TOKEN_TYPE, '', 0);
    mention.content = handle;
    mention.markup = '@';
    mention.level = token.level;
    parts.push(mention);

    textStart = at + 1 + handle.length;
    at = text.indexOf('@', textStart);
  }

  if (parts.length === 0) return null;
  if (textStart < text.length) {
    const trailing = new state.Token('text', '', 0);
    trailing.content = text.slice(textStart);
    trailing.level = token.level;
    parts.push(trailing);
  }
  return parts;
}

/**
 * Replaces `@handle` text in one inline token stream with mention tokens.
 * Only plain text outside links is processed. Code spans are separate
 * `code_inline` tokens, and URLs are already links because this rule runs
 * after markdown-it's `linkify` rules.
 */
function tokenizeInlineMentions(state: StateCore, children: Token[]): Token[] {
  const result: Token[] = [];
  let linkDepth = 0;
  // Source character before the next token. Linkified URLs keep their text,
  // so the last URL character is the character before a following token.
  // Every other non-text token ends with markup punctuation, such as `*`, a
  // backtick, `)`, or `>`, or with a line break.
  let previous = '';

  for (const child of children) {
    if (child.type === 'link_open') linkDepth++;
    if (child.type === 'link_close') linkDepth = Math.max(0, linkDepth - 1);

    if (child.type !== 'text') {
      result.push(child);
      if (child.markup !== 'linkify') previous = '';
      continue;
    }

    const parts = linkDepth === 0 ? splitTextToken(state, child, previous) : null;
    if (parts) result.push(...parts);
    else result.push(child);
    previous = child.content.at(-1) ?? previous;
  }
  return result;
}

/**
 * Core rule that recognizes `@handle` mentions in the parsed token tree.
 * Mentions in blockquotes, code, and links (including linkified URLs such as
 * `https://example.social/@alice`) stay plain text. The server applies the
 * same rules in `cli/internal/core/mentions.go`; the shared cases in
 * `testdata/mentions/extraction.json` keep both implementations in agreement.
 */
function mentionCoreRule(state: StateCore): void {
  if (state.env?.[MENTIONS_ENV_KEY] !== true) return;

  let blockquoteDepth = 0;
  for (const token of state.tokens) {
    if (token.type === 'blockquote_open') blockquoteDepth++;
    else if (token.type === 'blockquote_close') blockquoteDepth = Math.max(0, blockquoteDepth - 1);
    else if (token.type === 'inline' && blockquoteDepth === 0 && token.children) {
      token.children = tokenizeInlineMentions(state, token.children);
    }
  }
}

/**
 * markdown-it plugin that adds mention tokens and renders each one as an
 * unresolved candidate: `<span data-mention-handle="alice">@alice</span>`.
 * The rule only runs when the markdown-it `env` sets {@link MENTIONS_ENV_KEY}.
 * Rendering does not know the room members; `resolveRenderedMentions` in
 * `$lib/mentions` resolves the candidates later.
 *
 * The rule runs after `linkify` and before `text_join`, so linkified URLs are
 * links and decoded entities are still separate `text_special` tokens. An
 * entity such as `&#64;` therefore never starts a mention.
 */
export function mentionPlugin(md: MarkdownIt): void {
  md.core.ruler.after('linkify', 'mention', mentionCoreRule);
  md.renderer.rules[MENTION_TOKEN_TYPE] = (tokens, idx) => {
    const handle = md.utils.escapeHtml(tokens[idx].content);
    return `<span ${MENTION_HANDLE_ATTRIBUTE}="${handle}">@${handle}</span>`;
  };
}

/**
 * Returns the mention handles in a parsed token stream, deduplicated, in order
 * of appearance.
 */
export function collectMentionHandles(tokens: Token[]): string[] {
  const handles = new Set<string>();
  for (const token of tokens) {
    for (const child of token.children ?? []) {
      if (child.type === MENTION_TOKEN_TYPE) handles.add(child.content);
    }
  }
  return [...handles];
}
