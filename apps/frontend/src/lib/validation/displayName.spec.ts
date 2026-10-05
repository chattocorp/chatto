import { describe, it, expect } from 'vitest';
import {
  validateDisplayName,
  normalizeDisplayName,
  validateAndNormalizeDisplayName,
  MAX_DISPLAY_NAME_LENGTH
} from './displayName';

describe('validateDisplayName', () => {
  const validNames = [
    'John Doe',
    'Mary-Jane',
    "O'Brien",
    'Dr. Smith',
    'Cool_User',
    'Player123',
    'Müller',
    'François',
    'Иван Петров',
    'たなか',
    '王小明',
    '김철수',
    'محمد علي',
    'דוד כהן',
    'Αλέξανδρος',
    'สมชาย',
    'राजेश कुमार',
    'John 田中',
    'ChattoBot [DEV]',
    '[DEV] ChattoBot',
    '(away) Alice',
    '「田中」',
    '{Bot}',
    'John; DROP TABLE',
    'user@domain',
    'Hello!',
    'Last, First',
    'A/B',
    'A\\B',
    'Pre"quoted"',
    'A&B',
    'star*',
    '100%',
    'John<3',
    'A+B',
    'co`de`',
    '!!!',
    '-Alice',
    '_Alice',
    '🎮 Gamer',
    '🦄',
    '🇺🇸',
    '🕹️',
    '👩‍💻',
    '👨‍👩‍👧‍👦',
    '1️⃣',
    '#️⃣',
    '🏴\u{E0067}\u{E0062}\u{E0065}\u{E006E}\u{E0067}\u{E007F}',
    'John  Doe',
    'John   Doe',
    'A\u00A0B',
    'e\u0301',
    '\u0301Alice',
    'می\u200Cروم',
    'क्\u200Dष'
  ];
  for (const name of validNames) {
    it('accepts ' + JSON.stringify(name), () => {
      expect(validateDisplayName(name)).toEqual({ valid: true });
    });
  }

  const invalidNames = [
    '',
    'John\nDoe',
    'John\tDoe',
    'John\rDoe',
    'John\x00Doe',
    'John\x07Doe',
    'John\u0085Doe',
    'John\u2028Doe',
    'John\u2029Doe',
    'John\u200BDoe',
    'John\u200EDoe',
    'John\u200FDoe',
    'John\uFEFFDoe',
    'John\u2060Doe',
    'John\u00ADDoe',
    'John\u061CDoe',
    'John\u202EDoe',
    'John\u2066Doe',
    'John\u2069Doe',
    ' ',
    '  ',
    '\u0301',
    '\u200C\u200D',
    '\uFE0F',
    '\u{E0067}\u{E007F}',
    '\u115F',
    '\u1160',
    '\u3164',
    '\uFFA0',
    '\u2800'
  ];
  for (const name of invalidNames) {
    it('rejects ' + JSON.stringify(name), () => {
      expect(validateDisplayName(name).valid).toBe(false);
    });
  }

  it('counts Unicode code points, including emoji and variation selectors', () => {
    expect(validateDisplayName('田'.repeat(MAX_DISPLAY_NAME_LENGTH)).valid).toBe(true);
    expect(validateDisplayName('田'.repeat(MAX_DISPLAY_NAME_LENGTH + 1)).valid).toBe(false);
    expect(validateDisplayName('🕹️'.repeat(16)).valid).toBe(true);
    expect(validateDisplayName('🕹️'.repeat(17)).valid).toBe(false);
  });
});

describe('normalizeDisplayName', () => {
  it('matches server whitespace trimming without hiding a BOM', () => {
    expect(normalizeDisplayName('\u0085 Alice  Smith \u0085')).toBe('Alice  Smith');
    expect(validateAndNormalizeDisplayName('\uFEFFAlice').valid).toBe(false);
  });
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
