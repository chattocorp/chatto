import { describe, it, expect } from 'vitest';
import {
  validateLogin,
  normalizeLogin,
  validateAndNormalizeLogin,
  getLoginChangeCooldownRemaining,
  formatCooldownRemaining,
  MAX_LOGIN_LENGTH,
  MIN_LOGIN_LENGTH,
  LOGIN_CHANGE_COOLDOWN_MS
} from './login';

describe('validateLogin', () => {
  describe('valid logins', () => {
    const validLogins: [string, string][] = [
      ['simple lowercase', 'alice'],
      ['with digits', 'alice123'],
      ['with period', 'alice.bob'],
      ['with underscore', 'alice_bob'],
      ['with hyphen', 'alice-bob'],
      ['starts with digit', '1alice'],
      ['min length', 'ab'],
      ['max length', 'a'.repeat(MAX_LOGIN_LENGTH)]
    ];

    it.each(validLogins)('%s: %s', (_, login) => {
      expect(validateLogin(login).valid).toBe(true);
    });
  });

  describe('invalid logins', () => {
    it('rejects empty string', () => {
      const result = validateLogin('');
      expect(result.valid).toBe(false);
      expect(result.error).toContain('empty');
    });

    it('rejects too short', () => {
      const result = validateLogin('a');
      expect(result.valid).toBe(false);
      expect(result.error).toContain(`${MIN_LOGIN_LENGTH}`);
    });

    it('rejects too long', () => {
      const result = validateLogin('a'.repeat(MAX_LOGIN_LENGTH + 1));
      expect(result.valid).toBe(false);
      expect(result.error).toContain(`${MAX_LOGIN_LENGTH}`);
    });

    it('rejects starting with period', () => {
      const result = validateLogin('.alice');
      expect(result.valid).toBe(false);
      expect(result.error).toContain('start with');
    });

    it('rejects starting with underscore', () => {
      const result = validateLogin('_alice');
      expect(result.valid).toBe(false);
      expect(result.error).toContain('start with');
    });

    it('rejects starting with hyphen', () => {
      const result = validateLogin('-alice');
      expect(result.valid).toBe(false);
      expect(result.error).toContain('start with');
    });

    it('rejects spaces', () => {
      const result = validateLogin('alice bob');
      expect(result.valid).toBe(false);
    });

    it('rejects special characters', () => {
      const result = validateLogin('alice@bob');
      expect(result.valid).toBe(false);
    });

    it('rejects emoji', () => {
      const result = validateLogin('alice😀');
      expect(result.valid).toBe(false);
    });
  });
});

describe('normalizeLogin', () => {
  it('preserves casing', () => {
    expect(normalizeLogin('Alice')).toBe('Alice');
  });

  it('trims whitespace', () => {
    expect(normalizeLogin('  alice  ')).toBe('alice');
  });

  it('trims but preserves casing', () => {
    expect(normalizeLogin('  Alice  ')).toBe('Alice');
  });
});

describe('validateAndNormalizeLogin', () => {
  it('returns normalized value on success preserving case', () => {
    const result = validateAndNormalizeLogin('Alice');
    expect(result.valid).toBe(true);
    expect(result.normalized).toBe('Alice');
  });

  it('returns error on failure', () => {
    const result = validateAndNormalizeLogin('');
    expect(result.valid).toBe(false);
    expect(result.normalized).toBeUndefined();
  });
});

describe('getLoginChangeCooldownRemaining', () => {
  it('returns 0 when no last change date', () => {
    expect(getLoginChangeCooldownRemaining(null)).toBe(0);
  });

  it('returns 0 when cooldown has expired', () => {
    const thirtyOneDaysAgo = new Date(Date.now() - 31 * 24 * 60 * 60 * 1000);
    expect(getLoginChangeCooldownRemaining(thirtyOneDaysAgo)).toBe(0);
  });

  it('returns remaining time when cooldown is active', () => {
    const fiveDaysAgo = new Date(Date.now() - 5 * 24 * 60 * 60 * 1000);
    const remaining = getLoginChangeCooldownRemaining(fiveDaysAgo);
    // Should be approximately 25 days in ms
    expect(remaining).toBeGreaterThan(24 * 24 * 60 * 60 * 1000);
    expect(remaining).toBeLessThan(LOGIN_CHANGE_COOLDOWN_MS);
  });

  it('returns full cooldown when just changed', () => {
    const justNow = new Date();
    const remaining = getLoginChangeCooldownRemaining(justNow);
    // Should be close to full 30 days
    expect(remaining).toBeGreaterThan(LOGIN_CHANGE_COOLDOWN_MS - 1000);
  });
});

describe('formatCooldownRemaining', () => {
  it('returns empty string for zero', () => {
    expect(formatCooldownRemaining(0)).toBe('');
  });

  it('formats days', () => {
    expect(formatCooldownRemaining(5 * 24 * 60 * 60 * 1000)).toBe('5 days');
  });

  it('formats hours', () => {
    expect(formatCooldownRemaining(3 * 60 * 60 * 1000)).toBe('3 hours');
  });

  it('formats minutes', () => {
    expect(formatCooldownRemaining(15 * 60 * 1000)).toBe('15 minutes');
  });

  it('formats single minute', () => {
    expect(formatCooldownRemaining(60 * 1000)).toBe('1 minute');
  });
});
