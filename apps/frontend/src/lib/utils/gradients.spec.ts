import { describe, it, expect } from 'vitest';
import { getGradientForName } from './gradients';

describe('getGradientForName', () => {
  it('returns a CSS linear-gradient string', () => {
    const gradient = getGradientForName('Engineering');
    expect(gradient).toMatch(
      /^linear-gradient\((135|150|165)deg,\s*#[0-9a-f]{6},\s*#[0-9a-f]{6}\)$/i
    );
  });

  it('is deterministic: same input → same gradient', () => {
    expect(getGradientForName('My Space')).toBe(getGradientForName('My Space'));
  });

  it('produces different gradients for sufficiently different inputs', () => {
    const a = getGradientForName('Alpha');
    const b = getGradientForName('Bravo');
    const c = getGradientForName('Zulu');
    // Not strictly guaranteed for all triples, but these three differ in practice
    const distinct = new Set([a, b, c]).size;
    expect(distinct).toBeGreaterThan(1);
  });

  it('handles empty string without throwing', () => {
    expect(() => getGradientForName('')).not.toThrow();
    expect(getGradientForName('')).toMatch(/^linear-gradient\(/);
  });

  it('handles unicode and emoji', () => {
    expect(getGradientForName('🚀 launch')).toMatch(/^linear-gradient\(/);
  });

  it('uses one of the three configured angles', () => {
    const angle = getGradientForName('test').match(/linear-gradient\((\d+)deg/)?.[1];
    expect(['135', '150', '165']).toContain(angle);
  });
});
