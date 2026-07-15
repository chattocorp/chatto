import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { RoomEventView } from '$lib/render/types';
import { RoomEventKind } from '$lib/render/eventKinds';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import SystemEvent from './SystemEvent.svelte';

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

function systemEvent(
  kind:
    | typeof RoomEventKind.UserJoinedRoom
    | typeof RoomEventKind.UserLeftRoom
    | typeof RoomEventKind.RoomArchived,
  actorName = 'Alice'
): RoomEventView {
  return {
    id: `evt-${kind}`,
    createdAt: '2026-06-15T12:00:00Z',
    actorId: 'user-1',
    actor: {
      id: 'user-1',
      login: 'alice',
      displayName: actorName,
      avatarUrl: null,
      presenceStatus: null
    },
    event: {
      kind,
      roomId: 'room-1'
    }
  } as unknown as RoomEventView;
}

describe('SystemEvent', () => {
  beforeEach(async () => {
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('renders member join copy with the actor name', () => {
    const { container } = render(SystemEvent, {
      props: { event: systemEvent(RoomEventKind.UserJoinedRoom, 'Alice') }
    });

    expect(container.textContent).toContain('Alice joined the room');
  });

  it('renders member leave copy with the actor name', () => {
    const { container } = render(SystemEvent, {
      props: { event: systemEvent(RoomEventKind.UserLeftRoom, 'Alice') }
    });

    expect(container.textContent).toContain('Alice left the room');
  });

  it.each([RoomEventKind.UserJoinedRoom, RoomEventKind.UserLeftRoom])(
    'does not render a missing actor for %s events',
    (kind) => {
      const event = systemEvent(kind);
      event.actor = null;

      const { container } = render(SystemEvent, { props: { event } });

      expect(container.querySelector('[data-event-id]')).toBeNull();
    }
  );

  it('does not render an actor marked as deleted', () => {
    const event = systemEvent(RoomEventKind.UserJoinedRoom);
    if (event.actor) event.actor.deleted = true;

    const { container } = render(SystemEvent, { props: { event } });

    expect(container.querySelector('[data-event-id]')).toBeNull();
  });

  it('preserves deleted-user placeholders for other system event types', () => {
    const event = systemEvent(RoomEventKind.RoomArchived);
    if (event.actor) event.actor.deleted = true;

    const { container } = render(SystemEvent, { props: { event } });

    expect(container.textContent).toContain('[deleted user] archived the room');
  });

  it('localizes event copy in German', async () => {
    await loadLocaleMessages('de-DE');
    setReactiveLocale('de-DE');
    const event = systemEvent(RoomEventKind.UserJoinedRoom, 'Alice');

    const { container } = render(SystemEvent, { props: { event } });

    expect(container.textContent).toContain('Alice ist dem Raum beigetreten');
  });
});
