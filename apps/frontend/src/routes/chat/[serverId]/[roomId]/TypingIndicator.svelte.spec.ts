import { render } from 'vitest-browser-svelte';
import { describe, expect, it } from 'vitest';
import { q } from '$lib/test-utils';
import TypingIndicator from './TypingIndicator.svelte';
import type { RoomMember } from '$lib/state/room';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

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
  return q(container, '[data-testid="typing-indicator"] .typing-label')?.textContent ?? undefined;
}

describe('TypingIndicator', () => {
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
      container.querySelectorAll('[data-testid="typing-indicator"] span[aria-hidden]:not(.typing-dots)')
      .length
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
    const region = q(container, '[data-testid="typing-indicator"]');
    await expect.element(region).toHaveAttribute('role', 'status');
    await expect.element(region).toHaveAttribute('aria-live', 'polite');
  });
});
