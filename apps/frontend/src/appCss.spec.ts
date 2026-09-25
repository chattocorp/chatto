import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { describe, expect, it } from 'vitest';

const appCss = readFileSync(new URL('./app.css', import.meta.url), 'utf8');
const tailwindTheme = readFileSync(
  createRequire(import.meta.url).resolve('tailwindcss/theme.css'),
  'utf8'
);

function neutralPalette(css: string): Map<string, string> {
  return new Map(
    [...css.matchAll(/--color-(neutral-\d+):\s*([^;]+);/g)].map(([, name, value]) => [
      name,
      value.trim()
    ])
  );
}

describe('app.css neutral palette', () => {
  const upstream = neutralPalette(tailwindTheme);
  const local = neutralPalette(appCss);

  it('overrides every Tailwind neutral', () => {
    expect([...local.keys()]).toEqual([...upstream.keys()]);
  });

  // Chrome before 138 turns a `none` hue into the blue channel of
  // `color-mix(in srgb, …)`, which tints the dark theme.
  it('matches Tailwind with a zero hue instead of `none`', () => {
    for (const [name, value] of upstream) {
      expect(local.get(name), name).toBe(value.replace(/\bnone\)$/, '0)'));
    }
  });
});
