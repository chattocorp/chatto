import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { UserStore } from '@chatto/client/server/users';
import type { UserAvatarUserView } from '@chatto/client/timeline/users';
import MessageViewTestHarness from './MessageViewTestHarness.svelte';

const actor: UserAvatarUserView = {
  id: 'author',
  login: 'author',
  displayName: 'Message author',
  presenceStatus: PresenceStatus.OFFLINE,
  customStatus: { emoji: '🌴', text: 'On holiday' }
};
const props = { eventId: 'message', actor, displayName: 'Message author', body: 'Hello' };

afterEach(() => vi.useRealTimers());

describe('message author custom status', () => {
  it.each([false, true])(
    'shows an emoji with accessible hover text (clickable: %s)',
    async (clickable) => {
      const view = render(MessageViewTestHarness, {
        ...props,
        onActorClick: clickable ? vi.fn() : undefined
      });
      const badge = view.getByRole('img', { name: '🌴 On holiday' });
      await expect.element(badge).toBeVisible();
      await expect.element(badge).toHaveAttribute('title', '🌴 On holiday');
      await expect.element(badge).toHaveTextContent('🌴');
      await expect.element(view.getByText('On holiday', { exact: true })).not.toBeInTheDocument();
    }
  );

  it('follows live profile set, replacement, clear, and deletion without changing the message', async () => {
    const users = new UserStore();
    const view = render(MessageViewTestHarness, { ...props, users });
    const holiday = view.getByRole('img', { name: '🌴 On holiday' });
    const lunch = view.getByRole('img', { name: '🍜 Out for lunch' });
    const focus = view.getByRole('img', { name: '🎯 Focusing' });
    await expect.element(holiday).toBeVisible();

    users.set(actor.id, new DirectoryMember({ user: { id: actor.id } }));
    await expect.element(holiday).not.toBeInTheDocument();
    users.set(
      actor.id,
      new DirectoryMember({
        user: {
          id: actor.id,
          customStatus: { emoji: '🍜', text: 'chatto:status:out_for_lunch' }
        }
      })
    );
    await expect.element(lunch).toBeVisible();
    users.set(
      actor.id,
      new DirectoryMember({
        user: {
          id: actor.id,
          customStatus: { emoji: '🎯', text: 'Focusing' }
        }
      })
    );
    await expect.element(lunch).not.toBeInTheDocument();
    await expect.element(focus).toBeVisible();
    users.set(actor.id, new DirectoryMember({ user: { id: actor.id } }));
    await expect.element(focus).not.toBeInTheDocument();
    await expect.element(holiday).not.toBeInTheDocument();

    users.delete(actor.id);
    await expect.element(holiday).not.toBeInTheDocument();
    users.dispose();
  });

  it('hides a fallback status when it expires without a profile update', async () => {
    vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout'] });
    vi.setSystemTime(new Date('2026-10-02T12:00:00Z'));
    const view = render(MessageViewTestHarness, {
      ...props,
      actor: {
        ...actor,
        customStatus: {
          emoji: '🌴',
          text: 'On holiday',
          expiresAt: new Date(Date.now() + 1000).toISOString()
        }
      }
    });
    const badge = view.getByRole('img', { name: '🌴 On holiday' });
    await expect.element(badge).toBeVisible();
    await vi.advanceTimersByTimeAsync(1000);
    await expect.element(badge).not.toBeInTheDocument();
  });

  it.each([
    { compact: true },
    { actor: { ...actor, deleted: true } },
    { actor: null, missingActorIsDeleted: false },
    { actor: null, authorLoading: true },
    { actor: { ...actor, customStatus: null } }
  ])('omits status for a header without a visible active author status: %j', async (overrides) => {
    const view = render(MessageViewTestHarness, { ...props, ...overrides });
    await expect.element(view.getByRole('img', { name: '🌴 On holiday' })).not.toBeInTheDocument();
  });

  it('uses the next author status when a message row is reused', async () => {
    const view = render(MessageViewTestHarness, props);
    await expect.element(view.getByRole('img', { name: '🌴 On holiday' })).toBeVisible();
    await view.rerender({
      eventId: 'next-message',
      actor: { ...actor, id: 'next-author', customStatus: { emoji: '🎯', text: 'Focusing' } }
    });
    await expect.element(view.getByRole('img', { name: '🌴 On holiday' })).not.toBeInTheDocument();
    await expect.element(view.getByRole('img', { name: '🎯 Focusing' })).toBeVisible();
  });
});
