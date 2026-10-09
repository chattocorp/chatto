import '../../app.css';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import PaneContent from './PaneContent.svelte';
import { page } from 'vitest/browser';

describe('PaneContent', () => {
  it('keeps one narrow page gutter for content outside panels', async () => {
    const rendered = render(PaneContent, {
      props: { children: testSnippet('<div>Content</div>') }
    });
    const content = rendered.container.querySelector('[data-page-reveal]')!;
    try {
      await page.viewport(390, 800);
      expect(getComputedStyle(content).paddingLeft).toBe('16px');
      expect(getComputedStyle(content).paddingTop).toBe('16px');
      await page.viewport(768, 800);
      expect(getComputedStyle(content).paddingLeft).toBe('24px');
      expect(getComputedStyle(content).paddingTop).toBe('24px');
    } finally {
      await page.viewport(1280, 720);
    }
  });

  it('provides one readable-width, scrollable pane column', () => {
    const { container } = render(PaneContent, {
      props: { children: testSnippet('<div data-testid="content">Content</div>') }
    });
    const fader = container.firstElementChild as HTMLElement;
    const scrollArea = fader.firstElementChild as HTMLElement;
    const content = container.querySelector('[data-testid="content"]')!.parentElement!;

    expect(scrollArea.className).toContain('overflow-y-auto');
    expect(fader.className).toContain('relative');
    expect(content.classList).toContain('max-w-pane');
    expect(content.classList).toContain('w-full');
  });

  it('allows a wider column for browsing grids', () => {
    const { container } = render(PaneContent, {
      props: { wide: true, children: testSnippet('<div data-testid="content">Content</div>') }
    });
    const content = container.querySelector('[data-testid="content"]')!.parentElement!;

    expect(content.classList).toContain('max-w-pane-wide');
    expect(content.classList).not.toContain('max-w-pane');
    expect(content.dataset.paneContent).toBe('wide');
  });

  it('can give a primary child the available page height', () => {
    const { container } = render(PaneContent, {
      props: {
        fillHeight: true,
        children: testSnippet('<div class="flex-1" data-testid="primary">Content</div>')
      }
    });
    container.style.cssText = 'display: flex; flex-direction: column; height: 240px; width: 400px;';
    const primary = container.querySelector<HTMLElement>('[data-testid="primary"]')!;
    const content = primary.parentElement!;
    const viewport = container.querySelector<HTMLElement>('.overflow-y-auto')!;
    const style = getComputedStyle(content);

    expect(viewport.clientHeight).toBe(240);
    expect(content.getBoundingClientRect().height).toBe(viewport.clientHeight);
    expect(primary.getBoundingClientRect().height).toBe(
      viewport.clientHeight -
        Number.parseFloat(style.paddingTop) -
        Number.parseFloat(style.paddingBottom)
    );
  });
  it('bounds an overflowing primary child to the available page height', () => {
    const { container } = render(PaneContent, {
      props: {
        fillHeight: true,
        children: testSnippet(
          '<div class="min-h-0 flex-1 overflow-y-auto" data-testid="primary"><div style="height: 600px">Tall content</div></div>'
        )
      }
    });
    container.style.cssText = 'display: flex; flex-direction: column; height: 240px; width: 400px;';
    const primary = container.querySelector<HTMLElement>('[data-testid="primary"]')!;
    const content = primary.parentElement!;
    const viewport = container.querySelector<HTMLElement>('.overflow-y-auto')!;
    expect(content.getBoundingClientRect().height).toBe(viewport.clientHeight);
    expect(primary.scrollHeight).toBe(600);
    expect(primary.clientHeight).toBeLessThan(viewport.clientHeight);
  });
});
