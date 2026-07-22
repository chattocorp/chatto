import '../../app.css';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import ScrollArea from './ScrollArea.svelte';

it('provides a native horizontal and vertical scroll viewport', () => {
  const { container } = render(ScrollArea, {
    props: {
      children: testSnippet('<div class="w-[40rem]">Scrollable table content</div>'),
      scrollX: true,
      class: 'h-48 rounded-md',
      scrollClass: 'overscroll-contain',
      'data-testid': 'scroll-area'
    }
  });

  const scrollArea = container.querySelector<HTMLElement>('[data-testid="scroll-area"]')!;

  expect(scrollArea.className).toContain('overflow-y-auto');
  expect(scrollArea.className).toContain('overflow-x-auto');
  expect(scrollArea.className).toContain('overscroll-contain');
  expect(scrollArea.parentElement?.className).toContain('relative');
});
