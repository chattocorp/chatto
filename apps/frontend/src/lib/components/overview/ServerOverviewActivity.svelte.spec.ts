import '../../../app.css';
import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
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
    render(ServerOverviewActivity, { serverSegment: '-', calls: [call], catchUp: [], onjoincall });

    await expect.element(page.getByRole('link', { name: '#lounge' })).toBeVisible();
    await expect.element(page.getByText('2 people in the call')).toBeVisible();
    await page.getByRole('button', { name: 'Join call' }).click();
    expect(onjoincall).toHaveBeenCalledWith('lounge-id');
  });

  it('lists catch-up rooms with notification counts or an unread marker', async () => {
    render(ServerOverviewActivity, {
      serverSegment: '-',
      calls: [],
      catchUp: [
        { id: 'design-id', name: 'design', importantCount: 2, otherCount: 0 },
        { id: 'random-id', name: 'random', importantCount: 0, otherCount: 0 }
      ],
      onjoincall: vi.fn()
    });

    await expect
      .element(page.getByRole('link', { name: /design/ }))
      .toHaveAttribute('href', expect.stringContaining('/chat/-/design-id'));
    await expect.element(page.getByText('2 notifications')).toBeInTheDocument();
    await expect.element(page.getByText('Unread messages')).toBeInTheDocument();
  });

  it('renders nothing without activity', () => {
    const { container } = render(ServerOverviewActivity, {
      serverSegment: '-',
      calls: [],
      catchUp: [],
      onjoincall: vi.fn()
    });
    expect(container.querySelector('section')).toBeNull();
  });
});
