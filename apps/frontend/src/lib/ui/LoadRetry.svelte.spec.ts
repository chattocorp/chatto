import '../../app.css';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import LoadRetry from './LoadRetry.svelte';

beforeEach(async () => {
  await loadLocaleMessages('en-GB');
  setReactiveLocale('en-GB');
});

afterEach(() => {
  vi.useRealTimers();
});

describe('LoadRetry', () => {
  it('retries at an interval and when the browser goes online', () => {
    vi.useFakeTimers();
    const onretry = vi.fn();
    render(LoadRetry, { props: { onretry } });

    vi.advanceTimersByTime(4_999);
    expect(onretry).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(onretry).toHaveBeenCalledOnce();

    window.dispatchEvent(new Event('online'));
    expect(onretry).toHaveBeenCalledTimes(2);
  });

  it('waits while the tab is hidden and retries when it becomes visible', () => {
    vi.useFakeTimers();
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    try {
      const onretry = vi.fn();
      render(LoadRetry, { props: { onretry } });

      vi.advanceTimersByTime(5_000);
      window.dispatchEvent(new Event('online'));
      expect(onretry).not.toHaveBeenCalled();

      visibility.mockReturnValue('visible');
      document.dispatchEvent(new Event('visibilitychange'));
      expect(onretry).toHaveBeenCalledOnce();
    } finally {
      visibility.mockRestore();
    }
  });

  it('leaves the retry to the user when another attempt cannot help', async () => {
    vi.useFakeTimers();
    const onretry = vi.fn();
    render(LoadRetry, { props: { onretry, autoRetry: false } });

    vi.advanceTimersByTime(60_000);
    window.dispatchEvent(new Event('online'));
    document.dispatchEvent(new Event('visibilitychange'));
    expect(onretry).not.toHaveBeenCalled();

    vi.useRealTimers();
    await page.getByRole('button', { name: 'Try Again' }).click();
    expect(onretry).toHaveBeenCalledOnce();
  });

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
