import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import MyThreadsNavItem from './MyThreadsNavItem.svelte';

vi.mock('$app/paths', () => ({
  assets: '',
  base: '',
  resolve: () => '/chat/-/threads'
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'local'
}));

describe('MyThreadsNavItem', () => {
  it('uses gray for unread threads and orange when one needs attention', () => {
    const unread = render(MyThreadsNavItem, {
      props: { active: false, hasUnread: true }
    });
    expect(q(unread.container, '[data-testid="my-threads-unread-dot"]')?.className).toContain(
      'bg-neutral-action'
    );

    const notified = render(MyThreadsNavItem, {
      props: { active: false, hasUnread: true, hasNotification: true }
    });
    expect(q(notified.container, '[data-testid="my-threads-unread-dot"]')?.className).toContain(
      'bg-attention'
    );
  });
});
