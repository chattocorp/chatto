// @vitest-environment node
import { describe, expect, it } from 'vitest';
import {
  createMessageTimestampToken,
  messageTimestampTokenPattern,
  parseMessageTimestampToken
} from './timestampTokens.js';

describe('message timestamp tokens', () => {
  it('creates and parses exact timestamp tokens', () => {
    expect(createMessageTimestampToken(1745764200)).toBe('<t:1745764200:F>');
    expect(parseMessageTimestampToken('<t:1745764200:F>')).toEqual({
      epochSeconds: 1745764200,
      format: 'F'
    });
  });

  it('rejects unsupported token formats and text around a token', () => {
    expect(parseMessageTimestampToken('<t:1745764200:R>')).toBeNull();
    expect(parseMessageTimestampToken('<t:abc:F>')).toBeNull();
    expect(parseMessageTimestampToken(' <t:1745764200:F>')).toBeNull();
  });

  it('rejects instants that Date cannot represent', () => {
    expect(() => createMessageTimestampToken(-1)).toThrow(RangeError);
    expect(() => createMessageTimestampToken(1.5)).toThrow(RangeError);
    expect(() => createMessageTimestampToken(9_000_000_000_000)).toThrow(RangeError);
  });

  it('finds every token candidate with an independent pattern', () => {
    const text = 'from <t:1:F> to <t:2:F>';
    const first = messageTimestampTokenPattern();
    expect([...text.matchAll(first)].map((match) => match[1])).toEqual(['1', '2']);
    first.exec(text);
    expect(messageTimestampTokenPattern().lastIndex).toBe(0);
  });
});
