import { describe, expect, it } from 'vitest';
import { profileBandColor, profileBandHue } from './profileBand';

describe('profileBandHue', () => {
  it('returns the same hue for the same user', () => {
    expect(profileBandHue('U123')).toBe(profileBandHue('U123'));
  });

  it('stays within the hue circle', () => {
    for (const id of ['', 'a', 'U1', 'U2', '01JXYZ', '😀']) {
      const hue = profileBandHue(id);
      expect(hue).toBeGreaterThanOrEqual(0);
      expect(hue).toBeLessThan(360);
      expect(Number.isInteger(hue)).toBe(true);
    }
  });

  it('spreads similar IDs across different hues', () => {
    const hues = new Set(['U1', 'U2', 'U3', 'U4', 'U5', 'U6'].map(profileBandHue));
    expect(hues.size).toBeGreaterThan(4);
  });
});

describe('profileBandColor', () => {
  it('uses the theme band tokens', () => {
    expect(profileBandColor('U1')).toBe(
      `oklch(var(--profile-band-lightness) var(--profile-band-chroma) ${profileBandHue('U1')})`
    );
  });
});
