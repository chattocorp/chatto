import { describe, expect, it } from 'vitest';
import { getEmojiDisplayName } from './emoji';

describe('getEmojiDisplayName', () => {
  it('formats known reaction shortcodes as readable names', () => {
    expect(getEmojiDisplayName('thumbsup')).toBe('Thumbs up');
    expect(getEmojiDisplayName('woman_health_worker')).toBe('Woman Health Worker');
  });

  it('formats unicode emoji as readable names', () => {
    expect(getEmojiDisplayName('🚀')).toBe('Rocket');
    expect(getEmojiDisplayName('❤️')).toBe('Heart');
  });

  it('falls back to a readable version of unknown names', () => {
    expect(getEmojiDisplayName('custom-party')).toBe('Custom Party');
  });
});
