import '../../app.css';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import PaneContent from './PaneContent.svelte';

describe('PaneContent', () => {
  it('provides one readable-width, scrollable pane column', () => {
    const { container } = render(PaneContent, {
      props: { children: testSnippet('<div data-testid="content">Content</div>') }
    });
    const fader = container.firstElementChild as HTMLElement;
    const scrollArea = fader.firstElementChild as HTMLElement;
    const content = container.querySelector('[data-testid="content"]')!.parentElement!;

    expect(scrollArea.className).toContain('overflow-y-auto');
    expect(fader.className).toContain('relative');
    expect(content.className).toContain('max-w-5xl');
    expect(content.className).toContain('w-full');
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
