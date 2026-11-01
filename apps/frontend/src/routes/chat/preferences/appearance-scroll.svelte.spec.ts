import '../../../app.css';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { userEvent } from 'vitest/browser';
import AppearancePage from './appearance/+page.svelte';

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

/**
 * Waits until web fonts are loaded and the scroller geometry stays the same
 * for several frames. A late web-font swap can change the layout, and scroll
 * anchoring or clamping then changes `scrollTop` without any user action.
 */
async function settleLayout(scroll: HTMLElement) {
  const geometry = () => `${scroll.scrollHeight} ${scroll.scrollTop}`;
  let stableFrames = 0;
  let previous = geometry();
  for (let frame = 0; frame < 300 && stableFrames < 5; frame++) {
    await document.fonts.ready;
    await new Promise(requestAnimationFrame);
    const current = geometry();
    stableFrames =
      current === previous && document.fonts.status === 'loaded' ? stableFrames + 1 : 0;
    previous = current;
  }
  expect(stableFrames, 'layout did not settle').toBe(5);
}

it('keeps focus inside visible accent swatches after scrolling', async () => {
  const screen = render(AppearancePage);
  // Match the constrained flex column used by the settings route.
  screen.container.style.cssText = 'display:flex;flex-direction:column;width:390px;height:600px';
  await settle();

  const scroll = screen.container.querySelector<HTMLElement>('.overflow-y-auto')!;
  const heading = screen.getByRole('heading', { name: 'Appearance', exact: true }).element();
  const violet = screen.container.querySelector<HTMLInputElement>('input[value="violet"]')!;
  // Let initial layout and scroll anchoring finish before the user scrolls.
  await settleLayout(scroll);
  scroll.scrollTop +=
    violet.parentElement!.getBoundingClientRect().top - scroll.getBoundingClientRect().top - 200;
  await settleLayout(scroll);
  expect(scroll.scrollTop).toBeGreaterThan(0);

  for (const color of ['violet', 'pink', 'amber', 'grey']) {
    const input = screen.container.querySelector<HTMLInputElement>(`input[value="${color}"]`)!;
    const swatch = input.nextElementSibling as HTMLElement;
    const swatchBounds = swatch.getBoundingClientRect();
    const inputBounds = input.getBoundingClientRect();
    // A hidden focus target must move with its visible swatch, including
    // when the scroller is inside a positioned, non-scrolling wrapper.
    expect(inputBounds.top).toBeGreaterThanOrEqual(swatchBounds.top);
    expect(inputBounds.bottom).toBeLessThanOrEqual(swatchBounds.bottom);

    const ancestors: HTMLElement[] = [];
    for (let element = input.parentElement; element; element = element.parentElement) {
      ancestors.push(element);
    }
    const offsets = ancestors.map((element) => element.scrollTop);
    const headingTop = heading.getBoundingClientRect().top;
    const frameTop = screen.container.getBoundingClientRect().top;
    const samples: number[][] = [];
    let animationFrame = 0;
    const sample = () => {
      samples.push([
        heading.getBoundingClientRect().top,
        screen.container.getBoundingClientRect().top,
        ...ancestors.map((element) => element.scrollTop)
      ]);
      animationFrame = requestAnimationFrame(sample);
    };
    sample();
    try {
      await userEvent.click(swatch);
      await settle();
      // Observe intermediate frames as well as the final geometry.
      for (let frame = 0; frame < 12; frame++) await new Promise(requestAnimationFrame);
    } finally {
      cancelAnimationFrame(animationFrame);
    }
    expect(input.checked).toBe(true);
    expect(document.activeElement).toBe(input);
    for (const geometry of samples)
      expect(geometry, color).toEqual([headingTop, frameTop, ...offsets]);
  }
});
