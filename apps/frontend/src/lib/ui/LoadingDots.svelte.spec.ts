import '../../app.css';
import { expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import LoadingDots from './LoadingDots.svelte';

it('announces a busy status with its label', async () => {
  render(LoadingDots, { props: { label: 'Loading messages...' } });
  const status = page.getByRole('status', { name: 'Loading messages...' });
  await expect.element(status).toHaveAttribute('aria-busy', 'true');
  expect(status.element().querySelectorAll('.loading-dot')).toHaveLength(3);
});
