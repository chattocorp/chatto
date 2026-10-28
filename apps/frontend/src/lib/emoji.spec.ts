import { describe, expect, it } from 'vitest';
import { emojiToName, getEmojiDisplayName, getReactionDisplay, emojiWithTone } from './emoji';

describe('emojiWithTone', () => {
  it('uses supported variants and converts the result back to a reaction name', () => {
    expect(emojiWithTone('wave', 4)).toEqual({
      kind: 'unicode',
      name: 'wave::skin-tone-4',
      emoji: '👋🏽'
    });
    expect(emojiToName(emojiWithTone('+1', 4).emoji)).toBe('thumbsup::skin-tone-4');
    expect(emojiWithTone('woman_health_worker', 5).emoji).toBe('👩🏾‍⚕️');
  });
  it('keeps explicit tones, unsupported bases, and the yellow default', () => {
    expect(emojiWithTone('wave::skin-tone-2', 6).emoji).toBe('👋🏻');
    expect(emojiWithTone('heart', 4).emoji).toBe('❤️');
    expect(emojiWithTone('wave', 0).emoji).toBe('👋');
  });
});

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
  it('uses readable base labels for toned names and Unicode sequences', () => {
    expect(getEmojiDisplayName('thumbsup::skin-tone-4')).toBe('Thumbs up');
    expect(getEmojiDisplayName('👍🏽')).toBe('Thumbs up');
    expect(getEmojiDisplayName('woman_health_worker::skin-tone-5')).toBe('Woman Health Worker');
    expect(getEmojiDisplayName('couple::skin-tone-2-6')).toBe('Couple');
  });
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
