import '../../app.css';
import { afterEach, expect, it } from 'vitest';
import { accentColors, applyContrastAge, surfaceDepths } from './userPreferences.svelte';

afterEach(() => {
  delete document.documentElement.dataset.accent;
  delete document.documentElement.dataset.theme;
  delete document.documentElement.dataset.depth;
  document.documentElement.style.removeProperty('--contrast-soft-mix');
  document.documentElement.style.removeProperty('--contrast-strong-mix');
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

it.each(['light', 'dark'])('%s at the middle keeps the original semantic colours', (theme) => {
  const root = document.documentElement;
  root.dataset.theme = theme;
  applyContrastAge(30);
  const style = getComputedStyle(root);
  for (const name of [
    'background',
    'text',
    'text-top',
    'muted',
    'surface',
    'surface-emphasized',
    'surface-strong',
    'border',
    'input-border'
  ]) {
    expect(rgb(style.getPropertyValue(`--color-${name}`).trim()), name).toEqual(
      rgb(style.getPropertyValue(`--contrast-base-${name}`).trim())
    );
  }
  expect(rgb(style.getPropertyValue('--color-surface-selected').trim())).toEqual(
    rgb(style.getPropertyValue('--contrast-base-surface-strong').trim())
  );
});

it.each(['light', 'dark'])('%s gives the app frame and panel inset a strong edge at 100%', (theme) => {
  const root = document.documentElement;
  root.dataset.theme = theme;
  const frame = document.createElement('div');
  frame.className = 'app-frame-inset';
  const inset = document.createElement('div');
  inset.className = 'panel-inset';
  document.body.append(frame, inset);
  try {
    applyContrastAge(30);
    expect(rgba(getComputedStyle(frame, '::before').borderColor)[3]).toBe(0);
    expect(rgba(getComputedStyle(inset).outlineColor)[3]).toBe(0);

    applyContrastAge(40);
    const background = rgb(getComputedStyle(root).getPropertyValue('--color-background').trim());
    const frameEdge = getComputedStyle(frame, '::before');
    expect(frameEdge.borderStyle).toBe('solid');
    expect(frameEdge.borderWidth).toBe('1px');
    expect(Number(frameEdge.zIndex)).toBeGreaterThan(50);
    expect(frameEdge.pointerEvents).toBe('none');
    expect(rgba(frameEdge.borderColor)[3]).toBe(1);
    expect(contrast(rgb(frameEdge.borderColor), background)).toBeGreaterThanOrEqual(15);
    const insetEdge = getComputedStyle(inset);
    expect(insetEdge.outlineStyle).toBe('solid');
    expect(insetEdge.outlineWidth).toBe('1px');
    expect(rgba(insetEdge.outlineColor)[3]).toBe(1);
    expect(contrast(rgb(insetEdge.outlineColor), background)).toBeGreaterThanOrEqual(15);
  } finally {
    frame.remove();
    inset.remove();
  }
});

it.each(accentColors)('%s stays readable across contrast ages and themes', (accent) => {
  const root = document.documentElement;
  root.dataset.accent = accent;
  for (const theme of ['light', 'dark']) {
    root.dataset.theme = theme;
    for (const age of [20, 25, 30, 35, 40]) {
      applyContrastAge(age);
      const style = getComputedStyle(root);
      const color = (name: string) => rgb(style.getPropertyValue(`--color-${name}`).trim());
      for (const surface of ['background', 'surface']) {
        expect(
          contrast(color('action'), color(surface)),
          `${accent}/${theme}/${age} action on ${surface}`
        ).toBeGreaterThanOrEqual(4.5);
        for (const foreground of ['text', 'muted']) {
          const minimum =
            age === 20 ? (foreground === 'text' ? 3 : 2) :
            age === 25 ? (foreground === 'text' ? 4 : 3) : 4.5;
          expect(
            contrast(color(foreground), color(surface)),
            `${accent}/${theme}/${age} ${foreground} on ${surface}`
          ).toBeGreaterThanOrEqual(minimum);
        }
      }
      expect(
        contrast(color('text'), color('surface-emphasized')),
        `${accent}/${theme}/${age} text on emphasized surface`
      ).toBeGreaterThanOrEqual(age === 20 ? 2.5 : age === 25 ? 3.5 : 4.5);
      expect(contrast(color('on-action'), color('action'))).toBeGreaterThanOrEqual(4.5);
      if (age === 20) {
        expect(contrast(color('text'), color('background'))).toBeLessThan(4);
        expect(contrast(color('muted'), color('background'))).toBeLessThan(3);
        expect(contrast(color('text-top'), color('background'))).toBeLessThan(5);
      }
      if (age === 40) {
        const highContrastText = theme === 'light' ? [0, 0, 0] : [1, 1, 1];
        expect(color('text')).toEqual(highContrastText);
        expect(color('text-top')).toEqual(highContrastText);
        expect(color('muted')).toEqual(highContrastText);
        expect(color('background')).toEqual(theme === 'light' ? [1, 1, 1] : [0, 0, 0]);
        expect(color('surface')).toEqual(theme === 'light' ? [1, 1, 1] : [0, 0, 0]);
        expect(contrast(color('border'), color('surface'))).toBeGreaterThanOrEqual(15);
        expect(contrast(color('input-border'), color('surface'))).toBeGreaterThanOrEqual(15);
      }
    }
  }
});
