import { expect, it } from 'vitest';
import { paintVoiceFog } from './voiceFog';

function mask(time: number, seed = 42) {
  const pixels = new Uint8ClampedArray(96 * 32 * 4);
  paintVoiceFog(pixels, 96, 32, time, seed);
  return pixels.filter((_, i) => i % 4 === 3);
}

it('paints repeatable, uneven fog with transparent edges', () => {
  const alpha = mask(0);
  expect(alpha).toEqual(mask(0));
  expect(new Set(alpha).size).toBeGreaterThan(50);
  expect(Math.max(...alpha)).toBeGreaterThan(100);
  expect(alpha.slice(0, 96).every((value) => value === 0)).toBe(true);
  expect(alpha.slice(-96).every((value) => value === 0)).toBe(true);
  expect(alpha).not.toEqual(mask(0, 43));
});

it('changes continuously as the fog flows', () => {
  const initial = mask(0);
  const next = mask(0.016);
  const later = mask(2);
  const difference = (other: Uint8ClampedArray) =>
    initial.reduce((sum, value, i) => sum + Math.abs(value - other[i]), 0) / initial.length;
  expect(difference(next)).toBeGreaterThan(0);
  expect(difference(next)).toBeLessThan(2);
  expect(difference(later)).toBeGreaterThan(difference(next) * 10);
});
