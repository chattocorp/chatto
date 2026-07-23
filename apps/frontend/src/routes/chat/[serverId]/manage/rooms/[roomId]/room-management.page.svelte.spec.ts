import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import {
  RealtimeProjectionEvent,
  RealtimeProjectionOperation,
  RealtimeProjectionRoom
} from '@chatto/api-types/realtime/v1/realtime_pb';
import { Room } from '@chatto/api-types/api/v1/rooms_pb';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import {
  roomManagementPageTestState,
  roomManagementTestPage
} from './RoomManagementPageTestState.svelte';

const mocks = vi.hoisted(() => ({
  getRoom: vi.fn(),
  projectionHandlers: [] as Array<(event: RealtimeProjectionEvent) => void>,
  refreshRooms: vi.fn(),
  protocolCapabilities: ['chatto.api.room-manager-member-reads.v1'] as string[]
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

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    getClient: (serverId: string) => ({
      serverId,
      connectBaseUrl: `https://${serverId}.example.test/api/connect`,
      bearerToken: `${serverId}-token`
    })
  }
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    isOriginServer: () => false,
    getServer: (serverId: string) => ({ id: serverId, url: `https://${serverId}.example.test` }),
    tryGetStore: () => ({
      serverInfo: {
        version: '0.5.0',
        get protocolCapabilities() {
          return mocks.protocolCapabilities;
        }
      }
    }),
    getStore: () => ({ rooms: { refresh: mocks.refreshRooms } })
  }
}));

vi.mock('$lib/state/server/chromePermissions.svelte', () => ({
  getChromePermissions: () => ({
    current: { canManageRooms: true, canManageRoles: true }
  })
}));

vi.mock('$lib/api-client/adminRoomLayout', () => ({
  createAdminRoomLayoutAPI: ({ serverId }: { serverId: string }) => ({
    getRoom: (roomId: string) => mocks.getRoom(serverId, roomId)
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
    updateRoom: vi.fn(),
    addMember: vi.fn(),
    removeMember: vi.fn()
  })
}));

vi.mock('$lib/components/rbac/PermissionMatrix.svelte', async () => ({
  default: (await import('./RoomManagementPagePermissionMatrixMock.svelte')).default
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

describe('room management page identity and realtime authority', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    mocks.projectionHandlers = [];
    mocks.protocolCapabilities = ['chatto.api.room-manager-member-reads.v1'];
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

    roomManagementPageTestState.serverId = 'server-b';
    flushSync();
    await settle();

    expect(mocks.getRoom).toHaveBeenCalledWith('server-b', 'shared-room');
    expect(container.textContent).toContain('#beta');
    expect(container.textContent).not.toContain('#alpha');
  });

  it('reconciles room rules and permissions after a realtime room update', async () => {
    mocks.getRoom.mockResolvedValueOnce(managedRoom('general')).mockResolvedValueOnce(
      managedRoom('general', {
        archived: true,
        isUniversal: true,
        canManageRoom: false
      })
    );
    const { container } = render(RoomManagementPage);
    await settle();
    expect(container.querySelector('#room-member-picker')).not.toBeNull();

    for (const handler of mocks.projectionHandlers) {
      handler(
        new RealtimeProjectionEvent({
          operations: [
            new RealtimeProjectionOperation({
              operation: {
                case: 'roomUpsert',
                value: new RealtimeProjectionRoom({
                  room: new RoomWithViewerState({
                    room: new Room({ id: 'shared-room', name: 'general' })
                  })
                })
              }
            })
          ]
        })
      );
    }
    await settle();

    expect(container.querySelector('#room-member-picker')).toBeNull();
    expect(container.textContent).toContain('Membership is automatic in Universal rooms.');
  });

  it('hides member management when the server does not advertise manager reads', async () => {
    mocks.protocolCapabilities = ['chatto.api.v1'];
    mocks.getRoom.mockResolvedValue(managedRoom('general'));

    const { container } = render(RoomManagementPage);
    await settle();

    expect(container.textContent).not.toContain('Members');
    expect(container.querySelector('#room-member-picker')).toBeNull();
  });
});
