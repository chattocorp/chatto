import '../../../app.css';
import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { NotificationAttentionLevel } from '@chatto/client/api/notifications';
import ServerOverviewActivity, { type OverviewCall } from './ServerOverviewActivity.svelte';

const call: OverviewCall = {
  roomId: 'lounge-id',
  roomName: 'lounge',
  participants: ['ada', 'grace'].map((login) => ({
    userId: `${login}-id`,
    login,
    displayName: login,
    avatarUrl: null,
    isBot: false
  }))
};

describe('ServerOverviewActivity', () => {
  it('shows an active call and opens it from Join call', async () => {
    const onjoincall = vi.fn();
    render(ServerOverviewActivity, {
      serverSegment: '-',
      calls: [call],
      catchUp: [],
      onjoincall,
      onopennotification: vi.fn()
    });

    await expect.element(page.getByRole('link', { name: 'lounge' })).toBeVisible();
    await expect.element(page.getByRole('img', { name: '2 people in the call' })).toBeVisible();
    await page.getByRole('button', { name: 'Join call' }).click();
    expect(onjoincall).toHaveBeenCalledWith('lounge-id');
  });

  it('opens the room from its name and the newest notification from each badge', async () => {
    const onopennotification = vi.fn();
    render(ServerOverviewActivity, {
      serverSegment: '-',
      calls: [],
      catchUp: [
        { id: 'design-id', name: 'design', importantCount: 2, otherCount: 1 },
        { id: 'random-id', name: 'random', importantCount: 0, otherCount: 0 }
      ],
      onjoincall: vi.fn(),
      onopennotification
    });

    await expect
      .element(page.getByRole('link', { name: /design/ }))
      .toHaveAttribute('href', expect.stringContaining('/chat/-/design-id'));
    await page.getByRole('button', { name: 'Go to 2 notifications' }).click();
    expect(onopennotification).toHaveBeenCalledWith(
      'design-id',
      NotificationAttentionLevel.IMPORTANT
    );
    await page.getByRole('button', { name: 'Go to 1 other notifications' }).click();
    expect(onopennotification).toHaveBeenLastCalledWith(
      'design-id',
      NotificationAttentionLevel.AMBIENT
    );
    await expect.element(page.getByText('Unread messages')).toBeInTheDocument();
  });

  it('renders nothing without activity', () => {
    const { container } = render(ServerOverviewActivity, {
      serverSegment: '-',
      calls: [],
      catchUp: [],
      onjoincall: vi.fn(),
      onopennotification: vi.fn()
    });
    expect(container.querySelector('section')).toBeNull();
  });
});
