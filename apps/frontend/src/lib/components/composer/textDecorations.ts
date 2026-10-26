import { Mark } from '@tiptap/core';
import { TEXT_DECORATIONS } from '@chatto/client/messaging/textDecorations';
import { matchTextDecoration } from '$lib/textDecorations';

/** Marks share the message renderer's fixed classes and serialize to portable tags. */
export const textDecorationMarks = TEXT_DECORATIONS.map((tag) =>
  Mark.create({
    name: tag,
    inclusive: false,
    parseHTML: () => [{ tag: `span.text-decoration.${tag}` }],
    renderHTML: () => ['span', { class: `text-decoration ${tag}` }, 0],
    markdownTokenizer: {
      name: tag,
      level: 'inline',
      start: (source) => source.indexOf(`&lt;${tag}>`),
      tokenize(source, _tokens, lexer) {
        if (!source.startsWith(`&lt;${tag}>`)) return;
        const match = matchTextDecoration(source, '&lt;');
        if (match) return { type: tag, raw: match.raw, tokens: lexer.inlineTokens(match.content) };
      }
    },
    parseMarkdown: (token, helpers) =>
      helpers
        .parseInline(token.tokens ?? [])
        .map((node) =>
          node.type === 'text' &&
          !node.marks?.some(
            (mark) =>
              mark.type === 'code' || (tag.startsWith('color-') && mark.type.startsWith('color-'))
          )
            ? { ...node, marks: [...(node.marks ?? []), { type: tag }] }
            : node
        ),
    renderMarkdown: (node, helpers) => `<${tag}>${helpers.renderChildren(node)}</${tag}>`
  })
);
