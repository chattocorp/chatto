/** Render emoji shortcodes in parsed prose without changing the message source. */
import type MarkdownIt from 'markdown-it';
import type StateCore from 'markdown-it/lib/rules_core/state_core.mjs';
import type Token from 'markdown-it/lib/token.mjs';
import { emojiShortcodes } from '@chatto/client/messaging/emojiShortcodes';

/** Splits known shortcodes into dedicated tokens; unknown names stay literal. */
function splitEmojiText(state: StateCore, token: Token): Token[] {
  const result: Token[] = [];
  let start = 0;

  function appendText(end: number) {
    if (end === start) return;
    const text = new state.Token('text', '', 0);
    text.content = token.content.slice(start, end);
    text.level = token.level;
    result.push(text);
  }

  for (const match of emojiShortcodes(token.content)) {
    if (match.resolution.kind === 'unknown') continue;
    appendText(match.start);
    const emoji = new state.Token('emoji_shortcode', '', 0);
    emoji.content = match.resolution.emoji;
    emoji.markup = token.content.slice(match.start, match.end);
    emoji.level = token.level;
    result.push(emoji);
    start = match.end;
  }

  if (result.length === 0) return [token];
  appendText(token.content.length);
  return result;
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
    for (const child of token.children) {
      if (child.type === 'link_open') automaticLink = child.info === 'auto';
      if (child.type === 'link_close') automaticLink = false;
      if (child.type === 'text' && !automaticLink && child.content.includes(':')) {
        children.push(...splitEmojiText(state, child));
      } else {
        children.push(child);
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
