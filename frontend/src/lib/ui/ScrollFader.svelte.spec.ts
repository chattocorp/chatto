import { createRawSnippet, flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { describe, expect, it } from 'vitest';
import ScrollFader from './ScrollFader.svelte';

function snippet(html: string) {
  return createRawSnippet(() => ({
    render: () => html
  }));
}

function setScrollMetrics(
  el: HTMLElement,
  metrics: { scrollTop: number; scrollHeight: number; clientHeight: number }
) {
  let scrollTop = metrics.scrollTop;

  Object.defineProperties(el, {
    scrollTop: {
      configurable: true,
      get: () => scrollTop,
      set: (value) => {
        scrollTop = value;
      }
    },
    scrollHeight: {
      configurable: true,
      get: () => metrics.scrollHeight
    },
    clientHeight: {
      configurable: true,
      get: () => metrics.clientHeight
    }
  });
}

async function nextFrame() {
  await new Promise((resolve) => requestAnimationFrame(() => resolve(null)));
}

function getBottomFade(container: HTMLElement) {
  const fades = container.querySelectorAll<HTMLElement>('[aria-hidden="true"]');
  return fades[fades.length - 1];
}

const scrollFaderProps = (refreshKey: number) => ({
  top: true,
  bottom: true,
  refreshKey,
  'data-testid': 'scroll',
  children: snippet('<div data-testid="content">Message</div>')
});

describe('ScrollFader', () => {
  it('recomputes bottom fade visibility when refreshKey changes without a scroll event', async () => {
    const { container, rerender } = render(ScrollFader, {
      props: scrollFaderProps(0)
    });

    const scrollEl = container.querySelector<HTMLElement>('[data-testid="scroll"]');
    if (!scrollEl) throw new Error('scroll container not rendered');

    setScrollMetrics(scrollEl, { scrollTop: 150, scrollHeight: 300, clientHeight: 100 });
    scrollEl.dispatchEvent(new Event('scroll'));
    flushSync();
    expect(getBottomFade(container).classList.contains('opacity-0')).toBe(false);

    scrollEl.scrollTop = 200;
    await rerender(scrollFaderProps(1));
    await nextFrame();
    flushSync();

    expect(getBottomFade(container).classList.contains('opacity-0')).toBe(true);
  });
});
