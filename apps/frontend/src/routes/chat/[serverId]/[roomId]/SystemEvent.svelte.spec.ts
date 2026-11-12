import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { TimelineEventKind, type TimelineEventView } from '@chatto/client/timeline/timelineEvents';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import SystemEvent from './SystemEvent.svelte';
import { RoomThreadingMode } from '@chatto/client/util/roomThreading';

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveBotOwnerUserId: (_userId: string, fallback: string | null) => fallback,
  getLiveBio: () => null,
  getLiveTimezone: () => null,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

function systemEvent(
  kind:
    | typeof TimelineEventKind.UserJoinedRoom
    | typeof TimelineEventKind.UserLeftRoom
    | typeof TimelineEventKind.RoomArchived
    | typeof TimelineEventKind.CallStarted
    | typeof TimelineEventKind.CallEnded,
  actorName = 'Alice'
): TimelineEventView {
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
      roomId: 'room-1',
      callId:
        kind === TimelineEventKind.CallStarted || kind === TimelineEventKind.CallEnded
          ? 'call-1'
          : undefined
    }
  } as unknown as TimelineEventView;
}

describe('SystemEvent', () => {
  beforeEach(async () => {
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('renders member join copy with the actor name', () => {
    const { container } = render(SystemEvent, {
      props: { event: systemEvent(TimelineEventKind.UserJoinedRoom, 'Alice') }
    });

    expect(container.textContent).toContain('Alice joined the room');
  });

  it('renders member leave copy with the actor name', () => {
    const { container } = render(SystemEvent, {
      props: { event: systemEvent(TimelineEventKind.UserLeftRoom, 'Alice') }
    });

    expect(container.textContent).toContain('Alice left the room');
  });

  it('renders an actor-attributed threading mode change', () => {
    const event = systemEvent(TimelineEventKind.RoomArchived, 'Alice');
    event.event = {
      kind: TimelineEventKind.RoomThreadingModeChanged,
      roomId: 'room-1',
      threadingMode: RoomThreadingMode.ENCOURAGED
    };

    const { container } = render(SystemEvent, { props: { event } });

    expect(container.textContent).toContain('Alice changed threading mode to Encouraged');
  });

  it('renders an unknown threading mode as Disabled', () => {
    const event = systemEvent(TimelineEventKind.RoomArchived, 'Alice');
    event.event = {
      kind: TimelineEventKind.RoomThreadingModeChanged,
      roomId: 'room-1',
      threadingMode: 99 as RoomThreadingMode
    };

    const { container } = render(SystemEvent, { props: { event } });

    expect(container.textContent).toContain('Alice changed threading mode to Disabled');
  });

  it('renders an actionable call-start event while its call is active', async () => {
    const onOpenCall = vi.fn();
    const { container } = render(SystemEvent, {
      props: {
        event: systemEvent(TimelineEventKind.CallStarted, 'Alice'),
        activeCallId: 'call-1',
        onOpenCall
      }
    });

    expect(container.textContent).toContain('Alice started a call in this room');
    expect(
      container.querySelector('button')?.parentElement?.textContent?.replace(/\s+/g, ' ').trim()
    ).toBe('Alice started a call in this room · Join call');
    await page.getByRole('button', { name: 'Join call' }).click();
    expect(onOpenCall).toHaveBeenCalledOnce();
  });

  it('removes the obsolete call action after the call ends', () => {
    const { container } = render(SystemEvent, {
      props: {
        event: systemEvent(TimelineEventKind.CallStarted, 'Alice'),
        activeCallId: null,
        onOpenCall: vi.fn()
      }
    });

    expect(container.textContent).toContain('Alice started a call in this room');
    expect(container.querySelector('button')).toBeNull();
  });

  it('renders call-end copy without relying on the event actor', () => {
    const event = systemEvent(TimelineEventKind.CallEnded);
    event.actor = null;

    const { container } = render(SystemEvent, { props: { event } });

    expect(container.textContent).toContain('The active call has ended');
  });

  it.each([TimelineEventKind.UserJoinedRoom, TimelineEventKind.UserLeftRoom])(
    'does not render a missing actor for %s events',
    (kind) => {
      const event = systemEvent(kind);
      event.actor = null;

      const { container } = render(SystemEvent, { props: { event } });

      expect(container.querySelector('[data-event-id]')).toBeNull();
    }
  );

  it('does not render an actor marked as deleted', () => {
    const event = systemEvent(TimelineEventKind.UserJoinedRoom);
    if (event.actor) event.actor.deleted = true;

    const { container } = render(SystemEvent, { props: { event } });

    expect(container.querySelector('[data-event-id]')).toBeNull();
  });

  it('preserves deleted-user placeholders for other system event types', () => {
    const event = systemEvent(TimelineEventKind.RoomArchived);
    if (event.actor) event.actor.deleted = true;

    const { container } = render(SystemEvent, { props: { event } });

    expect(container.textContent).toContain('[deleted user] archived the room');
  });

  it('opens the user menu from the actor name on click and right-click', async () => {
    const onOpenUser = vi.fn();
    const event = systemEvent(TimelineEventKind.UserJoinedRoom, 'Alice');
    render(SystemEvent, { props: { event, onOpenUser } });

    const name = page.getByRole('button', { name: 'Alice', exact: true });
    await name.click();
    expect(onOpenUser).toHaveBeenCalledExactlyOnceWith(event.actor, expect.any(DOMRect));

    const contextMenu = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
    name.element().dispatchEvent(contextMenu);
    expect(contextMenu.defaultPrevented).toBe(true);
    expect(onOpenUser).toHaveBeenCalledTimes(2);
    expect(onOpenUser).toHaveBeenLastCalledWith(event.actor, expect.any(DOMRect));
  });

  it('opens the user menu from the actor avatar', async () => {
    const onOpenUser = vi.fn();
    const event = systemEvent(TimelineEventKind.CallStarted, 'Alice');
    render(SystemEvent, { props: { event, onOpenUser } });

    await page.getByRole('button', { name: 'alice', exact: true }).click();
    expect(onOpenUser).toHaveBeenCalledExactlyOnceWith(event.actor, expect.any(DOMRect));
  });

  it('keeps the actor name and avatar static without a user menu handler', () => {
    const { container } = render(SystemEvent, {
      props: { event: systemEvent(TimelineEventKind.UserJoinedRoom, 'Alice') }
    });

    expect(container.querySelector('button')).toBeNull();
  });

  it('keeps the deleted-user placeholder static', () => {
    const event = systemEvent(TimelineEventKind.RoomArchived);
    if (event.actor) event.actor.deleted = true;

    const { container } = render(SystemEvent, { props: { event, onOpenUser: vi.fn() } });

    expect(container.querySelector('button')).toBeNull();
  });

  it('localizes event copy in German', async () => {
    await loadLocaleMessages('de-DE');
    setReactiveLocale('de-DE');
    const event = systemEvent(TimelineEventKind.UserJoinedRoom, 'Alice');

    const { container } = render(SystemEvent, { props: { event } });

    expect(container.textContent).toContain('Alice ist dem Raum beigetreten');
  });
});
