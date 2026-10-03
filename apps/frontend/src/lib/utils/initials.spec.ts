import { describe, it, expect } from 'vitest';
import { getAvatarColour, getAvatarLabel } from './initials';

describe('getAvatarLabel', () => {
  it('skips invisible letter fillers when finding an initial', () => {
    expect(getAvatarLabel('\u3164Alice', 'login')).toEqual({ kind: 'text', text: 'A' });
    expect(getAvatarLabel('\u115F', 'alice')).toEqual({ kind: 'text', text: 'A' });
  });
  it('uses up to two initials and skips punctuation or emoji words', () => {
    expect(getAvatarLabel('John Robert Doe', 'login')).toEqual({ kind: 'text', text: 'JR' });
    expect(getAvatarLabel('[DEV] ChattoBot', 'login')).toEqual({ kind: 'text', text: 'DC' });
    expect(getAvatarLabel('ChattoBot [DEV]', 'login')).toEqual({ kind: 'text', text: 'CD' });
    expect(getAvatarLabel('🕹️ !!! Alice Smith', 'login')).toEqual({ kind: 'text', text: 'AS' });
    expect(getAvatarLabel('  alice   smith  ', 'login')).toEqual({ kind: 'text', text: 'AS' });
  });

  it('preserves graphemes in combining scripts and supplementary letters', () => {
    expect(getAvatarLabel('e\u0301mile 王小明', 'login')).toEqual({
      kind: 'text',
      text: 'E\u0301王'
    });
    expect(getAvatarLabel('𐐨', 'login')).toEqual({ kind: 'text', text: '𐐀' });
    expect(getAvatarLabel('क्\u200Dष', 'login')).toEqual({ kind: 'text', text: 'क्\u200Dष' });
  });

  it('uses the first complete emoji when there are no letters or numbers', () => {
    for (const emoji of [
      '🕹️',
      '👩‍💻',
      '👨‍👩‍👧‍👦',
      '🇺🇸',
      '#️⃣',
      '🏴\u{E0067}\u{E0062}\u{E0065}\u{E006E}\u{E0067}\u{E007F}'
    ]) {
      expect(getAvatarLabel('[' + emoji + '] 🌟', 'login')).toEqual({ kind: 'emoji', text: emoji });
    }
  });

  it('keeps numeric keycap graphemes intact as initials', () => {
    expect(getAvatarLabel('1️⃣', 'login')).toEqual({ kind: 'text', text: '1️⃣' });
  });

  it('falls back to the login for absent or punctuation-only names', () => {
    for (const name of [null, undefined, '', '   ', '!!!']) {
      expect(getAvatarLabel(name, 'alice')).toEqual({ kind: 'text', text: 'A' });
    }
    expect(getAvatarLabel('!!!', 'e\u0301mile')).toEqual({ kind: 'text', text: 'E\u0301' });
  });

  it('requests an icon when neither name nor login supplies a label', () => {
    expect(getAvatarLabel(null, null)).toEqual({ kind: 'icon' });
    expect(getAvatarLabel(undefined, undefined)).toEqual({ kind: 'icon' });
    expect(getAvatarLabel('!!!', '   ')).toEqual({ kind: 'icon' });
  });
});

describe('getAvatarColour', () => {
  it('keeps the palette mapping stable for immutable IDs', () => {
    // Fixed FNV-1a fixtures protect the palette order and hash algorithm.
    expect(getAvatarColour('a')).toBe('amber');
    expect(getAvatarColour('b')).toBe('orange');
    expect(getAvatarColour('user-1')).toBe(getAvatarColour('user-1'));
  });

  it('assigns all eight colours across account IDs', () => {
    const colours = new Set(Array.from({ length: 100 }, (_, i) => getAvatarColour('user-' + i)));
    expect(colours.size).toBe(8);
  });
});
