import { isTextDecoration, type TextDecoration } from '@chatto/client/messaging/textDecorations';
import type MarkdownIt from 'markdown-it';

/** A complete single-line decoration, including its delimiters. */
export type DecorationMatch = { tag: TextDecoration; raw: string; content: string };

/**
 * Match paired, attribute-free tags, with at most four nested decorations.
 * Code spans and Markdown link destinations cannot supply closing delimiters.
 * The visual editor escapes source HTML before its Markdown parser runs; its
 * tokenizer uses the encoded opening angle, keeping arbitrary HTML inert.
 */
export function matchTextDecoration(source: string, angle = '<'): DecorationMatch | undefined {
  const tagPattern = angle === '<' ? /^<(\/?)([a-z-]+)>/ : /^&lt;(\/?)([a-z-]+)>/;
  const opening = source.match(tagPattern);
  if (!opening || opening[1] || !isTextDecoration(opening[2])) return;
  const stack = [opening[2]];
  for (let i = opening[0].length; i < source.length;) {
    if (source[i] === '\n' || source[i] === '\r') return;
    if (source[i] === '`') {
      const delimiter = source.slice(i).match(/^`+/)![0];
      let end = i + delimiter.length;
      for (;;) {
        end = source.indexOf(delimiter, end);
        if (end < 0) break;
        if (source[end - 1] !== '`' && source[end + delimiter.length] !== '`') break;
        end += delimiter.length;
      }
      if (end >= 0 && !/[\r\n]/.test(source.slice(i, end))) {
        i = end + delimiter.length;
        continue;
      }
      i += delimiter.length;
      continue;
    }
    if (source.startsWith('](', i)) {
      let depth = 1;
      let end = i + 2;
      for (; end < source.length && depth; end++) {
        if (source[end] === '\n' || source[end] === '\r') return;
        if (source[end] === '\\') end++;
        else if (source[end] === '(') depth++;
        else if (source[end] === ')') depth--;
      }
      if (!depth) {
        i = end;
        continue;
      }
    }
    const tag = source.startsWith(angle, i) ? source.slice(i).match(tagPattern) : null;
    if (tag && isTextDecoration(tag[2])) {
      if (tag[1]) {
        if (stack.pop() !== tag[2]) return;
        if (!stack.length) {
          return {
            tag: opening[2],
            raw: source.slice(0, i + tag[0].length),
            content: source.slice(opening[0].length, i)
          };
        }
      } else {
        stack.push(tag[2]);
        if (stack.length > 4) return;
      }
      i += tag[0].length;
    } else i++;
  }
}

/** Recognize only our vocabulary while Markdown's general HTML rules stay disabled. */
export function textDecorationsPlugin(md: MarkdownIt): void {
  md.inline.ruler.before('autolink', 'text_decoration', (state, silent) => {
    // Link-label lookahead must see brackets inside a decoration so the
    // built-in link rule can reject nested Markdown links.
    if (silent) return false;
    if (state.src[state.pos] !== '<') return false;
    const match = matchTextDecoration(state.src.slice(state.pos, state.posMax));
    if (!match) return false;
    if (!silent) {
      const open = state.push('text_decoration_open', 'span', 1);
      open.attrSet('class', `text-decoration ${match.tag}`);
      const children: typeof state.tokens = [];
      // Preserve link and nesting context. A fresh top-level parse would allow
      // a decoration in a link label to create a nested anchor.
      const childState = new state.md.inline.State(match.content, state.md, state.env, children);
      childState.level = state.level;
      // markdown-it exposes linkLevel at runtime; its current declarations omit it.
      Object.assign(childState, {
        linkLevel: (state as typeof state & { linkLevel: number }).linkLevel
      });
      state.md.inline.tokenize(childState);
      for (const rule of state.md.inline.ruler2.getRules('')) rule(childState);
      state.tokens.push(...children);
      state.push('text_decoration_close', 'span', -1);
    }
    state.pos += match.raw.length;
    return true;
  });
}
