// @vitest-environment node
import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { nameToEmoji } from 'gemoji';
import {
  emojiShortcodeQuery,
  emojiShortcodes,
  resolveEmoji,
  emojiNameFromUnicode
} from './emojiShortcodes.js';

describe('emojiNameFromUnicode', () => {
  it.each([
    ['👍', 'thumbsup'],
    ['👎', 'thumbsdown'],
    ['👍🏽', 'thumbsup::skin-tone-4'],
    ['👩🏾‍⚕️', 'woman_health_worker::skin-tone-5'],
    ['👩🏻‍🤝‍👨🏿', 'couple::skin-tone-2-6']
  ])('keeps a stable name for %s', (emoji, name) => {
    expect(emojiNameFromUnicode(emoji)).toBe(name);
  });
  it('rejects text, prototype properties, and invalid sequences', () => {
    for (const emoji of ['hello', 'constructor', '❤️🏽', '👍👍'])
      expect(emojiNameFromUnicode(emoji)).toBeUndefined();
  });
});

describe('resolveEmoji', () => {
  it.each([
    ['wave', '👋'],
    ['heart', '❤️'],
    ['thumbsup', '👍'],
    ['+1', '👍'],
    ['-1', '👎'],
    ['woman_health_worker', '👩‍⚕️'],
    ['thumbsup::skin-tone-4', '👍🏽'],
    ['+1::skin-tone-2', '👍🏻'],
    ['wave::skin-tone-3', '👋🏼'],
    ['v::skin-tone-6', '✌🏿'],
    ['woman_health_worker::skin-tone-5', '👩🏾‍⚕️'],
    ['people_holding_hands::skin-tone-4', '🧑🏽‍🤝‍🧑🏽'],
    ['handshake::skin-tone-2-6', '🫱🏻‍🫲🏿'],
    ['couple::skin-tone-2-6', '👩🏻‍🤝‍👨🏿']
  ])('resolves the built-in name %s', (name, emoji) => {
    expect(resolveEmoji(name)).toEqual({ kind: 'unicode', name, emoji });
  });

  it.each(['foo', 'Wave', ':wave:', '👋', '', '__proto__', 'constructor', 'toString'])(
    'keeps unknown name %s literal',
    (name) => expect(resolveEmoji(name)).toEqual({ kind: 'unknown', name })
  );

  it.each([
    'heart::skin-tone-4',
    'wave::skin-tone-1',
    'wave::skin-tone-7',
    'wave::skin-tone-4::skin-tone-5',
    'custom::skin-tone-4'
  ])('rejects unsupported tone sequence %s', (name) => {
    expect(resolveEmoji(name)).toEqual({ kind: 'unknown', name });
  });

  it('agrees with the converter map and preserves every existing gemoji name', () => {
    const map = JSON.parse(
      readFileSync(new URL('../../../../tools/emoji-map.json', import.meta.url), 'utf8')
    ) as Record<string, string>;
    for (const [name, emoji] of Object.entries(map)) {
      expect(resolveEmoji(name)).toEqual({ kind: 'unicode', name, emoji });
    }
    for (const [name, emoji] of Object.entries(nameToEmoji)) expect(map[name]).toBe(emoji);
  });
});

describe('emojiShortcodes', () => {
  it('parses mixed-tone pairs as a single shortcode', () => {
    const matches = [...emojiShortcodes(':couple::skin-tone-2-6:')];
    expect(matches).toEqual([
      {
        start: 0,
        end: 23,
        resolution: { kind: 'unicode', name: 'couple::skin-tone-2-6', emoji: '👩🏻‍🤝‍👨🏿' }
      }
    ]);
  });
  it('consumes complete tone sequences and preserves UTF-16 offsets', () => {
    const source = '😀 :thumbsup::skin-tone-4::heart: :woman_health_worker::skin-tone-2:';
    const matches = [...emojiShortcodes(source)];
    expect(matches.map(({ start, end }) => source.slice(start, end))).toEqual([
      ':thumbsup::skin-tone-4:',
      ':heart:',
      ':woman_health_worker::skin-tone-2:'
    ]);
    expect(
      matches.map(({ resolution }) => resolution.kind === 'unicode' && resolution.emoji)
    ).toEqual(['👍🏽', '❤️', '👩🏻‍⚕️']);
  });

  it.each([
    ':heart::skin-tone-4:',
    ':wave::skin-tone-7:',
    ':wave::skin-tone-nope:',
    ':wave::skin-tone-4::skin-tone-5:'
  ])('keeps an unsupported compound literal: %s', (source) => {
    const matches = [...emojiShortcodes(source)];
    expect(matches).toHaveLength(1);
    expect(matches[0]?.resolution.kind).toBe('unknown');
    expect(source.slice(matches[0]?.start, matches[0]?.end)).toBe(source);
  });

  it.each(['a:wave::skin-tone-4:', ':wave::skin-tone-4:a', ':wave::skin-tone-4:中'])(
    'does not partially convert word-adjacent compounds: %s',
    (source) => {
      expect(
        [...emojiShortcodes(source)].every(({ resolution }) => resolution.kind === 'unknown')
      ).toBe(true);
    }
  );
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
