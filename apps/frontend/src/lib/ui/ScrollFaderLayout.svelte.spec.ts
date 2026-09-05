import '../../app.css';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { describe, expect, it, vi } from 'vitest';
import Harness from './ScrollFaderLayoutHarness.svelte';

function layout(container: HTMLElement) {
  const viewport = container.querySelector<HTMLElement>('[data-testid="layout-scroll"]')!;
  const content = container.querySelector<HTMLElement>('[data-testid="layout-content"]')!;
  const fades = () =>
    [...viewport.parentElement!.querySelectorAll<HTMLElement>('[aria-hidden="true"]')].map(
      (fade) => !fade.classList.contains('opacity-0')
    );
  return { viewport, content, fades };
}

describe('ScrollFader content layout', () => {
  it('bottom-aligns short content and tracks growth, scrolling, and shrinkage', async () => {
    const { container, component } = render(Harness);
    const { viewport, content, fades } = layout(container);
    expect(viewport.clientHeight).toBe(200);
    expect(content.getBoundingClientRect().bottom).toBe(viewport.getBoundingClientRect().bottom);
    expect(viewport.scrollHeight).toBe(200);
    component.resizeContent(500);
    flushSync();
    await vi.waitFor(() => expect(fades()).toEqual([false, true]));
    viewport.scrollTop = viewport.scrollHeight;
    await vi.waitFor(() => expect(fades()).toEqual([true, false]));
    component.resizeContent(60);
    flushSync();
    await vi.waitFor(() => expect(fades()).toEqual([false, false]));
    expect(viewport.scrollHeight).toBe(200);
  });

  it('updates fades when only the viewport size changes', async () => {
    const { container, component } = render(Harness);
    const { fades } = layout(container);
    component.resizeContent(300);
    flushSync();
    await vi.waitFor(() => expect(fades()).toEqual([false, true]));
    component.resizeViewport(400);
    flushSync();
    await vi.waitFor(() => expect(fades()).toEqual([false, false]));
  });

  it('measures intrinsic content and caps taller previews', async () => {
    const { container, component } = render(Harness, { intrinsic: true });
    const { viewport, fades } = layout(container);
    expect(viewport.clientHeight).toBe(60);
    component.resizeContent(500);
    flushSync();
    await vi.waitFor(() => expect(fades()).toEqual([false, true]));
    expect(viewport.clientHeight).toBe(Number.parseFloat(getComputedStyle(viewport).maxHeight));
  });

  it('updates fades after an image gains its natural height', async () => {
    const { container, component } = render(Harness);
    const { viewport, fades } = layout(container);
    component.loadImage();
    flushSync();
    await container.querySelector('img')!.decode();
    await vi.waitFor(() => expect(fades()).toEqual([false, true]));
    expect(viewport.scrollHeight).toBe(460);
  });
});
