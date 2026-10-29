import { afterEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import CopyButton from './CopyButton.svelte';

afterEach(() => vi.restoreAllMocks());

it('shows the check only after the clipboard accepts the text', async () => {
  let finish!: () => void;
  const write = vi
    .spyOn(navigator.clipboard, 'writeText')
    .mockImplementation(() => new Promise<void>((resolve) => (finish = resolve)));
  const { container } = render(CopyButton, { value: 'sample' });
  const button = container.querySelector('button')!;
  button.click();
  expect(write).toHaveBeenCalledWith('sample');
  expect(button.dataset.copied).toBeUndefined();
  finish();
  await vi.waitFor(() => expect(button.dataset.copied).toBe('true'));
});

it('does not show a success check when copying fails', async () => {
  vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new Error('Denied'));
  const { container } = render(CopyButton, { value: 'sample' });
  const button = container.querySelector('button')!;
  button.click();
  await Promise.resolve();
  await Promise.resolve();
  expect(button.dataset.copied).toBeUndefined();
});
