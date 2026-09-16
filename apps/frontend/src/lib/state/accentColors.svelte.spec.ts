import '../../app.css';
import { afterEach, expect, it } from 'vitest';
import { accentColors, surfaceDepths } from './userPreferences.svelte';

afterEach(() => {
  delete document.documentElement.dataset.accent;
  delete document.documentElement.dataset.theme;
  delete document.documentElement.dataset.depth;
  document.documentElement.style.removeProperty('transition');
});

/** Resolve browser-supported colours to sRGB, including gamut clipping. */
function rgba(color: string): number[] {
  const canvas = document.createElement('canvas');
  canvas.width = canvas.height = 1;
  const context = canvas.getContext('2d')!;
  context.fillStyle = color;
  context.fillRect(0, 0, 1, 1);
  return [...context.getImageData(0, 0, 1, 1).data].map((v) => v / 255);
}

function rgb(color: string): number[] {
  return rgba(color).slice(0, 3);
}

function luminance(color: number[]): number {
  const linear = color.map((v) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
  return linear[0] * 0.2126 + linear[1] * 0.7152 + linear[2] * 0.0722;
}

function contrast(a: number[], b: number[]): number {
  const [low, high] = [luminance(a), luminance(b)].sort((x, y) => x - y);
  return (high + 0.05) / (low + 0.05);
}

it.each(accentColors)(
  '%s preserves readable accent text and white button labels in both themes',
  (accent) => {
    const root = document.documentElement;
    root.dataset.accent = accent;
    root.style.transition = 'none';
    for (const theme of ['light', 'dark']) {
      root.dataset.theme = theme;
      const style = getComputedStyle(root);
      const color = (name: string) => rgb(style.getPropertyValue(`--color-${name}`).trim());
      for (const surface of ['background', 'surface']) {
        expect(
          contrast(color('action'), color(surface)),
          `${accent}/${theme} on ${surface}`
        ).toBeGreaterThanOrEqual(4.5);
      }
      expect(contrast(color('action'), color('on-action'))).toBeGreaterThanOrEqual(4.5);
      expect(color('on-button-action')).toEqual([1, 1, 1]);
      // Read the real gloss from CSS so stronger modes cannot bypass this check.
      const probe = document.createElement('span');
      probe.className = 'btn-action';
      probe.style.color = 'var(--lighting-top)';
      document.body.append(probe);
      try {
        for (const depth of surfaceDepths) {
          root.dataset.depth = depth;
          const highlight = rgba(getComputedStyle(probe).color);
          for (const fill of ['button-action', 'button-action-hover']) {
            const glossy = color(fill).map((channel, index) =>
              channel * (1 - highlight[3]) + highlight[index] * highlight[3]
            );
            expect(
              contrast(glossy, color('on-button-action')),
              `${accent}/${theme}/${depth} ${fill} gloss`
            ).toBeGreaterThanOrEqual(4.5);
          }
        }
      } finally {
        probe.remove();
      }
    }
  }
);
