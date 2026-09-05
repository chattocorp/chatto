import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

const mocks = vi.hoisted(() => ({
  page: {
    url: new URL('https://chatto.test/chat'),
    state: {} as App.PageState
  },
  replaceState: vi.fn()
}));

vi.mock('$app/state', () => ({ page: mocks.page }));
vi.mock('$app/paths', () => ({ base: '', resolve: (path: string) => path }));
vi.mock('$app/navigation', () => ({ replaceState: mocks.replaceState }));
vi.mock('$lib/ui', async () => ({ Hint: (await import('$lib/ui/Hint.svelte')).default }));
vi.mock('$lib/i18n/messages', () => ({ m: (key: string) => key }));

import WelcomeBanner from './WelcomeBanner.svelte';

describe('WelcomeBanner', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout'] });
    mocks.page.url = new URL('https://chatto.test/chat');
    mocks.page.state = {};
    mocks.replaceState.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it.each(['query', 'state', 'both'])(
    'consumes the %s flag and preserves other navigation values',
    async (source) => {
      mocks.page.url = new URL(
        `https://chatto.test/chat?filter=unread${source !== 'state' ? '&welcome=true' : ''}#latest`
      );
      mocks.page.state = {
        threadFilter: 'unread',
        ...(source !== 'query' ? { welcome: true } : {})
      };

      const screen = render(WelcomeBanner);
      await tick();
      await vi.advanceTimersByTimeAsync(0);
      await expect.element(screen.getByText('welcome.verified')).toBeVisible();
      expect(mocks.replaceState).toHaveBeenCalledExactlyOnceWith(
        '/chat?filter=unread#latest',
        { threadFilter: 'unread' }
      );
    }
  );

  it('does not show or replace navigation state without a true welcome flag', async () => {
    mocks.page.url.searchParams.set('welcome', 'false');
    mocks.page.state = { welcome: false };
    const screen = render(WelcomeBanner);
    await tick();
    await vi.advanceTimersByTimeAsync(0);

    await expect.element(screen.getByText('welcome.verified')).not.toBeInTheDocument();
    expect(mocks.replaceState).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('dismisses after five seconds', async () => {
    mocks.page.state = { welcome: true };
    const screen = render(WelcomeBanner);
    await tick();
    await vi.advanceTimersByTimeAsync(0);

    await vi.advanceTimersByTimeAsync(4999);
    await expect.element(screen.getByText('welcome.verified')).toBeVisible();
    await vi.advanceTimersByTimeAsync(1);
    await expect.element(screen.getByText('welcome.verified')).not.toBeInTheDocument();
  });

  it('cancels the dismissal timer when manually dismissed', async () => {
    mocks.page.state = { welcome: true };
    const screen = render(WelcomeBanner);
    await tick();
    await vi.advanceTimersByTimeAsync(0);
    expect(vi.getTimerCount()).toBe(1);

    await screen.getByRole('button', { name: 'common.dismiss' }).click();
    await vi.advanceTimersByTimeAsync(0);
    await expect.element(screen.getByText('welcome.verified')).not.toBeInTheDocument();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('does not consume navigation flags after an immediate unmount', async () => {
    mocks.page.state = { welcome: true };
    const screen = render(WelcomeBanner);
    screen.unmount();
    await vi.advanceTimersByTimeAsync(0);
    expect(mocks.replaceState).not.toHaveBeenCalled();
  });

  it('cancels the dismissal timer when unmounted', async () => {
    mocks.page.state = { welcome: true };
    const screen = render(WelcomeBanner);
    await tick();
    await vi.advanceTimersByTimeAsync(0);
    expect(vi.getTimerCount()).toBe(1);

    screen.unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});
