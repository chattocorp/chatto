import { describe, expect, it } from 'vitest';
import { emojiToName, getEmojiDisplayName, getReactionDisplay } from './emoji';

describe('getReactionDisplay', () => {
  it('treats object property names as literal unknown names', () => {
    for (const name of ['constructor', 'toString', '__proto__']) {
      expect(emojiToName(name)).toBeUndefined();
      expect(getReactionDisplay(name)).toBe(`:${name}:`);
      expect(typeof getEmojiDisplayName(name)).toBe('string');
    }
  });

  it('renders built-ins and preserves unknown reaction names', () => {
    expect(getReactionDisplay('heart')).toBe('❤️');
    expect(getReactionDisplay('Party_Parrot')).toBe(':Party_Parrot:');
  });
});

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
