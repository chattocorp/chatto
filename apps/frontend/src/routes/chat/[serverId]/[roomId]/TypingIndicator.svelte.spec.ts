import { render } from 'vitest-browser-svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { q } from '$lib/test-utils';
import TypingIndicator from './TypingIndicator.svelte';
import type { RoomMember } from '$lib/state/room';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { UserStore } from '$lib/state/server/users.svelte';

function member(id: string, displayName: string): RoomMember {
  return {
    id,
    login: id,
    displayName,
    presenceStatus: PresenceStatus.ONLINE
  };
}

const members = [member('alice', 'Alice'), member('bob', 'Bob'), member('carol', 'Carol')];

function indicatorText(container: HTMLElement): string | undefined {
  return q(container, '.typing-label')?.textContent ?? undefined;
}

afterEach(async () => {
  await loadLocaleMessages('en-GB');
});

describe('TypingIndicator', () => {
  it('shows the shared badge beside a bot in the typing label', () => {
    const { container } = render(TypingIndicator, {
      props: { typingUserIds: ['helper'], members: [{ ...member('helper', 'Helper'), isBot: true }] }
    });
    expect(q(container, '.typing-label [data-testid="bot-badge"]')?.previousElementSibling?.textContent).toBe('Helper');
    expect(indicatorText(container)).toBe('HelperBOT is typing');
    expect(indicatorText(container)).not.toContain('(BOT)');
  });

  it('keeps badges with both named bots when it aggregates typers', () => {
    const { container } = render(TypingIndicator, {
      props: {
        typingUserIds: ['helper', 'assistant', 'alice'],
        members: [
          { ...member('helper', 'Helper'), isBot: true },
          { ...member('assistant', 'Assistant'), isBot: true },
          members[0]
        ]
      }
    });
    expect(container.querySelectorAll('.typing-label [data-testid="bot-badge"]')).toHaveLength(2);
    expect(indicatorText(container)).toContain('1 other');
  });
  it('renders nothing when nobody is typing', () => {
    const { container } = render(TypingIndicator, { props: { typingUserIds: [], members } });
    expect(q(container, '[data-testid="typing-indicator"]')).toBeNull();
  });

  it('names a single typer', () => {
    const { container } = render(TypingIndicator, {
      props: { typingUserIds: ['alice'], members }
    });
    expect(indicatorText(container)).toContain('Alice');
    expect(
      container.querySelectorAll(
        '[data-testid="typing-indicator"] span[aria-hidden]:not(.typing-dots)'
      ).length
    ).toBe(1);
  });

  it('names two typers', () => {
    const { container } = render(TypingIndicator, {
      props: { typingUserIds: ['alice', 'bob'], members }
    });
    const text = indicatorText(container);
    expect(text).toContain('Alice');
    expect(text).toContain('Bob');
  });

  it('aggregates large groups instead of listing every name', () => {
    const { container } = render(TypingIndicator, {
      props: { typingUserIds: ['alice', 'bob', 'carol'], members }
    });
    const text = indicatorText(container) ?? '';
    expect(text).toContain('Alice');
    expect(text).not.toContain('Carol');
  });

  it('caps the number of visible avatars', () => {
    const extra = member('dave', 'Dave');
    const { container } = render(TypingIndicator, {
      props: { typingUserIds: ['alice', 'bob', 'carol', 'dave'], members: [...members, extra] }
    });
    // Avatar wrappers are aria-hidden spans; one per visible typer (the dots
    // cluster is also aria-hidden but carries its own class).
    const avatars = container.querySelectorAll(
      '[data-testid="typing-indicator"] span[aria-hidden]:not(.typing-dots)'
    );
    expect(avatars.length).toBeLessThanOrEqual(3);
  });

  it('exposes an accessible live region', async () => {
    const { container } = render(TypingIndicator, {
      props: { typingUserIds: ['alice'], members }
    });
    const region = q(container, '[role="status"]');
    await expect.element(region).toHaveAttribute('role', 'status');
    await expect.element(region).toHaveAttribute('aria-live', 'polite');
    await expect.element(region).toHaveAttribute('aria-atomic', 'true');
  });

  it('counts missing profiles without dropping typers or exposing their IDs', () => {
    const { container } = render(TypingIndicator, {
      props: { typingUserIds: ['alice', 'missing-profile', 'carol'], members: [members[0]] }
    });
    expect(indicatorText(container)).toBe('Alice, Unknown user and 1 other are typing');
    expect(container.textContent).not.toContain('missing-profile');
  });

  it('labels unknown typers and falls back to a known login when a display name is empty', () => {
    const { container } = render(TypingIndicator, {
      props: { typingUserIds: ['ghost', 'bob'], members: [member('bob', '')] }
    });
    expect(indicatorText(container)).toBe('Unknown user and bob are typing');
  });

  it('replaces an unknown typer with a late shared profile before membership loads', async () => {
    const profiles = new UserStore();
    const { container } = render(TypingIndicator, {
      props: { typingUserIds: ['late'], members: [], profiles }
    });
    expect(indicatorText(container)).toBe('Unknown user is typing');

    profiles.set('late', new DirectoryMember({
      user: { id: 'late', login: 'late', displayName: 'Late Member', bot: {} }
    }));

    await expect.poll(() => indicatorText(container)).toBe('Late MemberBOT is typing');
    expect(container.querySelectorAll('[data-testid="typing-avatar"]')).toHaveLength(1);
    expect(container.querySelectorAll('.typing-label [data-testid="bot-badge"]')).toHaveLength(1);
    profiles.dispose();
  });

  it('uses the shared profile when a member row has an older name', () => {
    const profiles = new UserStore();
    profiles.set('alice', new DirectoryMember({
      user: { id: 'alice', login: 'alice', displayName: 'New Alice' }
    }));
    const { container } = render(TypingIndicator, {
      props: { typingUserIds: ['alice'], members, profiles }
    });
    expect(indicatorText(container)).toBe('New Alice is typing');
    profiles.dispose();
  });

  it('counts distinct people only', () => {
    const { container } = render(TypingIndicator, {
      props: { typingUserIds: ['alice', 'alice', 'bob'], members }
    });
    expect(indicatorText(container)).toBe('Alice and Bob are typing');
    expect(container.querySelectorAll('[data-testid="typing-avatar"]')).toHaveLength(2);
  });

  it('keeps one live region through start, group changes, and stop', async () => {
    const view = render(TypingIndicator, { props: { typingUserIds: [], members } });
    const region = q(view.container, '[role="status"]');
    expect(region?.textContent).toBe('');
    await view.rerender({ typingUserIds: ['alice'] });
    expect(indicatorText(view.container)).toBe('Alice is typing');
    await view.rerender({ typingUserIds: ['alice', 'bob', 'carol'] });
    expect(indicatorText(view.container)).toBe('Alice, Bob and 1 other are typing');
    await view.rerender({ typingUserIds: [] });
    await expect
      .element(q(view.container, '[data-testid="typing-indicator"]'))
      .not.toBeInTheDocument();
    expect(q(view.container, '[role="status"]')).toBe(region);
  });

  it('updates labels when the locale changes', async () => {
    const { container } = render(TypingIndicator, {
      props: { typingUserIds: ['alice', 'bob', 'carol'], members }
    });
    await loadLocaleMessages('de-DE');
    expect(indicatorText(container)).toBe('Alice, Bob und 1 weitere Person schreiben');
  });

  it('isolates names with mixed text directions', () => {
    const { container } = render(TypingIndicator, {
      props: { typingUserIds: ['alice', 'bob'], members: [member('alice', 'علي'), members[1]] }
    });
    expect(container.querySelectorAll('.typing-label bdi')).toHaveLength(2);
    expect(indicatorText(container)).toBe('علي and Bob are typing');
  });

  it('fits long labels inside a narrow pane in both directions', async () => {
    await page.viewport(320, 600);
    const { container } = render(TypingIndicator, {
      props: {
        typingUserIds: ['alice', 'bob', 'carol'],
        members: [member('alice', 'A very long display name '.repeat(8)), ...members.slice(1)]
      }
    });
    container.style.cssText = 'position:relative;width:240px;height:80px';
    for (const direction of ['ltr', 'rtl']) {
      container.dir = direction;
      const bounds = container.getBoundingClientRect();
      const indicator = q(container, '[data-testid="typing-indicator"]')!.getBoundingClientRect();
      expect(indicator.left).toBeGreaterThanOrEqual(bounds.left);
      expect(indicator.right).toBeLessThanOrEqual(bounds.right);
    }
  });
});
