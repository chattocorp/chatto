import { cdp } from 'vitest/browser';

const FINE_HOVER_POINTER = [
  { name: 'hover', value: 'hover' },
  { name: 'any-hover', value: 'hover' },
  { name: 'pointer', value: 'fine' },
  { name: 'any-pointer', value: 'fine' }
];

/**
 * Makes the browser report a mouse: a fine pointer that can hover, and no
 * coarse pointer. The headless browser in CI reports no hover support, so
 * hover-only styles do not apply there without this. Browser specs only.
 *
 * Returns a function that restores the browser's own media features. Call it
 * in `afterEach` or a `finally` block.
 */
export async function emulateHoverPointer(): Promise<() => Promise<void>> {
  await cdp().send('Emulation.setEmulatedMedia', { features: FINE_HOVER_POINTER });
  return async () => {
    await cdp().send('Emulation.setEmulatedMedia', { features: [] });
  };
}
