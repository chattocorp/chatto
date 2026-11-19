import { afterEach, describe, expect, it, vi } from 'vitest';
import { createRawSnippet, flushSync } from 'svelte';
import { experimentalFeatures } from '$lib/state/experimentalFeatures.svelte';
import '../../../app.css';
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

it('shows the login between the display name and timestamp when the experiment is enabled', async () => {
  const headerMeta = createRawSnippet(() => ({
    render: () => '<time class="text-xs">12:34</time>'
  }));
  const view = render(MessageViewTestHarness, { ...props, headerMeta });
  const author = view.getByText(props.displayName, { exact: true });
  const time = view.getByText('12:34');
  await expect.element(author).toBeVisible();
  const login = view.getByText('@author', { exact: true });
  await expect.element(login).not.toBeInTheDocument();
  try {
    flushSync(() => (experimentalFeatures.newMessageHeader.enabled = true));
    await expect.element(login).toBeVisible();
    await expect
      .element(view.getByText('·', { exact: true }))
      .toHaveAttribute('aria-hidden', 'true');
    expect(getComputedStyle(time.element()).fontSize).toBe(
      getComputedStyle(author.element()).fontSize
    );
    const nameBounds = author.element().getBoundingClientRect();
    const loginBounds = login.element().getBoundingClientRect();
    const timeBounds = time.element().getBoundingClientRect();
    expect(loginBounds.left).toBeGreaterThanOrEqual(nameBounds.right);
    expect(timeBounds.left).toBeGreaterThanOrEqual(loginBounds.right);
    expect(timeBounds.top).toBeLessThan(nameBounds.bottom);
  } finally {
    flushSync(() => (experimentalFeatures.newMessageHeader.enabled = false));
  }
  await expect.element(login).not.toBeInTheDocument();
});

it('truncates the username first and keeps the timestamp on the header line', async () => {
  const displayName = 'Alexandra Montgomery-Westfield';
  const login = 'alexandra.westfield';
  const headerMeta = createRawSnippet(() => ({ render: () => '<time>12:11 PM</time>' }));
  const view = render(MessageViewTestHarness, {
    ...props,
    actor: { ...actor, login, displayName, customStatus: undefined },
    displayName,
    headerMeta
  });
  try {
    flushSync(() => (experimentalFeatures.newMessageHeader.enabled = true));
    const name = view.getByText(displayName, { exact: true }).element();
    const handle = view.getByText(`@${login}`, { exact: true }).element();
    const dot = view.getByText('·', { exact: true }).element();
    const time = view.getByText('12:11 PM').element();
    const row = view.getByRole('article').element() as HTMLElement;
    for (const width of [520, 390, 320, 520]) {
      row.style.width = `${width}px`;
      if (width === 520) {
        await expect.element(handle).toBeVisible();
        expect(handle.scrollWidth).toBeGreaterThan(handle.clientWidth);
        expect(name.scrollWidth).toBe(name.clientWidth);
      } else {
        await expect.element(handle).not.toBeVisible();
        expect(name.getBoundingClientRect().right).toBeCloseTo(
          name.closest('strong')!.getBoundingClientRect().right,
          0
        );
      }
      const bounds = name.getBoundingClientRect();
      for (const item of [dot, time]) {
        const rect = item.getBoundingClientRect();
        expect(rect.top).toBeLessThan(bounds.bottom);
        expect(rect.bottom).toBeGreaterThan(bounds.top);
        expect(rect.right).toBeLessThanOrEqual(row.getBoundingClientRect().right);
      }
    }
  } finally {
    flushSync(() => (experimentalFeatures.newMessageHeader.enabled = false));
  }
});

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
