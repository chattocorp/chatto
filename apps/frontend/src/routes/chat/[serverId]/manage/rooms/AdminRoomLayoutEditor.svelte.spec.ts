import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { goto } from '$app/navigation';
import { q } from '$lib/test-utils';
import type { AdminRoomLayoutAPI } from '$lib/api-client/adminRoomLayout';
import type { RoomCommandAPI } from '$lib/api-client/rooms';
import {
  AdminRoomLayoutStore,
  type AdminRoomGroup,
  type AdminRoomInfo
} from '$lib/state/server/adminRoomLayout.svelte';
import AdminRoomLayoutEditor from './AdminRoomLayoutEditor.svelte';

vi.mock('$app/navigation', () => ({
  afterNavigate: vi.fn(),
  beforeNavigate: vi.fn(),
  disableScrollHandling: vi.fn(),
  goto: vi.fn(),
  invalidate: vi.fn(),
  invalidateAll: vi.fn(),
  onNavigate: vi.fn(),
  preloadCode: vi.fn(),
  preloadData: vi.fn(),
  pushState: vi.fn(),
  replaceState: vi.fn()
}));

vi.mock('$app/paths', () => ({
  assets: '',
  base: '',
  resolve: (path: string, params?: Record<string, string>) =>
    path
      .replace('[serverId]', params?.serverId ?? '')
      .replace('[groupId]', params?.groupId ?? '')
      .replace('[roomId]', params?.roomId ?? '')
}));

vi.mock('svelte-dnd-action', () => ({
  dndzone: () => ({
    update: vi.fn(),
    destroy: vi.fn()
  }),
  dragHandleZone: () => ({
    update: vi.fn(),
    destroy: vi.fn()
  }),
  dragHandle: () => ({
    destroy: vi.fn()
  })
}));

function room(id: string, overrides: Partial<AdminRoomInfo> = {}): AdminRoomInfo {
  return {
    id,
    name: overrides.name ?? id,
    description: overrides.description ?? null,
    archived: overrides.archived ?? false,
    isUniversal: overrides.isUniversal ?? false
  };
}

function group(id: string, rooms: AdminRoomInfo[], name = id): AdminRoomGroup {
  return {
    id,
    name,
    canCreateRoom: true,
    rooms,
    items: rooms.map((room) => ({ id: `room:${room.id}`, kind: 'room', room }))
  };
}

function roomAPI(): Pick<RoomCommandAPI, 'updateRoom' | 'archiveRoom' | 'unarchiveRoom'> {
  return {
    updateRoom: vi.fn().mockResolvedValue(null),
    archiveRoom: vi.fn().mockResolvedValue(null),
    unarchiveRoom: vi.fn().mockResolvedValue(null)
  };
}

function makeLayout(): AdminRoomLayoutStore {
  const layoutAPI = {
    getRoom: vi.fn().mockResolvedValue(null),
    getRoomGroup: vi.fn().mockResolvedValue(null),
    listRoomGroups: vi.fn().mockResolvedValue([]),
    createRoomGroup: vi.fn().mockResolvedValue(null),
    updateRoomGroup: vi.fn().mockResolvedValue(null),
    deleteRoomGroup: vi.fn().mockResolvedValue(true),
    reorderRoomGroups: vi.fn().mockResolvedValue([]),
    moveRoomToGroup: vi.fn().mockResolvedValue(undefined),
    reorderSidebarItemsInGroup: vi.fn().mockResolvedValue(null),
    createSidebarLink: vi.fn().mockResolvedValue(null),
    updateSidebarLink: vi.fn().mockResolvedValue(null),
    deleteSidebarLink: vi.fn().mockResolvedValue(true),
    moveSidebarLinkToGroup: vi.fn().mockResolvedValue(undefined)
  } satisfies AdminRoomLayoutAPI;
  return new AdminRoomLayoutStore(layoutAPI, roomAPI());
}

function renderEditor(layout: AdminRoomLayoutStore) {
  return render(AdminRoomLayoutEditor, {
    props: { layout, serverSegment: '-' }
  });
}

function buttonByText(container: Element, text: string): HTMLButtonElement {
  const button = [...container.querySelectorAll('button')].find((b) =>
    b.textContent?.includes(text)
  );
  if (!(button instanceof HTMLButtonElement)) {
    throw new Error(`button not found: ${text}`);
  }
  return button;
}

function fill(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}

describe('AdminRoomLayoutEditor', () => {
  it('renders loading, error, empty, and populated states from the layout store', async () => {
    const loading = makeLayout();
    loading.isRefreshing = true;
    const loadingRender = renderEditor(loading);
    await expect.element(q(loadingRender.container, 'div')).toHaveTextContent('Loading rooms...');

    const error = makeLayout();
    error.error = 'Server not found';
    const errorRender = renderEditor(error);
    expect(errorRender.container.textContent).toContain('Server not found');

    const empty = makeLayout();
    empty.initialized = true;
    const emptyRender = renderEditor(empty);
    expect(emptyRender.container.textContent).toContain('No room groups yet');

    const populated = makeLayout();
    populated.initialized = true;
    populated.groups = [
      group('g1', [room('r1', { name: 'general', description: 'Public room' })], 'Lobby')
    ];
    const populatedRender = renderEditor(populated);
    expect(populatedRender.container.textContent).toContain('Lobby');
    expect(populatedRender.container.textContent).toContain('general');
    expect(populatedRender.container.textContent).toContain('Public room');

    const shell = populatedRender.container.querySelector('section.panel-shell') as HTMLElement;
    const header = shell.querySelector(':scope > header') as HTMLElement;
    const frame = shell.querySelector(':scope > div:last-child') as HTMLElement;
    const inset = frame.firstElementChild as HTMLElement;
    expect(shell.className).toContain('shrink-0');
    expect(header.className).toContain('px-6');
    expect(frame.className).toContain('px-1');
    expect(frame.className).toContain('pb-1');
    expect(inset.className).toContain('panel-inset');
    const roomDragHandle = populatedRender.container.querySelector(
      '[aria-label="Drag to reorder room"]'
    ) as HTMLElement;
    expect(roomDragHandle.className).toContain('uil--draggabledots');
    expect(roomDragHandle.className).toContain('cursor-grab');
  });

  it('opens the create-group dialog and delegates submission to the layout store', async () => {
    const layout = makeLayout();
    layout.initialized = true;
    layout.groups = [group('g1', [], 'Lobby')];
    const createGroup = vi.spyOn(layout, 'createGroup').mockResolvedValue({
      ok: true,
      group: group('g2', [], 'Projects')
    });
    const { container } = renderEditor(layout);

    buttonByText(container, 'New Group').click();
    flushSync();
    fill(q(container, '#new-group-name') as HTMLInputElement, 'Projects');
    buttonByText(container, 'Create Group').click();

    await vi.waitFor(() => {
      expect(createGroup).toHaveBeenCalledWith('Projects');
    });
  });

  it('opens the room edit page from the room edit action without showing a dialog', async () => {
    const layout = makeLayout();
    layout.initialized = true;
    layout.groups = [group('g1', [room('r1', { name: 'general' })], 'Lobby')];
    const { container } = renderEditor(layout);

    const edit = container.querySelector('[title="Edit room"]');
    if (!(edit instanceof HTMLButtonElement)) throw new Error('edit button not found');
    edit.click();

    expect(goto).toHaveBeenCalledWith('/chat/-/manage/rooms/r1');
    expect(container.querySelector('#edit-room-name')).toBeNull();
    expect(container.querySelector('[role="dialog"]')).toBeNull();
  });

  it('opens the room-group edit page from the group edit action without showing a dialog', async () => {
    const layout = makeLayout();
    layout.initialized = true;
    layout.groups = [group('g1', [], 'Lobby')];
    const { container } = renderEditor(layout);

    const edit = container.querySelector('[title="Rename group"]');
    if (!(edit instanceof HTMLButtonElement)) throw new Error('edit button not found');
    edit.click();

    expect(goto).toHaveBeenCalledWith('/chat/-/manage/room-groups/g1');
    expect(container.querySelector('#edit-group-name')).toBeNull();
    expect(container.querySelector('[role="dialog"]')).toBeNull();
  });
});
