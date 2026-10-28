/** Render emoji shortcodes in parsed prose without changing the message source. */
import type MarkdownIt from 'markdown-it';
import type StateCore from 'markdown-it/lib/rules_core/state_core.mjs';
import type Token from 'markdown-it/lib/token.mjs';
import { emojiShortcodes } from '@chatto/client/messaging/emojiShortcodes';
import { MENTION_TOKEN_TYPE } from './markdownMentions';

/** Appends shortcode and text tokens directly; unknown names stay literal. */
function appendEmojiText(state: StateCore, token: Token, previous: string, result: Token[]): void {
  let start = 0;

  function appendText(end: number) {
    if (end === start) return;
    const text = new state.Token('text', '', 0);
    text.content = token.content.slice(start, end);
    text.level = token.level;
    result.push(text);
  }

  for (const match of emojiShortcodes(previous + token.content)) {
    // The preceding character checks the boundary; it is not part of this token.
    if (match.start < previous.length || match.resolution.kind === 'unknown') continue;
    const matchStart = match.start - previous.length;
    const matchEnd = match.end - previous.length;
    appendText(matchStart);
    const emoji = new state.Token('emoji_shortcode', '', 0);
    emoji.content = match.resolution.emoji;
    emoji.markup = token.content.slice(matchStart, matchEnd);
    emoji.level = token.level;
    result.push(emoji);
    start = matchEnd;
  }

  if (start === 0) result.push(token);
  else appendText(token.content.length);
}

/**
 * Processes text after linkification and entity decoding. Code has separate
 * tokens. Automatic link labels retain the URL; explicit link labels can
 * contain emoji, but their destinations and titles are never processed.
 */
function emojiCoreRule(state: StateCore): void {
  for (const token of state.tokens) {
    if (token.type !== 'inline' || !token.children) continue;
    const children: Token[] = [];
    let automaticLink = false;
    // Mentions and linkified URLs preserve source characters. Their token
    // boundaries must not permit conversion in `@alice:wave:` or `example.com:wave:`.
    let previous = '';
    for (const child of token.children) {
      if (child.type === 'link_open') automaticLink = child.info === 'auto';
      if (child.type === 'link_close') automaticLink = false;
      if (child.type === 'text' && !automaticLink && child.content.includes(':')) {
        appendEmojiText(state, child, previous, children);
      } else {
        children.push(child);
      }
      if (child.type === 'text' || child.type === MENTION_TOKEN_TYPE) {
        // Keep the final Unicode code point; a line break is also a boundary.
        previous = child.content.match(/.$/u)?.[0] ?? '';
      } else if (child.markup !== 'linkify') {
        previous = '';
      }
    }
    token.children = children;
  }
}

/** Adds built-in emoji rendering to full and inline Markdown surfaces. */
export function emojiPlugin(md: MarkdownIt): void {
  md.core.ruler.after('text_join', 'emoji_shortcode', emojiCoreRule);
  md.renderer.rules.emoji_shortcode = (tokens, index) => md.utils.escapeHtml(tokens[index].content);
}
