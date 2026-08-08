import { describe, expect, it } from 'vitest';
import { fromInlineEndOffset, getDocumentDirection, toInlineEndDelta } from './direction';

describe('document direction helpers', () => {
  it('falls back to left-to-right without a document', () => {
    expect(getDocumentDirection()).toBe('ltr');
  });

  it('keeps horizontal values unchanged in left-to-right documents', () => {
    expect(toInlineEndDelta(24, 'ltr')).toBe(24);
    expect(fromInlineEndOffset(300, 'ltr')).toBe(300);
  });

  it('mirrors horizontal values in right-to-left documents', () => {
    expect(toInlineEndDelta(24, 'rtl')).toBe(-24);
    expect(fromInlineEndOffset(300, 'rtl')).toBe(-300);
  });
});
