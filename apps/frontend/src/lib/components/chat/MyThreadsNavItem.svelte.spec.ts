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
  it('renders and clears the dot from followed-thread unread state', async () => {
    const rendered = render(MyThreadsNavItem, {
      props: { active: false, hasUnread: false }
    });

    expect(rendered.container.querySelector('[data-testid="my-threads-unread-dot"]')).toBeNull();

    await rendered.rerender({ active: false, hasUnread: true });
    await expect
      .element(
        rendered.container.querySelector<HTMLElement>('[data-testid="my-threads-unread-dot"]')
      )
      .toBeInTheDocument();

    await rendered.rerender({ active: false, hasUnread: false });
    expect(rendered.container.querySelector('[data-testid="my-threads-unread-dot"]')).toBeNull();
  });
});
