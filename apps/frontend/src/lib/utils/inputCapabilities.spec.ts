import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  prefersTouchActions,
  supportsAnyFinePointer,
  supportsHoverActions
} from './inputCapabilities';

function mediaQueryList(query: string, matches: boolean): MediaQueryList {
  return {
    matches,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(() => false)
  } as MediaQueryList;
}

function mockMedia(matches: Record<string, boolean>) {
  vi.stubGlobal('window', {
    matchMedia: vi.fn((query: string) => mediaQueryList(query, matches[query] ?? false))
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('input capabilities', () => {
  it('treats touch-primary devices as preferring touch actions', () => {
    mockMedia({
      '(pointer: coarse)': true
    });

    expect(prefersTouchActions()).toBe(true);
    expect(supportsHoverActions()).toBe(false);
  });

  it('supports hover actions independently of viewport size', () => {
    mockMedia({
      '(any-hover: hover)': true,
      '(any-pointer: fine)': true
    });

    expect(prefersTouchActions()).toBe(false);
    expect(supportsAnyFinePointer()).toBe(true);
    expect(supportsHoverActions()).toBe(true);
  });

  it('allows hover actions on hybrid devices with a fine hover pointer', () => {
    mockMedia({
      '(pointer: coarse)': true,
      '(any-hover: hover)': true,
      '(any-pointer: fine)': true
    });

    expect(prefersTouchActions()).toBe(true);
    expect(supportsHoverActions()).toBe(true);
  });

  it('keeps mouse keyboard defaults and hover actions on mouse-primary hybrids', () => {
    mockMedia({
      '(pointer: fine)': true,
      '(any-pointer: coarse)': true,
      '(any-pointer: fine)': true,
      '(any-hover: hover)': true
    });
    expect(prefersTouchActions()).toBe(false);
    expect(supportsHoverActions()).toBe(true);
  });
});
