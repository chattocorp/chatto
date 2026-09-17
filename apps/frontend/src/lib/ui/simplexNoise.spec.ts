import { expect, it } from 'vitest';
import { simplexNoise2D } from './simplexNoise';

it('stays bounded and repeatable across independent motion streams', () => {
  for (let step = -1000; step <= 1000; step++) {
    const position = step / 37;
    const value = simplexNoise2D(position, position * 0.7, 42);
    expect(value).toBeGreaterThanOrEqual(-1);
    expect(value).toBeLessThanOrEqual(1);
    expect(simplexNoise2D(position, position * 0.7, 42)).toBe(value);
  }
  const path = (seed: number) =>
    Array.from({ length: 20 }, (_, i) => simplexNoise2D(i / 3, i / 7, seed));
  expect(path(42)).not.toEqual(path(43));
  expect(new Set(path(42)).size).toBe(20);
});

it('moves smoothly across lattice joins without position or velocity jumps', () => {
  const epsilon = 0.000001;
  // Along x = y, skewed lattice boundaries occur at multiples of 1 / sqrt(3).
  for (const position of [-2, -1, 0, 1, 2].map((n) => n / Math.sqrt(3))) {
    const before = simplexNoise2D(position - epsilon, position - epsilon, 42);
    const center = simplexNoise2D(position, position, 42);
    const after = simplexNoise2D(position + epsilon, position + epsilon, 42);
    expect(Math.abs(after - before)).toBeLessThan(0.001);
    expect(Math.abs((after - center) / epsilon - (center - before) / epsilon)).toBeLessThan(0.01);
  }
});
