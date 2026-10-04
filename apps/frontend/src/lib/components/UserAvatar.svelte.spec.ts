import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import '../../app.css';

import { q } from '$lib/test-utils';
import UserAvatar from './UserAvatar.svelte';
import UserAvatarTestHarness from './UserAvatarTestHarness.svelte';

function computedBackgroundColor(color: string): string {
  const element = document.createElement('span');
  element.style.backgroundColor = color;
  document.body.append(element);
  const computed = window.getComputedStyle(element).backgroundColor;
  element.remove();
  return computed;
}

describe('UserAvatar', () => {
  it('updates Unicode labels', async () => {
    const user = {
      id: 'user-1',
      login: 'alice',
      displayName: '[DEV] Alice',
      avatarUrl: null,
      presenceStatus: PresenceStatus.OFFLINE
    };
    const view = render(UserAvatar, { props: { user, useLiveProfile: false } });
    const avatar = q(view.container, '[aria-label="alice"]')!;
    expect(avatar.textContent?.trim()).toBe('DA');
    await view.rerender({ user: { ...user, displayName: '👩‍💻' } });
    await expect.element(view.getByRole('img', { name: 'alice' })).toHaveTextContent('👩‍💻');
    await view.rerender({ user: { ...user, displayName: '!!!' } });
    await expect.element(view.getByRole('img', { name: 'alice' })).toHaveTextContent('A');
  });

  it('uses a generic icon when there is no label and keeps deleted users neutral', async () => {
    const user = {
      id: 'missing-name',
      login: '',
      displayName: '!!!',
      avatarUrl: null,
      presenceStatus: PresenceStatus.OFFLINE,
      deleted: false
    };
    const view = render(UserAvatar, { props: { user, useLiveProfile: false } });
    expect(
      q(view.container, 'span[aria-hidden="true"]')?.classList.contains('icon-[uil--user]')
    ).toBe(true);
    await view.rerender({ user: { ...user, deleted: true } });
    expect(q(view.container, '.bg-surface-emphasized')).toBeTruthy();
    expect(
      q(view.container, 'span[aria-hidden="true"]')?.classList.contains('icon-[uil--user-times]')
    ).toBe(true);
  });

  it('uses the same neutral surface and muted label for different accounts in both themes', async () => {
    const root = document.documentElement;
    const previousTheme = root.getAttribute('data-theme');
    const user = {
      id: 'a',
      login: 'avatar',
      displayName: 'Avatar',
      avatarUrl: null,
      presenceStatus: PresenceStatus.OFFLINE
    };
    const view = render(UserAvatar, { props: { user, useLiveProfile: false } });
    try {
      for (const theme of ['light', 'dark']) {
        root.setAttribute('data-theme', theme);
        for (const id of ['a', 'b']) {
          await view.rerender({ user: { ...user, id } });
          const avatar = q(view.container, '[aria-label="avatar"]')!;
          const style = getComputedStyle(avatar);
          const tokens = getComputedStyle(root);
          expect(style.backgroundColor).toBe(
            computedBackgroundColor(tokens.getPropertyValue('--color-surface-emphasized'))
          );
          expect(style.color).toBe(
            computedBackgroundColor(tokens.getPropertyValue('--color-muted'))
          );
        }
      }
    } finally {
      if (previousTheme === null) root.removeAttribute('data-theme');
      else root.setAttribute('data-theme', previousTheme);
    }
  });

  it('renders medium placeholder avatars with a subtle inset ring', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'md' });
    const avatar = q(container, '[aria-label="alice"]')!;

    expect(avatar.className).toContain('rounded-full');
    expect(avatar.className).toContain('ring-1');
    expect(avatar.className).toContain('ring-inset');
    expect(avatar.className).toContain('ring-muted/15');
    expect(q(container, '[aria-label="🍜 Out for lunch"]')).toBeFalsy();
    expect(q(container, '[aria-label="Online"]')).toBeFalsy();
  });

  it('shows custom status badges when requested', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'sm', showStatus: true });

    expect(q(container, '[aria-label="🍜 Out for lunch"]')).toBeTruthy();
  });

  it('does not show presence dots on small avatars by default', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'sm' });
    const avatar = q(container, '[aria-label="alice"]')!;

    expect(avatar.className).toContain('rounded-full');
    expect(q(container, '[aria-label="Online"]')).toBeFalsy();
  });

  it('shows presence dots on small avatars when explicitly requested', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'sm', showPresence: true });
    const presenceDot = q(container, '[aria-label="Online"] span')!;

    expect(presenceDot.className).toContain('bg-presence-online');
  });

  it('shows presence dots on medium avatars when explicitly requested', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'md', showPresence: true });
    const presenceDot = q(container, '[aria-label="Online"] span')!;

    expect(presenceDot.className).toContain('bg-presence-online');
  });

  it('renders away presence dots in the semantic warm-gold tone', () => {
    const { container } = render(UserAvatarTestHarness, {
      size: 'md',
      showPresence: true,
      presenceStatus: PresenceStatus.AWAY
    });
    const presenceDot = q(container, '[aria-label="Away"] span')!;
    const presenceAway = window
      .getComputedStyle(document.documentElement)
      .getPropertyValue('--color-presence-away')
      .trim();

    expect(presenceDot.className).toContain('bg-presence-away');
    expect(window.getComputedStyle(presenceDot).backgroundColor).toBe(
      computedBackgroundColor(presenceAway)
    );
  });

  it('keeps the offline presence dot for a person', () => {
    const { container } = render(UserAvatarTestHarness, {
      size: 'sm',
      showPresence: true,
      presenceStatus: PresenceStatus.OFFLINE
    });

    expect(q(container, '[aria-label="Offline"] [data-testid="presence-dot"]')).toBeTruthy();
  });

  it.each([PresenceStatus.OFFLINE, PresenceStatus.UNSPECIFIED])(
    'hides the presence dot for a bot without available presence (%s)',
    (presenceStatus) => {
      const { container } = render(UserAvatarTestHarness, {
        size: 'sm',
        showPresence: true,
        isBot: true,
        presenceStatus
      });

      expect(q(container, '[data-testid="presence-dot"]')).toBeFalsy();
    }
  );

  it.each([
    [PresenceStatus.ONLINE, 'Online'],
    [PresenceStatus.AWAY, 'Away'],
    [PresenceStatus.DO_NOT_DISTURB, 'Do not disturb']
  ] as const)('shows a bot presence dot for %s', (presenceStatus, label) => {
    const { container } = render(UserAvatarTestHarness, {
      size: 'sm',
      showPresence: true,
      isBot: true,
      presenceStatus
    });

    expect(q(container, `[aria-label="${label}"] [data-testid="presence-dot"]`)).toBeTruthy();
  });

  it('shows the current presence instead of the presence in the user profile', () => {
    const { container } = render(UserAvatarTestHarness, {
      size: 'sm',
      showPresence: true,
      presenceStatus: PresenceStatus.ONLINE,
      presence: PresenceStatus.DO_NOT_DISTURB
    });

    expect(q(container, '[aria-label="Do not disturb"] [data-testid="presence-dot"]')).toBeTruthy();
    expect(q(container, '[aria-label="Online"]')).toBeFalsy();
  });

  it('updates when the current presence changes', async () => {
    const view = render(UserAvatarTestHarness, {
      size: 'sm',
      showPresence: true,
      presenceStatus: PresenceStatus.ONLINE,
      presence: PresenceStatus.AWAY
    });
    expect(q(view.container, '[aria-label="Away"]')).toBeTruthy();

    await view.rerender({ presence: PresenceStatus.OFFLINE });

    expect(q(view.container, '[aria-label="Offline"]')).toBeTruthy();
    expect(q(view.container, '[aria-label="Away"]')).toBeFalsy();
  });

  it('keeps extra-small avatars free of presence overlays', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'xs', showPresence: true });

    expect(q(container, '[aria-label="Online"]')).toBeFalsy();
  });

  it('keeps extra-small bot avatars free of robot badges', () => {
    const { container } = render(UserAvatarTestHarness, { size: 'xs', isBot: true });

    expect(q(container, '[aria-label="alice"]')).toBeTruthy();
    expect(q(container, '[data-testid="bot-badge"]')).toBeFalsy();
  });

  it.each(['sm', 'md', 'message', 'lg', 'xl'] as const)(
    'keeps %s bot avatars free of robot badges',
    (size) => {
      const { container } = render(UserAvatarTestHarness, {
        size,
        isBot: true
      });

      expect(q(container, '[data-testid="bot-badge"]')).toBeFalsy();
    }
  );

  it('renders static directory identities without app-level live caches', () => {
    const { container } = render(UserAvatar, {
      props: {
        user: {
          id: 'bot-1',
          login: 'helper_bot',
          displayName: 'Helper Bot',
          deleted: false,
          isBot: true,
          avatarUrl: null,
          presenceStatus: PresenceStatus.OFFLINE
        },
        size: 'sm',
        useLiveProfile: false
      }
    });

    expect(q(container, '[data-testid="bot-badge"]')).toBeFalsy();
  });

  it('uses a complete emoji when an avatar image fails', async () => {
    const view = render(UserAvatar, {
      props: {
        user: {
          id: 'user-1',
          login: 'alice',
          displayName: '👩‍💻',
          avatarUrl: '/missing-avatar.png',
          presenceStatus: PresenceStatus.OFFLINE
        },
        useLiveProfile: false
      }
    });

    const image = q(view.container, 'img[alt="alice"]');
    expect(q(view.container, '.bg-surface-emphasized')).toBeNull();
    expect(view.container.querySelector('.skeleton')).toBeNull();
    image?.dispatchEvent(new Event('error'));
    await expect.element(view.getByRole('img', { name: 'alice' })).toHaveTextContent('👩‍💻');
    expect(q(view.container, '.bg-surface-emphasized.text-muted')).toBeTruthy();
  });
});
