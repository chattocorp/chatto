import { afterEach, describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import Harness from './RoomDirectoryTestHarness.svelte';
import {
  RoomDirectoryStore,
  type DirectoryRoom
} from '$lib/state/space/roomDirectory.svelte';
import type { RoomsListItem } from '$lib/state/space';
import { RoomType } from '$lib/gql/graphql';

const room = (id: string, overrides: Partial<DirectoryRoom> = {}): DirectoryRoom => ({
  id,
  name: overrides.name ?? id,
  description: overrides.description ?? null,
  archived: overrides.archived ?? false,
  viewerCanJoinRoom: overrides.viewerCanJoinRoom ?? true
});

const joined = (id: string): RoomsListItem => ({
  id,
  name: id,
  type: RoomType.Channel,
  hasUnread: false,
  hasMention: false,
  members: []
});

function findButton(container: Element, label: string): HTMLButtonElement | undefined {
  return [...container.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === label
  ) as HTMLButtonElement | undefined;
}

describe('RoomDirectory', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders a row per non-archived room with a Join button when not joined', () => {
    const { container } = render(Harness, {
      props: {
        initialRooms: [room('r1'), room('r2', { archived: true }), room('r3')],
        joinedRooms: [],
        roomGroups: null
      }
    });
    flushSync();

    const items = container.querySelectorAll('li');
    // r2 is archived → filtered out of `visibleRooms`. r1 and r3 render.
    expect(items.length).toBe(2);

    const joinButtons = [...container.querySelectorAll('button')].filter(
      (b) => b.textContent?.trim() === 'Join'
    );
    expect(joinButtons.length).toBe(2);
  });

  it('renders "Joined" for rooms in the joined membership set', () => {
    const { container } = render(Harness, {
      props: {
        initialRooms: [room('r1'), room('r2')],
        joinedRooms: [joined('r1')],
        roomGroups: null
      }
    });
    flushSync();

    // Each row has at most one button (the join/joined/leave control).
    // For the joined row, both "Joined" and "Leave" labels live in the
    // button as sibling spans (hover swaps them via group-hover classes),
    // so we look for either via textContent.
    expect(container.textContent).toContain('Joined');
    expect(container.textContent).toContain('Join');
  });

  it('renders "Restricted" for rooms the viewer cannot join', () => {
    const { container } = render(Harness, {
      props: {
        initialRooms: [room('locked', { viewerCanJoinRoom: false })],
        joinedRooms: [],
        roomGroups: null
      }
    });
    flushSync();

    expect(container.textContent).toContain('Restricted');
  });

  it('shows the empty state when there are no visible rooms', () => {
    const { container } = render(Harness, {
      props: {
        initialRooms: [],
        joinedRooms: [],
        roomGroups: null
      }
    });
    flushSync();

    expect(container.textContent).toContain('No rooms in this server yet');
  });

  it('groups rooms by section when a layout is provided', () => {
    const { container } = render(Harness, {
      props: {
        initialRooms: [room('r1', { name: 'general' }), room('r2', { name: 'random' })],
        joinedRooms: [],
        roomGroups: [{ id: 'sec', name: 'Important', roomIds: ['r1'] }]
      }
    });
    flushSync();

    // The section header is rendered.
    expect(container.textContent).toContain('Important');
  });

  // -- "Join all" group action -----------------------------------------------

  it('renders "Join all" on a group card with at least one joinable, non-joined room', () => {
    const { container } = render(Harness, {
      props: {
        initialRooms: [room('a'), room('b')],
        joinedRooms: [],
        roomGroups: [{ id: 'g1', name: 'Group One', roomIds: ['a', 'b'] }]
      }
    });
    flushSync();
    expect(findButton(container, 'Join all')).toBeDefined();
  });

  it('hides "Join all" when every room in the group is already joined', () => {
    const { container } = render(Harness, {
      props: {
        initialRooms: [room('a'), room('b')],
        joinedRooms: [joined('a'), joined('b')],
        roomGroups: [{ id: 'g1', name: 'All Joined', roomIds: ['a', 'b'] }]
      }
    });
    flushSync();
    expect(findButton(container, 'Join all')).toBeUndefined();
  });

  it('hides "Join all" when no room in the group is joinable', () => {
    const { container } = render(Harness, {
      props: {
        initialRooms: [
          room('a', { viewerCanJoinRoom: false }),
          room('b', { viewerCanJoinRoom: false })
        ],
        joinedRooms: [],
        roomGroups: [{ id: 'g1', name: 'Restricted Only', roomIds: ['a', 'b'] }]
      }
    });
    flushSync();
    expect(findButton(container, 'Join all')).toBeUndefined();
  });

  it('clicking "Join all" calls directory.joinGroup with the group ID', async () => {
    // Spy on the store prototype so the harness's own instance picks it up.
    const spy = vi
      .spyOn(RoomDirectoryStore.prototype, 'joinGroup')
      .mockResolvedValue({ ok: true, joinedRoomIds: ['a'] });

    const { container } = render(Harness, {
      props: {
        initialRooms: [room('a'), room('b')],
        joinedRooms: [joined('b')],
        roomGroups: [{ id: 'g1', name: 'Mixed', roomIds: ['a', 'b'] }]
      }
    });
    flushSync();

    const btn = findButton(container, 'Join all');
    expect(btn).toBeDefined();
    btn!.click();

    expect(spy).toHaveBeenCalledWith('g1');
  });

  // Filter is bound via bind:value; this confirms the search-match derivation
  // wires through to the rendered list.
  it('filters by search query (case-insensitive, matches name or description)', async () => {
    const { container } = render(Harness, {
      props: {
        initialRooms: [
          room('r1', { name: 'general' }),
          room('r2', { name: 'random', description: 'off-topic chat' })
        ],
        joinedRooms: [],
        roomGroups: null
      }
    });
    flushSync();

    const input = container.querySelector('input[type="text"]') as HTMLInputElement;
    input.value = 'off-topic';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    const items = container.querySelectorAll('li');
    expect(items.length).toBe(1);
    expect(items[0].textContent).toContain('random');
  });
});
