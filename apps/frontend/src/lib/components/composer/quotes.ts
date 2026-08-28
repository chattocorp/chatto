import type { QuoteInsertionContent, SelectedQuoteBlock } from '$lib/state/room';

/** Normalize reply selections independently of either composer implementation. */
export function normalizeQuoteInsertionContent(text: QuoteInsertionContent): SelectedQuoteBlock[] {
  if (typeof text !== 'string') {
    return text
      .map((block) => ({
        quoteDepth: Math.max(0, Math.floor(block.quoteDepth)),
        text: block.text.replace(/\r\n?/g, '\n').trim()
      }))
      .filter((block) => block.text.length > 0);
  }

  const normalized = text.replace(/\r\n?/g, '\n').trim();
  if (!normalized) return [];
  return normalized.split('\n').map((line) => ({ quoteDepth: 0, text: line }));
}

/** Serialize selected reply blocks as Markdown blockquote source. */
export function serializeQuoteInsertionContent(text: QuoteInsertionContent): string {
  return normalizeQuoteInsertionContent(text)
    .flatMap((block) => {
      const prefix = '> '.repeat(block.quoteDepth + 1);
      return block.text.split('\n').map((line) => `${prefix}${line}`.trimEnd());
    })
    .join('\n');
}
