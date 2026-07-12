import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { RoomEventView } from '$lib/render/types';
import { RoomEventKind } from '$lib/render/eventKinds';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import SystemEventGroup from './SystemEventGroup.svelte';

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_scope: { serverId: string; userId: string }, fallback: unknown) => fallback
  })
}));

function systemEvents(actorNames: string[]): RoomEventView[] {
  return actorNames.map(
    (actorName, index) =>
      ({
        id: `event-${index}`,
        createdAt: `2026-06-15T12:00:0${index}Z`,
        actorId: `user-${index}`,
        actor: {
          id: `user-${index}`,
          login: actorName.toLowerCase(),
          displayName: actorName,
          avatarUrl: null,
          presenceStatus: null
        },
        event: {
          kind: RoomEventKind.UserJoinedRoom,
          roomId: 'room-1'
        }
      }) as unknown as RoomEventView
  );
}

function renderedCopy(container: HTMLElement): string {
  const copy = container.querySelector<HTMLElement>('[data-event-id] > span');
  return copy?.textContent?.replace(/\s+/g, ' ').trim() ?? '';
}

describe('SystemEventGroup', () => {
  beforeEach(async () => {
    await loadLocaleMessages('en');
    setReactiveLocale('en');
  });

  it('separates two actors with the localized conjunction', () => {
    const { container } = render(SystemEventGroup, {
      props: { events: systemEvents(['Alice', 'Bob']), kind: 'join' }
    });

    expect(renderedCopy(container)).toBe('Alice and Bob joined the room');
  });

  it('uses comma-separated formatting for three actors', () => {
    const { container } = render(SystemEventGroup, {
      props: { events: systemEvents(['Alice', 'Bob', 'Charlie']), kind: 'join' }
    });

    expect(renderedCopy(container)).toBe('Alice, Bob, and Charlie joined the room');
  });

  it('renders grouped leave wording', () => {
    const { container } = render(SystemEventGroup, {
      props: { events: systemEvents(['Alice', 'Bob']), kind: 'leave' }
    });

    expect(renderedCopy(container)).toBe('Alice and Bob left the room');
  });

  it('localizes the conjunction and plural action wording in German', async () => {
    await loadLocaleMessages('de');
    setReactiveLocale('de');
    const { container } = render(SystemEventGroup, {
      props: { events: systemEvents(['Alice', 'Bob']), kind: 'join' }
    });

    expect(renderedCopy(container)).toBe('Alice und Bob sind dem Raum beigetreten');
  });
});
