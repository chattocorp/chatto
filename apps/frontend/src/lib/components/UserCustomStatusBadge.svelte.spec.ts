import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import UserCustomStatusBadge from './UserCustomStatusBadge.svelte';

describe('custom status expiry', () => {
  beforeEach(() => {
    // Keep animation frames real so Svelte's async tick can finish during rerender.
    vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout'] });
    vi.setSystemTime(new Date('2026-09-05T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('hides a status when its deadline is reached', async () => {
    const rendered = render(UserCustomStatusBadge, {
      props: {
        status: {
          emoji: '🌴',
          text: 'On holiday',
          expiresAt: new Date(Date.now() + 1000).toISOString()
        },
        showText: true
      }
    });

    await expect.element(rendered.getByText('On holiday')).toBeVisible();
    await vi.advanceTimersByTimeAsync(999);
    await expect.element(rendered.getByText('On holiday')).toBeVisible();
    await vi.advanceTimersByTimeAsync(1);
    await expect.element(rendered.getByText('On holiday')).not.toBeInTheDocument();
  });

  it('uses a replacement status deadline and supports removal of expiry', async () => {
    const initialTime = Date.now();
    const rendered = render(UserCustomStatusBadge, {
      props: {
        status: {
          emoji: '🌴',
          text: 'On holiday',
          expiresAt: new Date(initialTime + 1000).toISOString()
        },
        showText: true
      }
    });

    await rendered.rerender({
      status: {
        emoji: '🌴',
        text: 'On holiday',
        expiresAt: new Date(initialTime + 2000).toISOString()
      }
    });
    await vi.advanceTimersByTimeAsync(1000);
    await expect.element(rendered.getByText('On holiday')).toBeVisible();
    await vi.advanceTimersByTimeAsync(1000);
    await expect.element(rendered.getByText('On holiday')).not.toBeInTheDocument();

    await rendered.rerender({
      status: {
        emoji: '🌴',
        text: 'On holiday',
        expiresAt: new Date(Date.now() + 1000).toISOString()
      }
    });
    await rendered.rerender({ status: { emoji: '🌴', text: 'On holiday' } });
    await vi.advanceTimersByTimeAsync(2000);
    await expect.element(rendered.getByText('On holiday')).toBeVisible();
  });

  it.each(['2026-09-05T11:59:59Z', 'invalid'])(
    'does not display a status with expiry %s',
    async (expiresAt) => {
      const rendered = render(UserCustomStatusBadge, {
        props: { status: { emoji: '🌴', text: 'On holiday', expiresAt }, showText: true }
      });
      await expect.element(rendered.getByText('On holiday')).not.toBeInTheDocument();
    }
  );
});
