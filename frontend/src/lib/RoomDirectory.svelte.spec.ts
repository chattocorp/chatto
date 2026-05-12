import { describe, it, expect } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import Harness from './RoomDirectoryTestHarness.svelte';
import type { DirectoryRoom } from '$lib/state/space/roomDirectory.svelte';
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

describe('RoomDirectory', () => {
  it('renders a row per non-archived room with a Join button when not joined', () => {
    const { container } = render(Harness, {
      props: {
        initialRooms: [room('r1'), room('r2', { archived: true }), room('r3')],
        joinedRooms: [],
        layoutSections: null
      }
    });
    flushSync();

    const items = container.querySelectorAll('li');
    // r2 is archived → filtered out of `visibleRooms`. r1 and r3 render.
    expect(items.length).toBe(2);

    const joinButtons = container.querySelectorAll('button.bg-primary');
    expect(joinButtons.length).toBe(2);
    expect([...joinButtons].every((b) => b.textContent?.trim() === 'Join')).toBe(true);
  });

  it('renders "Joined" for rooms in the joined membership set', () => {
    const { container } = render(Harness, {
      props: {
        initialRooms: [room('r1'), room('r2')],
        joinedRooms: [joined('r1')],
        layoutSections: null
      }
    });
    flushSync();

    const buttons = [...container.querySelectorAll('button')];
    const labels = buttons.map((b) => b.textContent?.trim());
    expect(labels).toContain('Joined');
    expect(labels).toContain('Join');
  });

  it('renders "No permission" for rooms the viewer cannot join', () => {
    const { container } = render(Harness, {
      props: {
        initialRooms: [room('locked', { viewerCanJoinRoom: false })],
        joinedRooms: [],
        layoutSections: null
      }
    });
    flushSync();

    expect(container.textContent).toContain('No permission');
  });

  it('shows the empty state when there are no visible rooms', () => {
    const { container } = render(Harness, {
      props: {
        initialRooms: [],
        joinedRooms: [],
        layoutSections: null
      }
    });
    flushSync();

    expect(container.textContent).toContain('No rooms in this space yet');
  });

  it('groups rooms by section when a layout is provided', () => {
    const { container } = render(Harness, {
      props: {
        initialRooms: [room('r1', { name: 'general' }), room('r2', { name: 'random' })],
        joinedRooms: [],
        layoutSections: [{ id: 'sec', name: 'Important', roomIds: ['r1'] }]
      }
    });
    flushSync();

    // The section header is rendered.
    expect(container.textContent).toContain('Important');
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
        layoutSections: null
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
