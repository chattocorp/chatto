import '../../../app.css';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import VideoProcessingPlaceholder from './VideoProcessingPlaceholder.svelte';

it('uses the shared fog with one accessible status and a visible label', async () => {
  const view = render(VideoProcessingPlaceholder, { label: 'Processing video…' });
  const fog = view.container.querySelector<HTMLElement>('[data-loading-fog]');

  expect(fog?.getAttribute('role')).toBe('status');
  expect(fog?.getAttribute('aria-busy')).toBe('true');
  expect(fog?.getAttribute('aria-label')).toBe('Processing video…');
  await expect.element(view.getByText('Processing video…')).toBeVisible();
  expect(view.container.querySelectorAll('[role="status"]')).toHaveLength(1);
  expect(view.container.querySelector('canvas')).toBeNull();
});
