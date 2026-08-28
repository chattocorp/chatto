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
    const content = scrollArea.firstElementChild as HTMLElement;

    expect(scrollArea.className).toContain('overflow-y-auto');
    expect(fader.className).toContain('relative');
    expect(content.className).toContain('max-w-5xl');
    expect(content.className).toContain('w-full');
  });

  it('can give a primary child the available page height', () => {
    const { container } = render(PaneContent, {
      props: { fillHeight: true, children: testSnippet('<div>Content</div>') }
    });
    const content = container.firstElementChild?.firstElementChild?.firstElementChild as HTMLElement;

    expect(content.className).toContain('h-full');
    expect(content.className).toContain('flex');
  });
});
