import '../../app.css';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import LoadingFog from './LoadingFog.svelte';

it('shows a busy fog and removes it as soon as loading ends', async () => {
  const view = render(LoadingFog, { props: { class: 'h-24 w-40' } });
  const fog = view.container.querySelector<HTMLElement>('[data-loading-fog]')!;
  expect(fog.getAttribute('aria-busy')).toBe('true');
  expect(fog.getAttribute('role')).toBe('status');

  await vi.waitFor(() => expect(fog.style.getPropertyValue('--fog-first-x')).not.toBe(''));
  view.unmount();
  expect(fog.isConnected).toBe(false);
  expect(document.querySelector('[data-loading-fog]')).toBeNull();
  expect(document.querySelector('.loading-fog[aria-hidden="true"]')).toBeNull();
});
