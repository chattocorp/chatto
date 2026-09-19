import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { Capacitor } from '@capacitor/core';
import { useVisualViewport } from './useVisualViewport.svelte';

let dispose: (() => void) | undefined;
let viewport: EventTarget & { height: number; width: number; offsetTop: number };

beforeEach(() => {
  viewport = Object.assign(new EventTarget(), { height: 800, width: 390, offsetTop: 0 });
  vi.spyOn(window, 'visualViewport', 'get').mockReturnValue(viewport as VisualViewport);
});

afterEach(() => {
  dispose?.();
  dispose = undefined;
  vi.restoreAllMocks();
});

function start() {
  dispose = $effect.root(() => useVisualViewport());
  flushSync();
}

function resize(height: number, width = viewport.width) {
  Object.assign(viewport, { height, width });
  viewport.dispatchEvent(new Event('resize'));
}

describe('useVisualViewport keyboard state', () => {
  it('leaves native iOS sizing and header visibility to the host', () => {
    vi.spyOn(Capacitor, 'getPlatform').mockReturnValue('ios');
    start();
    resize(400);
    expect(document.body.hasAttribute('data-keyboard-open')).toBe(false);
    expect(document.body.style.height).toBe('');
    resize(800);
    expect(document.body.style.height).toBe('');
  });
  it('exposes keyboard opening and closing with the body height', () => {
    start();
    expect(document.body.hasAttribute('data-keyboard-open')).toBe(false);
    resize(400);
    expect(document.body.hasAttribute('data-keyboard-open')).toBe(true);
    expect(document.body.style.height).toBe('400px');
    resize(800);
    expect(document.body.hasAttribute('data-keyboard-open')).toBe(false);
    expect(document.body.style.height).toBe('');
  });

  it('does not treat focus or a small viewport change as an open keyboard', () => {
    start();
    const input = document.createElement('input');
    document.body.append(input);
    try {
      input.focus();
      expect(document.body.hasAttribute('data-keyboard-open')).toBe(false);
      resize(740);
      expect(document.body.hasAttribute('data-keyboard-open')).toBe(false);
      expect(document.body.style.height).toBe('');
    } finally {
      input.remove();
    }
  });

  it('clears keyboard state when the viewport changes orientation', () => {
    start();
    resize(400);
    resize(390, 800);
    expect(document.body.hasAttribute('data-keyboard-open')).toBe(false);
    expect(document.body.style.height).toBe('');
  });

  it('clears state and removes listeners on teardown', () => {
    start();
    resize(400);
    dispose?.();
    dispose = undefined;
    expect(document.body.hasAttribute('data-keyboard-open')).toBe(false);
    expect(document.body.style.height).toBe('');
    resize(350);
    expect(document.body.hasAttribute('data-keyboard-open')).toBe(false);
    expect(document.body.style.height).toBe('');
  });

  it('leaves headers visible when VisualViewport is unavailable', () => {
    vi.spyOn(window, 'visualViewport', 'get').mockReturnValue(null);
    start();
    expect(document.body.hasAttribute('data-keyboard-open')).toBe(false);
  });
});
