import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const appHtml = readFileSync(new URL('./app.html', import.meta.url), 'utf8');

function metaContent(name: string, mediaFragment: string): string | null {
  const tag = appHtml.match(
    new RegExp(
      `<meta\\s+[^>]*name="${name}"[^>]*media="[^"]*${mediaFragment}[^"]*"[^>]*>`,
      'i'
    )
  )?.[0];

  return tag?.match(/\bcontent="([^"]+)"/i)?.[1] ?? null;
}

describe('app.html metadata', () => {
  it('defines theme colors matching the outer frame background colors', () => {
    expect(metaContent('theme-color', 'light')).toBe('#e5e7eb');
    expect(metaContent('theme-color', 'dark')).toBe('#262626');
  });
});
