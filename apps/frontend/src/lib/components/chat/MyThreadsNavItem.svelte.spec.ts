import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MyThreadsNavItem from './MyThreadsNavItem.svelte';

vi.mock('$app/paths', () => ({
  resolve: (path: string, params: Record<string, string>) =>
    path.replace('[serverId]', params.serverId)
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (serverId: string) => serverId
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'server-1'
}));

describe('MyThreadsNavItem', () => {
  it('uses the room-unread color for unread activity and warning for notifications', async () => {
    const rendered = render(MyThreadsNavItem, {
      props: { active: false, hasUnread: false, hasNotification: false }
    });

    expect(rendered.container.querySelector('[data-testid="my-threads-unread-dot"]')).toBeNull();

    await rendered.rerender({ active: false, hasUnread: true, hasNotification: false });
    const unreadDot = rendered.container.querySelector<HTMLElement>(
      '[data-testid="my-threads-unread-dot"]'
    );
    await expect.element(unreadDot).toBeInTheDocument();
    expect(unreadDot).toHaveClass('bg-primary');

    await rendered.rerender({ active: false, hasUnread: true, hasNotification: true });
    expect(
      rendered.container.querySelector<HTMLElement>('[data-testid="my-threads-unread-dot"]')
    ).toHaveClass('bg-warning');

    await rendered.rerender({ active: false, hasUnread: false, hasNotification: false });
    expect(rendered.container.querySelector('[data-testid="my-threads-unread-dot"]')).toBeNull();
  });
});
