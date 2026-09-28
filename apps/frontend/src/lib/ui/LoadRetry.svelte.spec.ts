import '../../app.css';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import LoadRetry from './LoadRetry.svelte';

beforeEach(async () => {
  await loadLocaleMessages('en-GB');
  setReactiveLocale('en-GB');
});

describe('LoadRetry', () => {
  it('announces the network error and retries', async () => {
    const onretry = vi.fn();
    render(LoadRetry, { props: { onretry } });

    const alert = page.getByRole('alert');
    await expect.element(alert).toHaveTextContent('Network error');
    await page.getByRole('button', { name: 'Try Again' }).click();

    expect(onretry).toHaveBeenCalledOnce();
  });

  it('shows a custom message and a secondary action after Retry', async () => {
    const onclose = vi.fn();
    render(LoadRetry, {
      props: {
        onretry: vi.fn(),
        message: 'Thread unavailable',
        secondaryAction: { label: 'Close thread', onclick: onclose }
      }
    });

    await expect.element(page.getByRole('alert')).toHaveTextContent('Thread unavailable');
    const buttons = page.getByRole('button').elements();
    expect(buttons.map((button) => button.textContent?.trim())).toEqual([
      'Try Again',
      'Close thread'
    ]);
    await page.getByRole('button', { name: 'Close thread' }).click();
    expect(onclose).toHaveBeenCalledOnce();
  });
});
