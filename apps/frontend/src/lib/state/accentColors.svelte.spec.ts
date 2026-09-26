import '../../app.css';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import {
  accentColors,
  applyContrastAge,
  applySurfaceTones,
  getLoadingPalettes,
  hexFromComputedColor,
  surfaceTones
} from './userPreferences.svelte';

// Read settled palette values; the fade itself is covered separately.
beforeEach(() => {
  document.documentElement.style.transition = 'none';
});

afterEach(() => {
  delete document.documentElement.dataset.accent;
  delete document.documentElement.dataset.theme;
  document.documentElement.style.removeProperty('--depth-level');
  delete document.documentElement.dataset.lightTone;
  delete document.documentElement.dataset.darkTone;
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
        for (const depth of [0, 50, 100]) {
          root.style.setProperty('--depth-level', String(depth));
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

it.each(['light', 'dark'] as const)(
  '%s default tone reproduces the original neutral palette',
  (theme) => {
    const root = document.documentElement;
    root.dataset.theme = theme;
    applyContrastAge(30);
    const style = getComputedStyle(root);
    const surface = () => rgb(style.getPropertyValue('--color-surface').trim());
    const text = () => rgb(style.getPropertyValue('--color-text').trim());
    const original =
      theme === 'light'
        ? { surface: 'var(--color-gray-200)', text: 'var(--color-gray-600)' }
        : { surface: 'var(--color-neutral-800)', text: 'var(--color-neutral-300)' };
    const probe = document.createElement('span');
    document.body.append(probe);
    try {
      probe.style.color = original.surface;
      const originalSurface = rgb(getComputedStyle(probe).color);
      probe.style.color = original.text;
      const originalText = rgb(getComputedStyle(probe).color);
      expect(surface()).toEqual(originalSurface);
      expect(text()).toEqual(originalText);
      root.dataset.lightTone = 'gray';
      root.dataset.darkTone = 'neutral';
      expect(surface()).toEqual(originalSurface);
      expect(text()).toEqual(originalText);
      // Only the tone of the active theme changes the palette.
      root.dataset[theme === 'light' ? 'darkTone' : 'lightTone'] = 'midnight';
      expect(surface()).toEqual(originalSurface);
      root.dataset[theme === 'light' ? 'lightTone' : 'darkTone'] = 'midnight';
      expect(surface()).not.toEqual(originalSurface);
    } finally {
      probe.remove();
    }
  }
);

it.each(surfaceTones)('%s tone keeps text and every accent readable', (tone) => {
  const root = document.documentElement;
  root.dataset.lightTone = tone;
  root.dataset.darkTone = tone;
  for (const theme of ['light', 'dark']) {
    root.dataset.theme = theme;
    for (const age of [20, 30, 40]) {
      applyContrastAge(age);
      for (const accent of accentColors) {
        root.dataset.accent = accent;
        const style = getComputedStyle(root);
        const color = (name: string) => rgb(style.getPropertyValue(`--color-${name}`).trim());
        for (const surface of ['background', 'surface']) {
          expect(
            contrast(color('action'), color(surface)),
            `${tone}/${theme}/${age}/${accent} action on ${surface}`
          ).toBeGreaterThanOrEqual(4.5);
        }
      }
      const style = getComputedStyle(root);
      const color = (name: string) => rgb(style.getPropertyValue(`--color-${name}`).trim());
      for (const surface of ['background', 'surface']) {
        for (const foreground of ['text', 'muted']) {
          const minimum = age === 20 ? (foreground === 'text' ? 3 : 2) : 4.5;
          expect(
            contrast(color(foreground), color(surface)),
            `${tone}/${theme}/${age} ${foreground} on ${surface}`
          ).toBeGreaterThanOrEqual(minimum);
        }
      }
      expect(
        contrast(color('text'), color('surface-emphasized')),
        `${tone}/${theme}/${age} text on emphasized surface`
      ).toBeGreaterThanOrEqual(age === 20 ? 2.5 : 4.5);
    }
  }
});

it('fades palette colours instead of switching them instantly', async () => {
  const root = document.documentElement;
  root.style.removeProperty('transition');
  root.dataset.theme = 'light';
  const surface = () => getComputedStyle(root).getPropertyValue('--color-surface').trim();
  const before = rgb(surface());
  root.dataset.lightTone = 'forest';
  // Registered colours start at the old value and settle on the new tone.
  expect(rgb(surface())).toEqual(before);
  const running = root
    .getAnimations()
    .find((animation) => (animation as CSSTransition).transitionProperty === '--color-surface');
  expect(running).toBeDefined();
  await running!.finished;
  expect(rgb(surface())).not.toEqual(before);
});

it('saves startup colours that match app.html for the default tones', () => {
  applySurfaceTones({ light: 'gray', dark: 'neutral' });
  // These literals are the #app-loading and theme-color fallbacks in app.html.
  expect(getLoadingPalettes()).toEqual({
    light: { background: '#f3f4f6', highlight: '#99a1af', text: '#4a5565', surface: '#e5e7eb' },
    dark: { background: '#171717', highlight: '#404040', text: '#d4d4d4', surface: '#262626' },
    tones: { light: 'gray', dark: 'neutral' }
  });

  applySurfaceTones({ light: 'forest', dark: 'plum' });
  const palettes = getLoadingPalettes();
  expect(palettes?.light.background).not.toBe('#f3f4f6');
  expect(palettes?.dark.background).not.toBe('#171717');
});

it.each([
  [0, 0, 1],
  [30, 0.45, 1],
  [50, 0.75, 1],
  [80, 1.35, 1.3],
  [100, 1.75, 1.5]
])('depth level %s gives strength %s and width %s', (level, strength, width) => {
  const root = document.documentElement;
  // 0, 50, and 100 match the former Flat, Kinda 3D, and Very 3D modes.
  root.style.setProperty('--depth-level', String(level));
  const style = getComputedStyle(root);
  expect(Number(style.getPropertyValue('--depth-strength'))).toBeCloseTo(strength);
  expect(Number(style.getPropertyValue('--depth-width'))).toBeCloseTo(width);
});

it('resolves startup colours without reading back a canvas', () => {
  // Fingerprinting protection can block or randomise canvas readback.
  const getContext = vi.spyOn(HTMLCanvasElement.prototype, 'getContext');
  try {
    applySurfaceTones({ light: 'forest', dark: 'plum' });
    expect(getContext).not.toHaveBeenCalled();
    expect(getLoadingPalettes()?.tones).toEqual({ light: 'forest', dark: 'plum' });
  } finally {
    getContext.mockRestore();
  }
});

it.each([
  ['color(srgb 1 0.5 0)', '#ff8000'],
  ['color(srgb 1.02 -0.01 0.2)', '#ff0033'],
  ['color(srgb 0.1 0.2 0.3 / 0.5)', null],
  ['rgb(18, 52, 86)', '#123456'],
  ['rgba(18, 52, 86, 0.5)', null],
  ['oklch(0.5 0.1 200)', null]
])('converts the computed colour %s to %s', (value, hex) => {
  expect(hexFromComputedColor(value)).toBe(hex);
});

it('uses the saved surface of the active tone as the browser theme colour', () => {
  const root = document.documentElement;
  const meta = document.createElement('meta');
  meta.name = 'theme-color';
  document.head.append(meta);
  try {
    root.dataset.theme = 'dark';
    applySurfaceTones({ light: 'gray', dark: 'plum' });
    expect(meta.content).toBe(getLoadingPalettes()?.dark.surface);
    expect(meta.content).not.toBe('#262626');
    applyContrastAge(40);
    expect(meta.content).toBe('#000000');
  } finally {
    meta.remove();
  }
});
