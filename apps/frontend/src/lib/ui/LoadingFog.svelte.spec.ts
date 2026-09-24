import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import LoadingFog from './LoadingFog.svelte';

it('keeps its busy state while the motion code loads', async () => {
  const view = render(LoadingFog, { props: { class: 'h-24 w-40' } });
  const fog = view.container.querySelector<HTMLElement>('[data-loading-fog]')!;
  expect(fog.getAttribute('aria-busy')).toBe('true');
  expect(fog.getAttribute('role')).toBe('status');

  await vi.waitFor(() => expect(fog.style.getPropertyValue('--fog-first-x')).not.toBe(''));
  view.unmount();
});
