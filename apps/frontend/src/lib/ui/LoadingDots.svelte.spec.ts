import '../../app.css';
import { afterEach, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import LoadingDots from './LoadingDots.svelte';
import { applyAnimationPreference } from '$lib/dom/motionPreference';

afterEach(() => applyAnimationPreference('automatic'));

it('announces a busy status with its label', async () => {
  render(LoadingDots, { props: { label: 'Loading messages...' } });
  const status = page.getByRole('status', { name: 'Loading messages...' });
  await expect.element(status).toHaveAttribute('aria-busy', 'true');
  expect(status.element().querySelectorAll('.loading-dot')).toHaveLength(3);
});

it('keeps the reveal delay when the user requests less motion', () => {
  applyAnimationPreference('disabled');
  const { container } = render(LoadingDots);
  const dots = container.querySelector<HTMLElement>('[data-loading-dots]')!;
  const style = getComputedStyle(dots);
  // Removing the whole animation would also remove the delay and flash the dots.
  expect(style.animationName).not.toBe('none');
  expect(parseFloat(style.animationDelay)).toBeGreaterThan(0);
  expect(parseFloat(style.animationDuration)).toBeLessThanOrEqual(0.001);
  expect(getComputedStyle(dots.firstElementChild!).animationName).toBe('none');
});
