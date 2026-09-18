import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MotdModal from './MotdModal.svelte';

it('updates the full rendered MOTD when its content changes', async () => {
  const rendered = render(MotdModal, { motd: '**First message**', onclose: vi.fn() });
  await expect.element(rendered.getByText('First message', { exact: true })).toBeVisible();
  await rendered.rerender({ motd: '**Updated message**' });
  await expect.element(rendered.getByText('Updated message', { exact: true })).toBeVisible();
});
