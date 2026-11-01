import '../../app.css';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { applySurfaceDepth } from '$lib/state/userPreferences.svelte';

/** Read paint alpha without depending on the browser's colour serialization. */
function paintAlphas(value: string): number[] {
  const context = document.createElement('canvas').getContext('2d')!;
  const colours = value.match(/rgba?\([^)]*\)|color\([^)]*\)/g) ?? [];
  return colours.map((colour) => {
    context.clearRect(0, 0, 1, 1);
    context.fillStyle = colour;
    context.fillRect(0, 0, 1, 1);
    return context.getImageData(0, 0, 1, 1).data[3];
  });
}

describe('preview frame depth', () => {
  let frame: HTMLDivElement;
  let content: HTMLDivElement;
  let previousStyle: string | null;
  let previousTheme: string | undefined;

  beforeEach(() => {
    const root = document.documentElement;
    previousStyle = root.getAttribute('style');
    previousTheme = root.dataset.theme;
    root.style.transition = 'none';
    frame = document.createElement('div');
    frame.className = 'preview-frame';
    frame.style.width = '240px';
    content = document.createElement('div');
    content.className = 'preview-content';
    content.style.height = '120px';
    frame.append(content);
    document.body.append(frame);
  });

  afterEach(() => {
    frame.remove();
    const root = document.documentElement;
    if (previousStyle === null) root.removeAttribute('style');
    else root.setAttribute('style', previousStyle);
    if (previousTheme === undefined) delete root.dataset.theme;
    else root.dataset.theme = previousTheme;
  });

  function expectFlat() {
    const outer = getComputedStyle(frame);
    const inner = getComputedStyle(content);
    for (const paint of [
      outer.borderTopColor,
      outer.backgroundColor,
      outer.backgroundImage,
      outer.boxShadow,
      inner.boxShadow
    ]) {
      expect(paintAlphas(paint).every((alpha) => alpha === 0)).toBe(true);
    }
  }

  it.each(['light', 'dark'])('follows the live depth setting without resizing (%s)', (theme) => {
    document.documentElement.dataset.theme = theme;
    applySurfaceDepth(0);
    expectFlat();
    const initial = frame.getBoundingClientRect();
    let previousShadowAlpha = 0;
    for (const level of [10, 50, 100]) {
      applySurfaceDepth(level);
      const style = getComputedStyle(frame);
      expect(paintAlphas(style.borderTopColor)[0]).toBeGreaterThan(0);
      expect(paintAlphas(style.backgroundColor)[0]).toBeGreaterThan(0);
      const shadowAlpha = Math.max(...paintAlphas(style.boxShadow));
      expect(shadowAlpha).toBeGreaterThan(previousShadowAlpha);
      previousShadowAlpha = shadowAlpha;
      const bounds = frame.getBoundingClientRect();
      expect([bounds.width, bounds.height]).toEqual([initial.width, initial.height]);
    }
    applySurfaceDepth(0);
    expectFlat();
  });

  it.each(['light', 'dark'])('applies local Flat depth to the complete frame (%s)', (theme) => {
    document.documentElement.dataset.theme = theme;
    applySurfaceDepth(100);
    frame.style.setProperty('--depth-strength', '0');
    expectFlat();
  });

  it('keeps floating menu boundaries and elevation at Flat', () => {
    applySurfaceDepth(0);
    frame.className = 'menu';
    const style = getComputedStyle(frame);
    expect(paintAlphas(style.borderTopColor)[0]).toBeGreaterThan(0);
    expect(Math.max(...paintAlphas(style.boxShadow))).toBeGreaterThan(0);
  });
});
