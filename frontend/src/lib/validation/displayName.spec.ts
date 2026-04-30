import { describe, it, expect } from 'vitest';
import {
  validateDisplayName,
  normalizeDisplayName,
  validateAndNormalizeDisplayName,
  MAX_DISPLAY_NAME_LENGTH
} from './displayName';

describe('validateDisplayName', () => {
  describe('valid names', () => {
    const validNames = [
      ['simple ASCII', 'John Doe'],
      ['single word', 'Alice'],
      ['with hyphen', 'Mary-Jane'],
      ['with apostrophe', "O'Brien"],
      ['with period', 'Dr. Smith'],
      ['with underscore', 'Cool_User'],
      ['with digits', 'Player123'],
      ['German umlaut', 'Müller'],
      ['French accents', 'François'],
      ['Spanish tilde', 'Señor García'],
      ['Russian Cyrillic', 'Иван Петров'],
      ['Japanese hiragana', 'たなか'],
      ['Japanese kanji', '田中太郎'],
      ['Chinese characters', '王小明'],
      ['Korean hangul', '김철수'],
      ['Arabic script', 'محمد علي'],
      ['Hebrew script', 'דוד כהן'],
      ['Greek letters', 'Αλέξανδρος'],
      ['Thai script', 'สมชาย'],
      ['Hindi Devanagari', 'राजेश कुमार'],
      ['emoji suffix', 'Alice 🚀'],
      ['mixed Latin-Japanese', 'John 田中'],
      ['mixed with emoji', 'Müller 🎵'],
      ['single char', 'A'],
      // Symbols allowed alongside emoji (when not the first character)
      ['with angle bracket (symbol)', 'John<3'],
      ['with equals (symbol)', 'a=b'],
      ['with pipe (symbol)', 'A|B'],
      ['with caret (symbol)', 'A^B'],
      ['with tilde (symbol)', 'u~ser'],
      ['with backtick (symbol)', 'co`de`'],
      ['with plus (symbol)', 'A+B']
    ] as const;

    for (const [description, name] of validNames) {
      it(`accepts ${description}: "${name}"`, () => {
        const result = validateDisplayName(name);
        expect(result.valid).toBe(true);
        expect(result.error).toBeUndefined();
      });
    }
  });

  describe('invalid names - control characters', () => {
    const invalidNames = [
      ['with newline', 'John\nDoe'],
      ['with tab', 'John\tDoe'],
      ['with carriage return', 'John\rDoe'],
      ['with null byte', 'John\x00Doe'],
      ['with bell', 'John\x07Doe']
    ] as const;

    for (const [description, name] of invalidNames) {
      it(`rejects ${description}`, () => {
        const result = validateDisplayName(name);
        expect(result.valid).toBe(false);
        expect(result.error).toContain('control characters');
      });
    }
  });

  describe('invalid names - zero-width characters', () => {
    const invalidNames = [
      ['with ZWSP', 'John\u200BDoe'],
      ['with ZWNJ', 'John\u200CDoe'],
      ['with ZWJ', 'John\u200DDoe'],
      ['with LTR mark', 'John\u200EDoe'],
      ['with RTL mark', 'John\u200FDoe'],
      ['with BOM', 'John\uFEFFDoe'],
      ['with word joiner', 'John\u2060Doe']
    ] as const;

    for (const [description, name] of invalidNames) {
      it(`rejects ${description}`, () => {
        const result = validateDisplayName(name);
        expect(result.valid).toBe(false);
        expect(result.error).toContain('invisible characters');
      });
    }
  });

  describe('invalid names - consecutive spaces', () => {
    it('rejects double space', () => {
      const result = validateDisplayName('John  Doe');
      expect(result.valid).toBe(false);
      expect(result.error).toContain('consecutive spaces');
    });

    it('rejects triple space', () => {
      const result = validateDisplayName('John   Doe');
      expect(result.valid).toBe(false);
      expect(result.error).toContain('consecutive spaces');
    });
  });

  describe('invalid names - must start with letter or digit', () => {
    const invalidStarts = [
      ['emoji prefix', '🎮 Gamer'],
      ['emoji only', '🦄'],
      ['flag emoji', '🇺🇸 American'],
      ['multiple emoji', '🌟 Star ⭐'],
      ['single emoji', '😀'],
      ['starts with hyphen', '-Alice'],
      ['starts with apostrophe', "'Alice"],
      ['starts with period', '.Alice'],
      ['starts with underscore', '_Alice'],
      ['starts with tilde', '~user'],
      ['starts with backtick', '`code`'],
      ['starts with plus', '+Alice'],
      ['starts with equals', '=Alice'],
      ['starts with caret', '^Alice'],
      ['starts with pipe', '|Alice'],
      ['starts with angle bracket', '<Alice']
    ] as const;

    for (const [description, name] of invalidStarts) {
      it(`rejects ${description}: "${name}"`, () => {
        const result = validateDisplayName(name);
        expect(result.valid).toBe(false);
        expect(result.error).toContain('must start with a letter or digit');
      });
    }
  });

  describe('invalid names - disallowed punctuation', () => {
    const invalidNames = [
      ['with curly brace', 'John{test}'],
      ['with semicolon', 'John; DROP TABLE'],
      ['with at sign', 'user@domain'],
      ['with hash', 'Pre#hashtag'],
      ['with exclamation', 'Hello!'],
      ['with question mark', 'Who?'],
      ['with comma', 'Last, First'],
      ['with colon', 'Title: Name'],
      ['with slash', 'A/B'],
      ['with backslash', 'A\\B'],
      ['with quotes', 'Pre"quoted"'],
      ['with parentheses', 'Pre(name)'],
      ['with square brackets', 'Pre[name]'],
      ['with ampersand', 'A&B'],
      ['with asterisk', 'star*'],
      ['with percent', '100%']
    ] as const;

    for (const [description, name] of invalidNames) {
      it(`rejects ${description}`, () => {
        const result = validateDisplayName(name);
        expect(result.valid).toBe(false);
        expect(result.error).toContain('can only contain');
      });
    }
  });

  describe('edge cases', () => {
    it('rejects empty string', () => {
      const result = validateDisplayName('');
      expect(result.valid).toBe(false);
      expect(result.error).toContain('cannot be empty');
    });

    it('rejects names exceeding character limit', () => {
      const longName = 'a'.repeat(33);
      const result = validateDisplayName(longName);
      expect(result.valid).toBe(false);
      expect(result.error).toContain('cannot exceed');
    });

    it('accepts names at exactly character limit', () => {
      const maxName = 'a'.repeat(MAX_DISPLAY_NAME_LENGTH);
      const result = validateDisplayName(maxName);
      expect(result.valid).toBe(true);
    });

    it('counts characters, not bytes', () => {
      // 30 Japanese characters (each 3 bytes in UTF-8) should be valid
      const japaneseName = '田'.repeat(30);
      const result = validateDisplayName(japaneseName);
      expect(result.valid).toBe(true);

      // 33 Japanese characters should be invalid (exceeds 32 char limit)
      const tooLongJapanese = '田'.repeat(33);
      const result2 = validateDisplayName(tooLongJapanese);
      expect(result2.valid).toBe(false);
    });
  });
});

describe('normalizeDisplayName', () => {
  it('trims leading whitespace', () => {
    expect(normalizeDisplayName(' Alice')).toBe('Alice');
  });

  it('trims trailing whitespace', () => {
    expect(normalizeDisplayName('Alice ')).toBe('Alice');
  });

  it('trims both ends', () => {
    expect(normalizeDisplayName('  Alice  ')).toBe('Alice');
  });

  it('preserves internal spaces', () => {
    expect(normalizeDisplayName('John Doe')).toBe('John Doe');
  });

  it('returns empty string for whitespace-only input', () => {
    expect(normalizeDisplayName('   ')).toBe('');
  });
});

describe('validateAndNormalizeDisplayName', () => {
  it('normalizes and validates valid name', () => {
    const result = validateAndNormalizeDisplayName('  Alice  ');
    expect(result.valid).toBe(true);
    expect(result.normalized).toBe('Alice');
  });

  it('normalizes and rejects invalid name', () => {
    const result = validateAndNormalizeDisplayName('  John\nDoe  ');
    expect(result.valid).toBe(false);
    expect(result.error).toBeDefined();
    expect(result.normalized).toBeUndefined();
  });

  it('rejects whitespace-only after normalization', () => {
    const result = validateAndNormalizeDisplayName('   ');
    expect(result.valid).toBe(false);
    expect(result.error).toContain('cannot be empty');
  });
});
