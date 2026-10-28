import { describe, expect, it } from 'vitest';
import { emojiShortcodes, resolveEmoji } from './emojiShortcodes.js';

describe('resolveEmoji', () => {
  it.each([
    ['wave', '👋'],
    ['heart', '❤️'],
    ['thumbsup', '👍'],
    ['+1', '👍'],
    ['-1', '👎'],
    ['woman_health_worker', '👩‍⚕️']
  ])('resolves the built-in name %s', (name, emoji) => {
    expect(resolveEmoji(name)).toEqual({ kind: 'unicode', name, emoji });
  });

  it.each(['foo', 'Wave', ':wave:', '👋', '', '__proto__', 'constructor', 'toString'])(
    'keeps unknown name %s literal',
    (name) => expect(resolveEmoji(name)).toEqual({ kind: 'unknown', name })
  );
});

describe('emojiShortcodes', () => {
  it('returns source offsets for adjacent tags, aliases, and unknown names', () => {
    const source = '👋 :wave::+1: :custom-party: :WAVE:';
    const matches = [...emojiShortcodes(source)];
    expect(matches.map(({ start, end }) => source.slice(start, end))).toEqual([
      ':wave:',
      ':+1:',
      ':custom-party:',
      ':WAVE:'
    ]);
    expect(matches[0]?.start).toBe(3);
    expect(matches.map(({ resolution }) => resolution.kind)).toEqual([
      'unicode',
      'unicode',
      'unknown',
      'unknown'
    ]);
  });

  it('ignores incomplete tags, spaces, punctuation, and non-ASCII names', () => {
    expect([...emojiShortcodes(': :two words: :foo.bar: :é: :wave')]).toEqual([]);
  });

  it('keeps iterators independent when a host stops parsing early', () => {
    const first = emojiShortcodes(':wave: :heart:');
    expect(first.next().value?.resolution.name).toBe('wave');
    expect([...emojiShortcodes(':heart:')]).toHaveLength(1);
    expect(first.next().value?.resolution.name).toBe('heart');
  });
});
