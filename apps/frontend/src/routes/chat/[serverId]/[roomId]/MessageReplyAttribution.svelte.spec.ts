import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import MessageReplyAttribution from './MessageReplyAttribution.svelte';

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_scope: { serverId: string; userId: string }, fallback: unknown) => fallback
  })
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
    getLiveBio: () => null,
    getLiveTimezone: () => null,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

const actor = {
  id: 'user-1',
  login: 'ada',
  displayName: 'Ada',
  avatarUrl: '',
  deleted: false,
  presenceStatus: PresenceStatus.ONLINE
};

describe('MessageReplyAttribution', () => {
  it('renders the active author, body, and call presence', async () => {
    const { container } = render(MessageReplyAttribution, {
      props: {
        preview: { name: 'Ada', body: 'Original message', actor, deleted: false },
        callPresence: 'video',
        onJump: vi.fn()
      }
    });

    await expect
      .element(q(container, '[data-testid="reply-attribution-author"]'))
      .toHaveTextContent('Ada');
    await expect
      .element(q(container, '[aria-label="in reply to Original message"]'))
      .toBeInTheDocument();
    await expect
      .element(q(container, '[data-testid="user-call-presence-video"]'))
      .toBeInTheDocument();
  });

  it('keeps author clicks separate from reply-target navigation', () => {
    const onJump = vi.fn();
    const onAuthorClick = vi.fn();
    const { container } = render(MessageReplyAttribution, {
      props: {
        preview: { name: 'Ada', body: 'Original message', actor, deleted: false },
        onJump,
        onAuthorClick
      }
    });

    q(container, '[data-testid="reply-attribution-author"]')!.click();

    expect(onAuthorClick).toHaveBeenCalledOnce();
    expect(onJump).not.toHaveBeenCalled();
  });

  it('uses the fallback body and deleted-user label when content is unavailable', async () => {
    const { container } = render(MessageReplyAttribution, {
      props: {
        preview: { name: 'Deleted user', body: null, actor: null, deleted: true },
        onJump: vi.fn()
      }
    });

    await expect.element(q(container, '[aria-label="in reply to Message"]')).toBeInTheDocument();
    expect(container.textContent).toContain('[deleted user]');
  });
});
