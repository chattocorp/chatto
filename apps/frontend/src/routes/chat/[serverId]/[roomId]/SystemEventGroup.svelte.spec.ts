import { beforeEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
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
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('separates two actors with the localized conjunction', () => {
    const { container } = render(SystemEventGroup, {
      props: {
        events: systemEvents(['Alice', 'Bob']),
        kind: 'join',
        expanded: false,
        onExpandedChange: vi.fn()
      }
    });

    expect(renderedCopy(container)).toBe('Alice and Bob joined the room');
  });

  it('uses comma-separated formatting for three actors', () => {
    const { container } = render(SystemEventGroup, {
      props: {
        events: systemEvents(['Alice', 'Bob', 'Charlie']),
        kind: 'join',
        expanded: false,
        onExpandedChange: vi.fn()
      }
    });

    expect(renderedCopy(container)).toBe('Alice, Bob, and Charlie joined the room');
  });

  it('renders grouped leave wording', () => {
    const { container } = render(SystemEventGroup, {
      props: {
        events: systemEvents(['Alice', 'Bob']),
        kind: 'leave',
        expanded: false,
        onExpandedChange: vi.fn()
      }
    });

    expect(renderedCopy(container)).toBe('Alice and Bob left the room');
  });

  it('filters deleted actors before formatting names and pluralization', () => {
    const events = systemEvents(['Alice', 'Deleted User']);
    if (events[1].actor) events[1].actor.deleted = true;

    const { container } = render(SystemEventGroup, {
      props: {
        events,
        kind: 'join',
        expanded: false,
        onExpandedChange: vi.fn()
      }
    });

    expect(renderedCopy(container)).toBe('Alice joined the room');
    expect(container.querySelector('[aria-label="alice"]')).not.toBeNull();
    expect(container.textContent).not.toContain('Deleted User');
  });

  it('omits missing actors from mixed groups', () => {
    const events = systemEvents(['Alice', 'Deleted User', 'Bob']);
    events[1].actor = null;

    const { container } = render(SystemEventGroup, {
      props: {
        events,
        kind: 'join',
        expanded: false,
        onExpandedChange: vi.fn()
      }
    });

    expect(renderedCopy(container)).toBe('Alice and Bob joined the room');
    expect(container.textContent).not.toContain('[deleted user]');
  });

  it('renders no row when every actor is deleted or missing', () => {
    const events = systemEvents(['Deleted User', 'Missing User']);
    if (events[0].actor) events[0].actor.deleted = true;
    events[1].actor = null;

    const { container } = render(SystemEventGroup, {
      props: {
        events,
        kind: 'leave',
        expanded: false,
        onExpandedChange: vi.fn()
      }
    });

    expect(container.querySelector('[data-event-id]')).toBeNull();
  });

  it('localizes the conjunction and plural action wording in German', async () => {
    await loadLocaleMessages('de-DE');
    setReactiveLocale('de-DE');
    const { container } = render(SystemEventGroup, {
      props: {
        events: systemEvents(['Alice', 'Bob']),
        kind: 'join',
        expanded: false,
        onExpandedChange: vi.fn()
      }
    });

    expect(renderedCopy(container)).toBe('Alice und Bob sind dem Raum beigetreten');
  });

  it('reports expansion changes through its controlled interface', async () => {
    const onExpandedChange = vi.fn();
    const events = systemEvents(['Alice', 'Bob', 'Charlie', 'Dora', 'Eve']);
    const rendered = render(SystemEventGroup, {
      props: { events, kind: 'join', expanded: false, onExpandedChange }
    });

    await page.getByRole('button', { name: '2 others' }).click();
    expect(onExpandedChange).toHaveBeenCalledExactlyOnceWith(true);

    await rendered.rerender({ events, kind: 'join', expanded: true, onExpandedChange });
    expect(renderedCopy(rendered.container)).toContain('Eve');
    await page.getByRole('button', { name: 'show less' }).click();
    expect(onExpandedChange).toHaveBeenLastCalledWith(false);
  });
});
