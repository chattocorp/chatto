import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { describe, expect, it } from 'vitest';
import ScrollFaderTestHarness from './ScrollFaderTestHarness.svelte';

async function nextFrame() {
  await new Promise((resolve) => requestAnimationFrame(() => resolve(null)));
}

function getBottomFade(container: HTMLElement) {
  const fades = container.querySelectorAll<HTMLElement>('[aria-hidden="true"]');
  return fades[fades.length - 1];
}

describe('ScrollFader', () => {
  it('recomputes bottom fade visibility when refreshKey changes without a scroll event', async () => {
    const { container, component } = render(ScrollFaderTestHarness);

    const scrollEl = container.querySelector<HTMLElement>('[data-testid="scroll"]');
    if (!scrollEl) throw new Error('scroll container not rendered');

    component.setScrollMetrics({ scrollTop: 150, scrollHeight: 300, clientHeight: 100 });
    scrollEl.dispatchEvent(new Event('scroll'));
    flushSync();
    expect(getBottomFade(container).classList.contains('opacity-0')).toBe(false);

    scrollEl.scrollTop = 200;
    component.refresh();
    flushSync();
    await nextFrame();
    flushSync();

    expect(getBottomFade(container).classList.contains('opacity-0')).toBe(true);
  });
});
