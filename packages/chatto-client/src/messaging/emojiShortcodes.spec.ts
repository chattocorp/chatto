import { describe, expect, it } from 'vitest';
import { emojiShortcodeQuery, emojiShortcodes, resolveEmoji } from './emojiShortcodes.js';

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
  it.each([
    'omg:poop:foo',
    'hello:wave:',
    ':wave:hello',
    '_:wave:',
    ':wave:_',
    'é:wave:',
    ':wave:中',
    '𐐀:wave:',
    ':wave:𝟙',
    'e\u0301:wave:',
    ':wave:\u0301'
  ])('leaves tags beside word characters literal: %s', (source) => {
    expect([...emojiShortcodes(source)]).toEqual([]);
  });

  it.each(['(:wave:)!', '**:wave:**', '👋:wave:❤️', '\u00a0:wave:\u00a0', '\n:wave:\n'])(
    'accepts punctuation, emoji, and whitespace boundaries: %s',
    (source) => {
      expect([...emojiShortcodes(source)].map(({ resolution }) => resolution.name)).toEqual([
        'wave'
      ]);
    }
  );

  it('continues after a rejected match and keeps adjacent valid tags', () => {
    expect(
      [...emojiShortcodes('foo:poop:bar :wave::heart:')].map(({ resolution }) => resolution.name)
    ).toEqual(['wave', 'heart']);
  });

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

describe('emojiShortcodeQuery', () => {
  it.each([':he', '(:he', '!:he', '👋:he', ':wave::he', '\u00a0:he'])(
    'finds a partial name at a valid opening boundary: %s',
    (text) => {
      expect(emojiShortcodeQuery(text)).toEqual({ name: 'he', start: text.length - 3 });
    }
  );

  it.each(['omg:he', '_:he', 'é:he', '𐐀:he', 'e\u0301:he', ':heart:', 'hello', ':he\n', ':he\r\n'])(
    'rejects completed tags and word-adjacent names: %s',
    (text) => {
      expect(emojiShortcodeQuery(text)).toBeNull();
    }
  );

  it.each(['+1', '-1', 'woman_health_worker', 'custom-party'])(
    'accepts the same name characters as complete tags: %s',
    (name) => {
      expect(emojiShortcodeQuery(`:${name}`)).toEqual({ name, start: 0 });
    }
  );
});
