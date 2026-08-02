import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import {
  RealtimeProjectionEvent,
  RealtimeProjectionOperation,
  RealtimeProjectionRoom,
  RealtimeProjectionRoomRemove
} from '@chatto/api-types/realtime/v1/realtime_pb';
import { Room } from '@chatto/api-types/api/v1/rooms_pb';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { queryClient } from '$lib/query/client';
import { adminQueryKeys } from '$lib/query/admin';
import { removeRegisteredAdminQueries } from '$lib/query/cacheRegistry';
import type { AdminManagedRoom } from '$lib/api-client/adminRoomLayout';
import {
  roomManagementPageTestState,
  roomManagementTestPage
} from './RoomManagementPageTestState.svelte';

const mocks = vi.hoisted(() => ({
  getRoom: vi.fn(),
  projectionHandlers: [] as Array<(event: RealtimeProjectionEvent) => void>,
  updateRoom: vi.fn(),
  refreshLayout: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
  serverVersion: '0.5.0'
}));

vi.mock('$app/state', () => ({ page: roomManagementTestPage }));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => roomManagementPageTestState.serverId
}));

vi.mock('$lib/hooks', () => ({
  useProjectionEvent: (handler: (event: RealtimeProjectionEvent) => void) => {
    mocks.projectionHandlers.push(handler);
  }
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    isOriginServer: () => false,
    getServer: (serverId: string) => ({ id: serverId, url: `https://${serverId}.example.test` }),
    tryGetStore: () => ({
      serverInfo: {
        get version() {
          return mocks.serverVersion;
        },
        supportsFeature: () => mocks.serverVersion === '0.5.0'
      }
    }),
    getStore: () => ({})
  }
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    get serverId() {
      return roomManagementPageTestState.serverId;
    },
    get connection() {
      const serverId = roomManagementPageTestState.serverId;
      return {
        queryScope: `${serverId}-query-scope`,
        getAPI: (factory: (config: never) => unknown) =>
          factory({
            serverId,
            baseUrl: `https://${serverId}.example.test/api/connect`,
            bearerToken: `${serverId}-token`
          } as never)
      };
    },
    get store() {
      return {
        serverInfo: {
          get version() {
            return mocks.serverVersion;
          },
          supportsFeature: () => mocks.serverVersion === '0.5.0'
        },
        adminRoomLayout: { refresh: mocks.refreshLayout }
      };
    },
    isCurrent: () => true
  })
}));

vi.mock('$lib/state/server/chromePermissions.svelte', () => ({
  getChromePermissions: () => () => ({ canManageRooms: true, canManageRoles: true })
}));

vi.mock('$lib/api-client/adminRoomLayout', () => ({
  createAdminRoomLayoutAPI: ({ serverId }: { serverId: string }) => ({
    getRoom: (roomId: string, options?: { signal?: AbortSignal }) =>
      mocks.getRoom(serverId, roomId, options)
  })
}));

vi.mock('$lib/api-client/memberDirectory', () => ({
  createMemberDirectoryAPI: () => ({
    listRoomMembers: () => Promise.resolve({ members: [], totalCount: 0, hasMore: false }),
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

import RoomManagementPage from './+page.svelte';

function managedRoom(
  name: string,
  overrides: Partial<{
    archived: boolean;
    isUniversal: boolean;
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

function dispatchProjection(operation: RealtimeProjectionOperation): void {
  const event = new RealtimeProjectionEvent({ operations: [operation] });
  for (const handler of mocks.projectionHandlers) handler(event);
}

function roomUpsert(): RealtimeProjectionOperation {
  return new RealtimeProjectionOperation({
    operation: {
      case: 'roomUpsert',
      value: new RealtimeProjectionRoom({
        room: new RoomWithViewerState({
          room: new Room({ id: 'shared-room', name: 'general' })
        })
      })
    }
  });
}

describe('room management page identity and realtime authority', () => {
  beforeEach(async () => {
    queryClient.clear();
    vi.clearAllMocks();
    mocks.projectionHandlers = [];
    mocks.serverVersion = '0.5.0';
    mocks.refreshLayout.mockResolvedValue(undefined);
    mocks.updateRoom.mockResolvedValue({
      id: 'shared-room',
      name: 'general',
      description: '',
      universal: false,
      archived: false
    });
    roomManagementPageTestState.reset();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('reloads metadata when the server changes but the room ID stays the same', async () => {
    mocks.getRoom.mockImplementation((serverId: string) =>
      Promise.resolve(managedRoom(serverId === 'server-a' ? 'alpha' : 'beta'))
    );
    const { container } = render(RoomManagementPage);
    await settle();
    expect(container.textContent).toContain('#alpha');
    expect(
      container
        .querySelector('[data-testid="permission-matrix"]')
        ?.getAttribute('data-scroll-contents')
    ).toBe('false');

    roomManagementPageTestState.serverId = 'server-b';
    flushSync();
    await settle();

    expect(mocks.getRoom).toHaveBeenCalledWith(
      'server-b',
      'shared-room',
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(container.textContent).toContain('#beta');
    expect(container.textContent).not.toContain('#alpha');
  });

  it('reconciles room rules and permissions after a realtime room update', async () => {
    mocks.getRoom.mockResolvedValueOnce(managedRoom('general')).mockResolvedValueOnce(
      managedRoom('remote-name', {
        archived: true,
        isUniversal: true
      })
    );
    const { container } = render(RoomManagementPage);
    await settle();
    expect(container.querySelector('#room-member-picker')).not.toBeNull();

    dispatchProjection(roomUpsert());
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
    const first = render(RoomManagementPage);
    await settle();

    const nameInput = first.container.querySelector('#room-settings-name') as HTMLInputElement;
    nameInput.value = 'local-draft';
    nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    dispatchProjection(roomUpsert());
    await vi.waitFor(() => expect(mocks.getRoom).toHaveBeenCalledTimes(2));
    await settle();

    expect((first.container.querySelector('#room-settings-name') as HTMLInputElement).value).toBe(
      'local-draft'
    );
    expect(first.container.textContent).toContain('Membership is automatic in Universal rooms.');
    first.unmount();

    mocks.getRoom.mockResolvedValue(managedRoom('remote-name', { isUniversal: true }));
    const second = render(RoomManagementPage);
    await settle();
    expect(mocks.getRoom).toHaveBeenCalledTimes(3);
    expect((second.container.querySelector('#room-settings-name') as HTMLInputElement).value).toBe(
      'remote-name'
    );
  });

  it('hides member management on servers that predate the room-management API', async () => {
    mocks.serverVersion = '0.4.19';
    mocks.getRoom.mockResolvedValue(managedRoom('general'));

    const { container } = render(RoomManagementPage);
    await settle();

    expect(container.textContent).not.toContain('Members');
    expect(container.querySelector('#room-member-picker')).toBeNull();
  });

  it('does not request management details from servers that predate the admin API', async () => {
    mocks.serverVersion = '0.4.19';

    const { container } = render(RoomManagementPage);
    await settle();

    expect(mocks.getRoom).not.toHaveBeenCalled();
    expect(container.textContent).toContain('You do not have permission to access this page.');
  });

  it('accepts and normalizes Unicode room names', async () => {
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    const { container } = render(RoomManagementPage);
    await settle();

    const nameInput = container.querySelector('#room-settings-name') as HTMLInputElement;
    nameInput.value = 'Ku\u0308che_繁體';
    nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    const submit = container.querySelector('form button[type="submit"]') as HTMLButtonElement;
    expect(submit.disabled).toBe(false);
    submit.click();

    await vi.waitFor(() => {
      expect(mocks.updateRoom).toHaveBeenCalledWith({
        roomId: 'shared-room',
        name: 'Küche_繁體'
      });
    });
  });

  it('purges room metadata synchronously when realtime removes access', async () => {
    mocks.getRoom.mockResolvedValueOnce(managedRoom('private-room'));
    const pendingReload = deferred<ReturnType<typeof managedRoom>>();
    const { container } = render(RoomManagementPage);
    await settle();
    expect(container.textContent).toContain('#private-room');

    mocks.getRoom.mockReturnValueOnce(pendingReload.promise);
    dispatchProjection(
      new RealtimeProjectionOperation({
        operation: {
          case: 'roomRemove',
          value: new RealtimeProjectionRoomRemove({ roomId: 'shared-room' })
        }
      })
    );
    flushSync();

    expect(container.textContent).not.toContain('#private-room');
    expect(container.querySelector('#room-settings-name')).toBeNull();

    pendingReload.resolve(managedRoom('private-room'));
    await settle();
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
    const { container } = render(RoomManagementPage);
    await settle();

    const nameInput = container.querySelector('#room-settings-name') as HTMLInputElement;
    nameInput.value = 'renamed';
    nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    (container.querySelector('form button[type="submit"]') as HTMLButtonElement).click();
    await settle();

    dispatchProjection(roomUpsert());
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

    expect(
      (container.querySelector('form button[type="submit"]') as HTMLButtonElement).disabled
    ).toBe(false);
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
    const view = render(RoomManagementPage);
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

    const queryKey = adminQueryKeys.room(
      'server-a',
      { queryScope: 'server-a-query-scope' },
      'shared-room'
    );
    expect(queryClient.getQueryData<AdminManagedRoom>(queryKey)?.name).not.toBe('private-name');
    expect(mocks.success).not.toHaveBeenCalled();
  });
});
