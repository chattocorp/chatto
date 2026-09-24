import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import '../../../app.css';
import AccountName from './AccountName.svelte';
import DirectMessageName from './DirectMessageName.svelte';
import UserIdentity from './UserIdentity.svelte';
import { formatAccountName } from '$lib/render/accountName';
import { buildDirectMessagePresentation } from '$lib/render/users';

describe('account names', () => {
  it.each([undefined, {}, { isBot: false }, { isBot: true, deleted: true }])(
    'does not mark a human, unknown, or deleted identity: %j',
    (identity) => {
      const view = render(AccountName, { name: 'Assistant', identity });
      expect(view.container.querySelector('[data-testid="bot-badge"]')).toBeNull();
      expect(formatAccountName('Assistant', identity)).toBe('Assistant');
    }
  );

  it('updates both the name and bot identity without remounting', async () => {
    const view = render(AccountName, { name: 'Alice', identity: { isBot: false } });
    await view.rerender({ name: 'Assistant', identity: { isBot: true } });
    flushSync();
    await expect.element(view.getByText('BOT', { exact: true })).toBeVisible();
    await expect.element(view.getByText('Assistant', { exact: true })).toBeVisible();
    expect(formatAccountName('Assistant', { isBot: true })).toBe('Assistant (BOT)');
    await view.rerender({ name: 'Deleted user', identity: { isBot: true, deleted: true } });
    expect(view.container.querySelector('[data-testid="bot-badge"]')).toBeNull();
  });

  it('keeps the badge visible while a long name truncates', () => {
    const view = render(AccountName, {
      name: 'A very long automated assistant display name',
      identity: { isBot: true },
      class: 'w-32'
    });
    const name = view.container.querySelector('bdi')!;
    const badge = view.container.querySelector('[data-testid="bot-badge"]')!;
    expect(name.scrollWidth).toBeGreaterThan(name.clientWidth);
    expect(badge.getBoundingClientRect().right).toBeLessThanOrEqual(
      name.parentElement!.getBoundingClientRect().right + 1
    );
  });

  it('marks each bot in mixed group DMs and preserves live names', () => {
    const participants = [
      { id: 'self', login: 'me', displayName: 'Me' },
      { id: 'human', login: 'alice', displayName: 'Alice' },
      { id: 'bot', login: 'helper_bot', displayName: 'Helper', isBot: true }
    ];
    const getDisplayName = (_id: string, fallback: string) => `Live ${fallback}`;
    const view = render(DirectMessageName, { participants, currentUserId: 'self', getDisplayName });
    expect(view.container.textContent).toContain('Live Alice');
    expect(view.container.textContent).toContain('Live Helper');
    expect(view.container.querySelectorAll('[data-testid="bot-badge"]')).toHaveLength(1);
    expect(buildDirectMessagePresentation(participants, 'self', 'You', getDisplayName).label).toBe(
      'Live Alice, Live Helper (BOT)'
    );
  });

  it('shows the live self-DM name with a separate, visible YOU badge', async () => {
    const self = { id: 'self', login: 'me', displayName: 'Original name' };
    const view = render(DirectMessageName, {
      participants: [self],
      currentUserId: 'self',
      getDisplayName: () => 'A long updated display name that must truncate'
    });
    view.container.firstElementChild?.classList.add('w-32');

    const name = view.container.querySelector('bdi')!;
    const badge = view.container.querySelector('[data-testid="you-badge"]')!;
    expect(name.textContent).toBe('A long updated display name that must truncate');
    expect(badge.textContent).toBe('You');
    expect(getComputedStyle(badge.firstElementChild!).textTransform).toBe('uppercase');
    expect(name.scrollWidth).toBeGreaterThan(name.clientWidth);
    expect(badge.getBoundingClientRect().right).toBeLessThanOrEqual(
      view.container.firstElementChild!.getBoundingClientRect().right + 1
    );

    await view.rerender({
      participants: [self],
      currentUserId: 'self',
      getDisplayName: () => 'New name'
    });
    expect(view.container.querySelector('bdi')?.textContent).toBe('New name');
    expect(view.container.querySelector('[data-testid="you-badge"]')).not.toBeNull();

    await view.rerender({ participants: [self], currentUserId: 'self', getDisplayName: () => '' });
    expect(view.container.querySelector('bdi')?.textContent).toBe('me');
  });

  it('does not show a YOU badge for another user who puts (You) in their name', () => {
    const view = render(DirectMessageName, {
      participants: [
        { id: 'self', login: 'me', displayName: 'Me' },
        { id: 'other', login: 'other', displayName: 'Alice (You)' }
      ],
      currentUserId: 'self'
    });
    expect(view.container.querySelector('bdi')?.textContent).toBe('Alice (You)');
    expect(view.container.querySelector('[data-testid="you-badge"]')).toBeNull();
  });

  it('shows the current-user fallback when self-DM participant data is missing', () => {
    const view = render(DirectMessageName, { participants: [], currentUserId: 'self' });
    expect(view.container.textContent).toBe('You');
    expect(view.container.querySelector('[data-testid="you-badge"]')).toBeNull();
  });

  it('marks a one-to-one DM and a profile identity beside the name', () => {
    const user = {
      id: 'bot',
      login: 'helper_bot',
      displayName: 'Helper',
      isBot: true,
      deleted: false,
      presenceStatus: PresenceStatus.OFFLINE
    };
    const dm = render(DirectMessageName, { participants: [user], currentUserId: 'self' });
    expect(
      dm.container.querySelector('[data-testid="bot-badge"]')?.previousElementSibling?.textContent
    ).toBe('Helper');
    const profile = render(UserIdentity, { user });
    expect(
      profile.container.querySelector('[data-testid="bot-badge"]')?.previousElementSibling
        ?.textContent
    ).toBe('Helper');
    expect(
      profile.container
        .querySelector('[aria-label="helper_bot"]')
        ?.querySelector('[data-testid="bot-badge"]')
    ).toBeNull();
  });
});
