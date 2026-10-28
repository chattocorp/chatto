/** Render emoji shortcodes in parsed prose without changing the message source. */
import type MarkdownIt from 'markdown-it';
import type StateCore from 'markdown-it/lib/rules_core/state_core.mjs';
import type Token from 'markdown-it/lib/token.mjs';
import { emojiShortcodes } from '@chatto/client/messaging/emojiShortcodes';
import { MENTION_TOKEN_TYPE } from './markdownMentions';

/** Splits known shortcodes into dedicated tokens; unknown names stay literal. */
function splitEmojiText(state: StateCore, token: Token, previous?: Token): Token[] {
  const result: Token[] = [];
  let start = 0;

  function appendText(end: number) {
    if (end === start) return;
    const text = new state.Token('text', '', 0);
    text.content = token.content.slice(start, end);
    text.level = token.level;
    result.push(text);
  }

  // A mention token replaces literal text, so it must not create a word
  // boundary in `@alice:wave:`. Other inline tokens have Markdown delimiters.
  const prefix = previous?.type === MENTION_TOKEN_TYPE ? previous.content : '';
  for (const match of emojiShortcodes(prefix + token.content)) {
    if (match.resolution.kind === 'unknown') continue;
    const matchStart = match.start - prefix.length;
    const matchEnd = match.end - prefix.length;
    appendText(matchStart);
    const emoji = new state.Token('emoji_shortcode', '', 0);
    emoji.content = match.resolution.emoji;
    emoji.markup = token.content.slice(matchStart, matchEnd);
    emoji.level = token.level;
    result.push(emoji);
    start = matchEnd;
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
    let previous: Token | undefined;
    for (const child of token.children) {
      if (child.type === 'link_open') automaticLink = child.info === 'auto';
      if (child.type === 'link_close') automaticLink = false;
      if (child.type === 'text' && !automaticLink && child.content.includes(':')) {
        // Attachments can contain more tokens than a function call accepts.
        for (const part of splitEmojiText(state, child, previous)) children.push(part);
      } else {
        children.push(child);
      }
      previous = child;
    }
    token.children = children;
  }
}

/** Adds built-in emoji rendering to full and inline Markdown surfaces. */
export function emojiPlugin(md: MarkdownIt): void {
  md.core.ruler.after('text_join', 'emoji_shortcode', emojiCoreRule);
  md.renderer.rules.emoji_shortcode = (tokens, index) => md.utils.escapeHtml(tokens[index].content);
}
