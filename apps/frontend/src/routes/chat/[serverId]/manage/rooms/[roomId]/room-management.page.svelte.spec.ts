import { beforeEach, describe, expect, it, vi } from 'vitest';
import { RealtimeProjectionUpdate } from '$lib/eventBus.svelte';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { Room } from '@chatto/api-types/api/v1/rooms_pb';
import { ListRoomsResponse, RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { RealtimeResourceUpdate } from '$lib/api-client/realtimeResources';
import { RoomDeletedEvent } from '@chatto/api-types/realtime/v1/events_pb';
import { RealtimeEvent as PublicRealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { queryClient } from '$lib/query/client';
import { adminQueryKeys } from '$lib/query/admin';
import { removeRegisteredAdminQueries } from '$lib/query/cacheRegistry';
import type { AdminManagedRoom } from '$lib/api-client/adminRoomLayout';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = vi.hoisted(() => ({
  getRoom: vi.fn(),
  goto: vi.fn(),
  listRoomMembers: vi.fn(),
  projectionHandlers: [] as Array<(event: RealtimeProjectionUpdate) => void>,
  updateRoom: vi.fn(),
  refreshLayout: vi.fn(),
  success: vi.fn(),
  error: vi.fn()
}));

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

vi.mock('$app/state', () => ({
  page: {
    get params() {
      return { serverId: server.serverId, roomId: 'shared-room' };
    },
    get route() {
      return { id: routeId };
    }
  }
}));

vi.mock('$app/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$app/navigation')>()),
  goto: mocks.goto
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => server.serverId
}));

vi.mock('$lib/hooks', () => ({
  useProjectionEvent: (handler: (event: RealtimeProjectionUpdate) => void) => {
    mocks.projectionHandlers.push(handler);
  }
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    isOriginServer: () => false,
    getServer: (serverId: string) => ({ id: serverId, url: `https://${serverId}.example.test` }),
    tryGetStore: () => server.scope.store,
    getStore: () => ({})
  }
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

let server: TestServerScope;
let routeId = '/chat/[serverId]/manage/rooms/[roomId]';

type Section = 'general' | 'members' | 'permissions';

/** Renders the sections on the route of the first one. */
function renderSections(...sections: Section[]) {
  routeId =
    sections[0] === 'general'
      ? '/chat/[serverId]/manage/rooms/[roomId]'
      : `/chat/[serverId]/manage/rooms/[roomId]/${sections[0]}`;
  return render(RoomManagementTestHarness, { props: { sections } });
}

vi.mock('$lib/api-client/adminRoomLayout', () => ({
  createAdminRoomLayoutAPI: ({ serverId }: { serverId: string }) => ({
    getRoom: (roomId: string, options?: { signal?: AbortSignal }) =>
      mocks.getRoom(serverId, roomId, options)
  })
}));

vi.mock('$lib/api-client/memberDirectory', () => ({
  createMemberDirectoryAPI: () => ({
    listRoomMembers: mocks.listRoomMembers,
    listUsers: () => Promise.resolve({ members: [], totalCount: 0, hasMore: false }),
    batchGetRoomMembers: () => Promise.resolve([])
  })
}));

vi.mock('$lib/api-client/rooms', () => ({
  createRoomCommandAPI: () => ({
    updateRoom: mocks.updateRoom,
    addMember: vi.fn(),
    removeMember: vi.fn()
  })
}));

vi.mock('$lib/components/rbac/PermissionMatrix.svelte', async () => ({
  default: (await import('./RoomManagementPagePermissionMatrixMock.svelte')).default
}));

vi.mock('$lib/ui/toast', () => ({
  toast: { success: mocks.success, error: mocks.error }
}));

import RoomManagementTestHarness from './RoomManagementTestHarness.svelte';
import { RoomThreadingMode } from '$lib/roomThreading';

function managedRoom(
  name: string,
  overrides: Partial<{
    archived: boolean;
    isUniversal: boolean;
    threadingMode: RoomThreadingMode;
    canManageRoom: boolean;
    canManagePermissions: boolean;
  }> = {}
) {
  return {
    id: 'shared-room',
    name,
    description: null,
    archived: overrides.archived ?? false,
    isUniversal: overrides.isUniversal ?? false,
    slowModeSeconds: 0,
    threadingMode: overrides.threadingMode ?? RoomThreadingMode.ENABLED,
    canManageRoom: overrides.canManageRoom ?? true,
    canManagePermissions: overrides.canManagePermissions ?? true
  };
}

async function settle(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function dispatchProjection(event: RealtimeProjectionUpdate): void {
  for (const handler of mocks.projectionHandlers) handler(event);
}

type RoomPatch = {
  name?: string;
  description?: string | null;
  universal?: boolean;
  slowModeSeconds?: number;
  threadingMode?: RoomThreadingMode;
};

/**
 * Serves one room like the server: an update applies its patch, and later
 * reads return the updated room. `hold` delays the response of the next
 * update until the returned function is called.
 */
function serveRoom(initial: ReturnType<typeof managedRoom>) {
  let stored: Omit<typeof initial, 'description'> & { description: string | null } = {
    ...initial
  };
  const held: Array<() => void> = [];
  let holdNext = false;
  mocks.getRoom.mockImplementation(() => Promise.resolve({ ...stored }));
  mocks.updateRoom.mockImplementation((input: RoomPatch & { roomId: string }) => {
    stored = {
      ...stored,
      ...(input.name !== undefined && { name: input.name }),
      ...(input.description !== undefined && { description: input.description }),
      ...(input.universal !== undefined && { isUniversal: input.universal }),
      ...(input.slowModeSeconds !== undefined && { slowModeSeconds: input.slowModeSeconds }),
      ...(input.threadingMode !== undefined && { threadingMode: input.threadingMode })
    };
    const response = {
      id: stored.id,
      name: stored.name,
      description: stored.description ?? '',
      universal: stored.isUniversal,
      slowModeSeconds: stored.slowModeSeconds,
      threadingMode: stored.threadingMode,
      archived: stored.archived
    };
    if (!holdNext) return Promise.resolve(response);
    holdNext = false;
    return new Promise((resolve) => held.push(() => resolve(response)));
  });
  return {
    hold() {
      holdNext = true;
      return () => held.shift()?.();
    }
  };
}

function roomSnapshot(present = true): RealtimeProjectionUpdate {
  return new RealtimeProjectionUpdate({
    resource: new RealtimeResourceUpdate({
      resource: {
        case: 'rooms',
        value: new ListRoomsResponse({
          rooms: present
            ? [
                new RoomWithViewerState({
                  room: new Room({ id: 'shared-room', name: 'general' })
                })
              ]
            : []
        })
      }
    })
  });
}

function roomRemoved(): RealtimeProjectionUpdate {
  return new RealtimeProjectionUpdate({
    event: new PublicRealtimeEvent({
      event: {
        case: 'roomDeleted',
        value: new RoomDeletedEvent({ roomId: 'shared-room' })
      }
    })
  });
}

describe('room management page identity and realtime authority', () => {
  beforeEach(async () => {
    queryClient.clear();
    vi.clearAllMocks();
    mocks.projectionHandlers = [];
    server = createTestServerScope({
      serverId: 'server-a',
      permissions: { canManageRooms: true, canAdminManageRoles: true },
      store: { adminRoomLayout: { refresh: mocks.refreshLayout } }
    });
    mocks.refreshLayout.mockResolvedValue(undefined);
    mocks.listRoomMembers.mockResolvedValue({ members: [], totalCount: 0, hasMore: false });
    mocks.updateRoom.mockResolvedValue({
      id: 'shared-room',
      name: 'general',
      description: '',
      universal: false,
      slowModeSeconds: 0,
      threadingMode: RoomThreadingMode.ENABLED,
      archived: false
    });
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('reconciles room rules and permissions after a realtime room update', async () => {
    mocks.getRoom.mockResolvedValueOnce(managedRoom('general')).mockResolvedValueOnce(
      managedRoom('remote-name', {
        archived: true,
        isUniversal: true
      })
    );
    const { container } = renderSections('general', 'members');
    await settle();
    expect(container.querySelector('#room-member-picker')).not.toBeNull();

    dispatchProjection(roomSnapshot());
    await settle();

    expect(container.querySelector('#room-member-picker')).toBeNull();
    expect(container.textContent).toContain('Membership is automatic in Universal rooms.');
    expect((container.querySelector('#room-settings-name') as HTMLInputElement).value).toBe(
      'remote-name'
    );
  });

  it('reuses a fresh room snapshot and preserves a dirty draft across projection refreshes', async () => {
    mocks.getRoom.mockResolvedValueOnce(managedRoom('general')).mockResolvedValueOnce(
      managedRoom('remote-name', {
        isUniversal: true
      })
    );
    const first = renderSections('general', 'members');
    await settle();

    const nameInput = first.container.querySelector('#room-settings-name') as HTMLInputElement;
    nameInput.value = 'local-draft';
    nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    dispatchProjection(roomSnapshot());
    await vi.waitFor(() => expect(mocks.getRoom).toHaveBeenCalledTimes(2));
    await settle();

    expect((first.container.querySelector('#room-settings-name') as HTMLInputElement).value).toBe(
      'local-draft'
    );
    expect(first.container.textContent).toContain('Membership is automatic in Universal rooms.');
    first.unmount();

    mocks.getRoom.mockResolvedValue(managedRoom('remote-name', { isUniversal: true }));
    const second = renderSections('general', 'members');
    await settle();
    expect(mocks.getRoom).toHaveBeenCalledTimes(3);
    expect((second.container.querySelector('#room-settings-name') as HTMLInputElement).value).toBe(
      'remote-name'
    );
  });

  it('accepts spaces, punctuation, emoji, and normalizes Unicode room names', async () => {
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();

    const nameInput = container.querySelector('#room-settings-name') as HTMLInputElement;
    nameInput.value = 'Team chat 💬 / Ku\u0308che!';
    nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    const submit = container.querySelector('form button[type="submit"]') as HTMLButtonElement;
    expect(submit.disabled).toBe(false);
    submit.click();

    await vi.waitFor(() => {
      expect(mocks.updateRoom).toHaveBeenCalledWith({
        roomId: 'shared-room',
        name: 'Team chat 💬 / Küche!'
      });
    });
  });

  it('saves a numeric slow-mode selection immediately as a sparse patch', async () => {
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();

    const select = container.querySelector('#room-settings-slow-mode') as HTMLSelectElement;
    select.value = '10';
    select.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() => {
      expect(mocks.updateRoom).toHaveBeenCalledWith({
        roomId: 'shared-room',
        slowModeSeconds: 10
      });
    });
  });

  it('saves a threading mode immediately when a radio choice is selected', async () => {
    serveRoom(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();

    const choices = Array.from(container.querySelectorAll<HTMLButtonElement>('[role="radio"]'));
    expect(choices).toHaveLength(4);
    expect(choices[2]).toHaveAttribute('aria-checked', 'true');
    choices[0].click();

    await vi.waitFor(() => {
      expect(mocks.updateRoom).toHaveBeenCalledWith({
        roomId: 'shared-room',
        threadingMode: RoomThreadingMode.REQUIRED
      });
    });
    await vi.waitFor(() => expect(choices[0]).toHaveAttribute('aria-checked', 'true'));
  });

  it('rejects invisible-only room names', async () => {
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();

    const nameInput = container.querySelector('#room-settings-name') as HTMLInputElement;
    nameInput.value = '\u200d\u2060';
    nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    expect(
      (container.querySelector('form button[type="submit"]') as HTMLButtonElement).disabled
    ).toBe(true);
    expect(container.textContent).toContain('Room name cannot be empty');
    expect(mocks.updateRoom).not.toHaveBeenCalled();
  });

  it('purges room metadata synchronously when realtime removes access', async () => {
    mocks.getRoom.mockResolvedValueOnce(managedRoom('private-room'));
    const pendingReload = deferred<ReturnType<typeof managedRoom>>();
    const { container } = renderSections('general');
    await settle();
    expect(container.textContent).toContain('#private-room');

    mocks.getRoom.mockReturnValueOnce(pendingReload.promise);
    dispatchProjection(roomRemoved());
    flushSync();

    expect(container.textContent).not.toContain('#private-room');
    expect(container.querySelector('#room-settings-name')).toBeNull();

    pendingReload.resolve(managedRoom('private-room'));
    await settle();
  });

  it('revalidates archived room members only after the admin room reread succeeds', async () => {
    mocks.getRoom
      .mockResolvedValueOnce(managedRoom('general'))
      .mockResolvedValueOnce(managedRoom('general', { archived: true }));
    const { container } = renderSections('members');
    await vi.waitFor(() => expect(mocks.listRoomMembers).toHaveBeenCalledOnce());

    dispatchProjection(roomRemoved());
    flushSync();

    expect(mocks.listRoomMembers).toHaveBeenCalledOnce();
    await vi.waitFor(() => expect(mocks.getRoom).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(mocks.listRoomMembers).toHaveBeenCalledTimes(2));
    expect(container.textContent).toContain(
      'Membership cannot be changed while this room is archived.'
    );
  });

  it('does not reopen member reads when the admin room reread confirms deletion', async () => {
    const deletedRoom = deferred<AdminManagedRoom | null>();
    mocks.getRoom
      .mockResolvedValueOnce(managedRoom('general'))
      .mockReturnValueOnce(deletedRoom.promise);
    const { container } = renderSections('members');
    await vi.waitFor(() => expect(mocks.listRoomMembers).toHaveBeenCalledOnce());

    dispatchProjection(roomRemoved());
    flushSync();
    expect(mocks.listRoomMembers).toHaveBeenCalledOnce();

    deletedRoom.resolve(null);
    await vi.waitFor(() =>
      expect(container.textContent).toContain('You do not have permission to access this page.')
    );
    expect(mocks.listRoomMembers).toHaveBeenCalledOnce();
  });

  it('ignores a stale admin reread superseded by a later room removal', async () => {
    const staleRoom = deferred<AdminManagedRoom | null>();
    const deletedRoom = deferred<AdminManagedRoom | null>();
    mocks.getRoom
      .mockResolvedValueOnce(managedRoom('general'))
      .mockReturnValueOnce(staleRoom.promise)
      .mockReturnValueOnce(deletedRoom.promise);
    renderSections('members');
    await vi.waitFor(() => expect(mocks.listRoomMembers).toHaveBeenCalledOnce());

    const removal = () => dispatchProjection(roomRemoved());
    removal();
    await vi.waitFor(() => expect(mocks.getRoom).toHaveBeenCalledTimes(2));
    removal();
    await vi.waitFor(() => expect(mocks.getRoom).toHaveBeenCalledTimes(3));

    staleRoom.resolve(managedRoom('stale-room', { archived: true }));
    await settle();
    expect(mocks.listRoomMembers).toHaveBeenCalledOnce();

    deletedRoom.resolve(null);
    await settle();
    expect(mocks.listRoomMembers).toHaveBeenCalledOnce();
  });

  it('clears saving after a realtime refresh supersedes the save response', async () => {
    const pendingSave = deferred<{
      id: string;
      name: string;
      description: string;
      universal: boolean;
      archived: boolean;
    }>();
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    mocks.updateRoom.mockReturnValueOnce(pendingSave.promise);
    const { container } = renderSections('general');
    await settle();

    const nameInput = container.querySelector('#room-settings-name') as HTMLInputElement;
    nameInput.value = 'renamed';
    nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    (container.querySelector('form button[type="submit"]') as HTMLButtonElement).click();
    await settle();

    dispatchProjection(roomSnapshot());
    await settle();
    pendingSave.resolve({
      id: 'shared-room',
      name: 'renamed',
      description: '',
      universal: false,
      archived: false
    });
    await settle();

    const refreshedInput = container.querySelector('#room-settings-name') as HTMLInputElement;
    refreshedInput.value = 'later';
    refreshedInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    await vi.waitFor(() =>
      expect(
        (container.querySelector('form button[type="submit"]') as HTMLButtonElement).disabled
      ).toBe(false)
    );
  });

  it('does not restore a room snapshot after an admin-cache privacy boundary', async () => {
    const pendingSave = deferred<{
      id: string;
      name: string;
      description: string;
      universal: boolean;
      archived: boolean;
    }>();
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    mocks.updateRoom.mockReturnValueOnce(pendingSave.promise);
    const view = renderSections('general');
    await settle();

    const input = view.container.querySelector('#room-settings-name') as HTMLInputElement;
    input.value = 'private-name';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    (view.container.querySelector('form button[type="submit"]') as HTMLButtonElement).click();
    await vi.waitFor(() => expect(mocks.updateRoom).toHaveBeenCalledOnce());

    removeRegisteredAdminQueries('server-a');
    view.unmount();
    pendingSave.resolve({
      id: 'shared-room',
      name: 'private-name',
      description: '',
      universal: false,
      archived: false
    });
    await settle();

    const queryKey = adminQueryKeys.room('server-a', server.scope.connection, 'shared-room');
    expect(queryClient.getQueryData<AdminManagedRoom>(queryKey)?.name).not.toBe('private-name');
    expect(mocks.success).not.toHaveBeenCalled();
  });
  function sectionLinks(root: ParentNode) {
    return [...root.querySelectorAll('nav[aria-label="Room sections"] a')].map((link) => ({
      label: link.textContent?.trim(),
      current: link.getAttribute('aria-current')
    }));
  }

  it('offers every section to a room manager and marks the current one', async () => {
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    const { container } = renderSections('permissions');
    await settle();

    expect(sectionLinks(container)).toEqual([
      { label: 'General', current: null },
      { label: 'Members', current: null },
      { label: 'Permissions', current: 'page' }
    ]);
    expect(container.querySelector('[data-testid="permission-matrix"]')).not.toBeNull();
  });

  it('sends a viewer who cannot change the settings to the members section', async () => {
    mocks.getRoom.mockResolvedValue(managedRoom('general', { canManageRoom: false }));
    const { container } = renderSections('general');
    await settle();

    expect(container.querySelector('#room-settings-name')).toBeNull();
    expect(sectionLinks(container).map((link) => link.label)).toEqual(['Members', 'Permissions']);
    await vi.waitFor(() =>
      expect(mocks.goto).toHaveBeenCalledWith(
        '/chat/server-a.example.test/manage/rooms/shared-room/members',
        {
          replaceState: true
        }
      )
    );
  });
  it('saves Universal immediately without resetting a dirty name draft', async () => {
    serveRoom(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();

    const nameInput = container.querySelector('#room-settings-name') as HTMLInputElement;
    nameInput.value = 'draft-name';
    nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    (container.querySelector('#room-settings-universal') as HTMLInputElement).click();

    await vi.waitFor(() =>
      expect(mocks.updateRoom).toHaveBeenCalledWith({ roomId: 'shared-room', universal: true })
    );
    await vi.waitFor(() => expect(mocks.success).toHaveBeenCalledWith('Room updated'));
    expect((container.querySelector('#room-settings-universal') as HTMLInputElement).checked).toBe(
      true
    );
    expect((container.querySelector('#room-settings-name') as HTMLInputElement).value).toBe(
      'draft-name'
    );
  });

  it('shows the saved Universal value again after a failed save', async () => {
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    mocks.updateRoom.mockRejectedValueOnce(new Error('offline'));
    const { container } = renderSections('general');
    await settle();

    const checkbox = container.querySelector('#room-settings-universal') as HTMLInputElement;
    checkbox.click();

    await vi.waitFor(() => expect(mocks.error).toHaveBeenCalled());
    await vi.waitFor(() => expect(checkbox.checked).toBe(false));
    expect(checkbox.disabled).toBe(false);
  });

  it('does not let an older concurrent save restore another panel value', async () => {
    const room = serveRoom(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();

    const releaseUniversal = room.hold();
    (container.querySelector('#room-settings-universal') as HTMLInputElement).click();
    await vi.waitFor(() => expect(mocks.updateRoom).toHaveBeenCalledOnce());
    // The server applied Universal first, so its response still has Slow Mode off.
    const select = container.querySelector('#room-settings-slow-mode') as HTMLSelectElement;
    select.value = '10';
    select.dispatchEvent(new Event('change', { bubbles: true }));
    await vi.waitFor(() => expect(mocks.updateRoom).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(mocks.success).toHaveBeenCalledOnce());
    // Keep the refetch from reconciling, so only the save responses write the cache.
    mocks.getRoom.mockImplementation(() => new Promise(() => {}));

    releaseUniversal();
    await vi.waitFor(() => expect(mocks.success).toHaveBeenCalledTimes(2));

    const cached = queryClient.getQueryData<AdminManagedRoom>(
      adminQueryKeys.room('server-a', server.scope.connection, 'shared-room')
    );
    expect(cached?.isUniversal).toBe(true);
    expect(cached?.slowModeSeconds).toBe(10);
  });
});
