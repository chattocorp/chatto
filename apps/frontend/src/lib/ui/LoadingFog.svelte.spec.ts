import '../../app.css';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { prefersReducedMotion } from 'svelte/motion';
import LoadingFog from './LoadingFog.svelte';

it('keeps its busy state while the motion code loads', async () => {
  const view = render(LoadingFog, { props: { class: 'h-24 w-40' } });
  const fog = view.container.querySelector<HTMLElement>('[data-loading-fog]')!;
  expect(fog.getAttribute('aria-busy')).toBe('true');
  expect(fog.getAttribute('role')).toBe('status');

  await vi.waitFor(() => expect(fog.style.getPropertyValue('--fog-first-x')).not.toBe(''));
  view.unmount();
  await vi.waitFor(() =>
    expect(document.querySelector('.loading-fog[aria-hidden="true"]')).toBeNull()
  );
});

it('removes its busy state immediately while the visual copy fades out', async () => {
  const view = render(LoadingFog, { props: { class: 'h-24 w-40' } });
  const fog = view.container.querySelector<HTMLElement>('[data-loading-fog]')!;
  await vi.waitFor(() => expect(fog.getBoundingClientRect().width).toBeGreaterThan(0));
  await vi.waitFor(() => expect(fog.getBoundingClientRect().height).toBeGreaterThan(0));

  view.unmount();

  expect(fog.isConnected).toBe(false);
  expect(document.querySelector('[data-loading-fog]')).toBeNull();
  const ghost = document.querySelector<HTMLElement>('.loading-fog[aria-hidden="true"]');
  expect(ghost).not.toBeNull();
  expect(ghost?.getAttribute('aria-busy')).toBeNull();
  expect(ghost?.getAnimations()[0]?.effect?.getTiming().duration).toBe(120);
  await vi.waitFor(() => expect(ghost?.isConnected).toBe(false));
});

it('removes the fog without a visual copy when reduced motion is requested', () => {
  const reducedMotion = vi.spyOn(prefersReducedMotion, 'current', 'get').mockReturnValue(true);
  try {
    const view = render(LoadingFog, { props: { class: 'h-24 w-40' } });
    view.unmount();
    expect(document.querySelector('[data-loading-fog]')).toBeNull();
    expect(document.querySelector('.loading-fog[aria-hidden="true"]')).toBeNull();
  } finally {
    reducedMotion.mockRestore();
  }
});
