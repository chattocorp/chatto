import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { on } from 'svelte/events';
import PreviewAction from './PreviewAction.svelte';

it('runs its action without activating the containing card', () => {
  const onclick = vi.fn();
  const onCardClick = vi.fn();
  const { container } = render(PreviewAction, {
    label: 'Delete preview',
    icon: 'icon-[uil--trash-alt]',
    onclick
  });
  const cleanup = on(container, 'click', onCardClick);
  container.querySelector('button')!.click();
  cleanup();
  expect(onclick).toHaveBeenCalledOnce();
  expect(onCardClick).not.toHaveBeenCalled();
});
