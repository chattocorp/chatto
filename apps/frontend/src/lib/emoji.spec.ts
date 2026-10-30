import { describe, expect, it } from 'vitest';
import {
  emojiToName,
  getEmojiDisplayName,
  getReactionDisplay,
  emojiWithTone,
  searchEmojis,
  EMOJI_BY_CATEGORY
} from './emoji';

describe('fuzzy emoji search', () => {
  it('finds names without separators, case-insensitively', () => {
    expect(searchEmojis('WomanHealthWorker')).toContainEqual(
      expect.objectContaining({ emoji: '👩‍⚕️' })
    );
    expect(searchEmojis('headshakingvertically')[0]).toMatchObject({
      name: 'head_shaking_vertically'
    });
  });

  it('matches aliases and keeps exact names ahead of fuzzy alternatives', () => {
    expect(searchEmojis('thumbsup')[0]).toMatchObject({ emoji: '👍' });
    expect(searchEmojis('fire')[0]).toMatchObject({ name: 'fire', emoji: '🔥' });
    const results = searchEmojis('hrt', 100);
    expect(results).toContainEqual(expect.objectContaining({ emoji: '❤️' }));
    expect(new Set(results.map((result) => result.emoji)).size).toBe(results.length);
  });

  it('respects result limits and rejects unmatched or empty queries', () => {
    expect(searchEmojis('face', 3)).toHaveLength(3);
    expect(searchEmojis('face', 0)).toEqual([]);
    expect(searchEmojis('zzzzzzzz')).toEqual([]);
    expect(searchEmojis('')).toEqual([]);
  });

  it('fuzzy-matches tags as well as shortcode aliases', () => {
    expect(searchEmojis('hry', 100)).toContainEqual(expect.objectContaining({ emoji: '🎉' }));
  });
});

it.each([
  ['face_with_bags_under_eyes', '🫩'],
  ['head_shaking_vertically', '🙂‍↕️'],
  ['leafless_tree', '🪾'],
  ['phoenix', '🐦‍🔥'],
  ['shovel', '🪏'],
  ['splatter', '🫟']
])('shows %s in reactions, search, and picker categories', (name, emoji) => {
  expect(getReactionDisplay(name)).toBe(emoji);
  expect(searchEmojis(name)[0]).toMatchObject({ name, emoji });
  expect(EMOJI_BY_CATEGORY.flatMap((category) => category.emojis)).toContainEqual({ name, emoji });
});

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
