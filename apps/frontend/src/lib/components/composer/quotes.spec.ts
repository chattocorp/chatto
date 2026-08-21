import { describe, expect, it } from 'vitest';
import { normalizeQuoteInsertionContent, serializeQuoteInsertionContent } from './quotes';

describe('normalizeQuoteInsertionContent', () => {
  it('normalizes plain selected text into top-level quote blocks', () => {
    expect(normalizeQuoteInsertionContent(' First\r\nSecond ')).toEqual([
      { quoteDepth: 0, text: 'First' },
      { quoteDepth: 0, text: 'Second' }
    ]);
  });

  it('normalizes structured quote depth and drops empty blocks', () => {
    expect(
      normalizeQuoteInsertionContent([
        { quoteDepth: 1.9, text: ' Nested\r\nline ' },
        { quoteDepth: -3, text: ' Root ' },
        { quoteDepth: 4, text: '  ' }
      ])
    ).toEqual([
      { quoteDepth: 1, text: 'Nested\nline' },
      { quoteDepth: 0, text: 'Root' }
    ]);
  });
});

describe('serializeQuoteInsertionContent', () => {
  it('preserves nested quote depth in Markdown source', () => {
    expect(
      serializeQuoteInsertionContent([
        { quoteDepth: 0, text: 'Root' },
        { quoteDepth: 1, text: 'Nested\nline' }
      ])
    ).toBe('> Root\n> > Nested\n> > line');
  });
});
