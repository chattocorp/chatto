import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminRoomLayoutService } from '@chatto/api-types/admin/v1/room_layout_connect';
import { createAdminRoomLayoutAPI } from '$lib/api-client/adminRoomLayout';
import { RoomThreadingMode } from '$lib/roomThreading';
import { fakeServer, mockService, receivedRequest } from '$lib/test-utils';

const mocks = mockService(AdminRoomLayoutService);

function roomLayoutAPI() {
  return createAdminRoomLayoutAPI(
    fakeServer((router) => router.service(AdminRoomLayoutService, mocks))
  );
}

describe('createAdminRoomLayoutAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('reads layout and sends group, room, link, and reorder commands through Connect', async () => {
    mocks.getRoom.mockReturnValue({
      room: { id: 'r1', name: 'general', description: 'General chat' },
      viewerCanManageRoom: false,
      viewerCanManagePermissions: true
    });
    mocks.getRoomGroup.mockReturnValue({
      group: { id: 'g1', name: 'Lobby', items: [] },
      viewerCanManageGroup: true,
      viewerCanManagePermissions: true
    });
    mocks.listRoomGroups.mockReturnValue({
      groups: [
        {
          id: 'g1',
          name: 'Lobby',
          description: 'Main rooms',
          canCreateRoom: true,
          items: [
            {
              item: {
                case: 'room',
                value: {
                  id: 'r1',
                  name: 'general',
                  archived: true
                }
              }
            }
          ]
        }
      ]
    });
    mocks.createRoomGroup.mockReturnValue({
      group: { id: 'g2', name: 'Projects', description: 'Project rooms', items: [] }
    });
    mocks.updateRoomGroup.mockReturnValue({ group: { id: 'g2', name: 'Renamed', items: [] } });
    mocks.deleteRoomGroup.mockReturnValue({});
    mocks.reorderRoomGroups.mockReturnValue({ groups: [] });
    mocks.moveRoomGroup.mockReturnValue({ groups: [] });
    mocks.moveRoomToGroup.mockReturnValue({});
    mocks.reorderSidebarItemsInGroup.mockReturnValue({ group: undefined });
    mocks.moveSidebarItem.mockReturnValue({ group: undefined });
    mocks.createSidebarLink.mockReturnValue({
      sidebarLink: { id: 'docs', label: 'Docs', url: '/docs' }
    });
    mocks.updateSidebarLink.mockReturnValue({
      sidebarLink: { id: 'docs', label: 'Docs', url: '/help' }
    });
    mocks.deleteSidebarLink.mockReturnValue({});
    mocks.moveSidebarLinkToGroup.mockReturnValue({});

    const api = roomLayoutAPI();

    await expect(api.getRoom('r1')).resolves.toMatchObject({
      id: 'r1',
      name: 'general',
      canManageRoom: false,
      canManagePermissions: true
    });
    await expect(api.getRoomGroup('g1')).resolves.toMatchObject({
      group: { id: 'g1', name: 'Lobby' },
      canManageGroup: true,
      canManagePermissions: true
    });

    await expect(api.listRoomGroups()).resolves.toEqual([
      {
        id: 'g1',
        name: 'Lobby',
        description: 'Main rooms',
        canCreateRoom: true,
        rooms: [
          {
            id: 'r1',
            name: 'general',
            description: null,
            archived: true,
            isUniversal: false,
            slowModeSeconds: 0,
            threadingMode: RoomThreadingMode.ENABLED
          }
        ],
        items: [
          {
            id: 'room:r1',
            kind: 'room',
            room: {
              id: 'r1',
              name: 'general',
              description: null,
              archived: true,
              isUniversal: false,
              slowModeSeconds: 0,
              threadingMode: RoomThreadingMode.ENABLED
            }
          }
        ]
      }
    ]);
    await expect(api.createRoomGroup({ name: 'Projects' })).resolves.toEqual({
      id: 'g2',
      name: 'Projects',
      description: 'Project rooms',
      canCreateRoom: false,
      rooms: [],
      items: []
    });
    await api.updateRoomGroup({ groupId: 'g2', name: 'Renamed' });
    await api.deleteRoomGroup('g2');
    await api.reorderRoomGroups(['g2', 'g1']);
    await api.moveRoomGroup({ groupId: 'g2', beforeGroupId: 'g1' });
    await api.moveRoomToGroup({ roomId: 'room-1', groupId: 'g2' });
    await api.reorderSidebarItemsInGroup({
      groupId: 'g2',
      items: [
        { kind: 'room', id: 'room-1' },
        { kind: 'link', id: 'docs' }
      ]
    });
    await api.moveSidebarItem({
      item: { kind: 'room', id: 'room-1' },
      groupId: 'g2',
      before: { kind: 'link', id: 'docs' }
    });
    await api.createSidebarLink({ groupId: 'g2', label: 'Docs', url: '/docs' });
    await api.updateSidebarLink({ linkId: 'docs', label: 'Docs', url: '/help' });
    await api.deleteSidebarLink('docs');
    await api.moveSidebarLinkToGroup({ linkId: 'docs', groupId: 'g1' });

    expect(receivedRequest(mocks.getRoom)).toMatchObject({ roomId: 'r1' });
    expect(receivedRequest(mocks.getRoomGroup)).toMatchObject({ groupId: 'g1' });
    expect(mocks.listRoomGroups).toHaveBeenCalledOnce();
    expect(receivedRequest(mocks.createRoomGroup)).toMatchObject({
      name: 'Projects',
      description: ''
    });
    expect(receivedRequest(mocks.updateRoomGroup)).toMatchObject({
      groupId: 'g2',
      name: 'Renamed',
      description: undefined,
      updateMask: { paths: ['name'] }
    });
    expect(receivedRequest(mocks.deleteRoomGroup)).toMatchObject({ groupId: 'g2' });
    expect(receivedRequest(mocks.reorderRoomGroups)).toMatchObject({
      orderedGroupIds: ['g2', 'g1']
    });
    expect(receivedRequest(mocks.moveRoomGroup)).toMatchObject({
      groupId: 'g2',
      beforeGroupId: 'g1'
    });
    expect(receivedRequest(mocks.moveRoomToGroup)).toMatchObject({
      roomId: 'room-1',
      groupId: 'g2'
    });
    expect(receivedRequest(mocks.reorderSidebarItemsInGroup)).toMatchObject({
      groupId: 'g2',
      items: [
        { item: { case: 'roomId', value: 'room-1' } },
        { item: { case: 'sidebarLinkId', value: 'docs' } }
      ]
    });
    expect(receivedRequest(mocks.moveSidebarItem)).toMatchObject({
      item: { item: { case: 'roomId', value: 'room-1' } },
      groupId: 'g2',
      before: { item: { case: 'sidebarLinkId', value: 'docs' } }
    });
    expect(receivedRequest(mocks.createSidebarLink)).toMatchObject({
      groupId: 'g2',
      label: 'Docs',
      url: '/docs'
    });
    expect(receivedRequest(mocks.updateSidebarLink)).toMatchObject({
      linkId: 'docs',
      label: 'Docs',
      url: '/help',
      updateMask: { paths: ['label', 'url'] }
    });
    expect(receivedRequest(mocks.deleteSidebarLink)).toMatchObject({ linkId: 'docs' });
    expect(receivedRequest(mocks.moveSidebarLinkToGroup)).toMatchObject({
      linkId: 'docs',
      groupId: 'g1'
    });
  });

  it('forwards cancellation signals for room detail snapshots', async () => {
    const api = roomLayoutAPI();

    await expect(api.getRoom('r1', { signal: AbortSignal.abort() })).rejects.toMatchObject({
      code: Code.Canceled
    });
    await expect(api.getRoomGroup('g1', { signal: AbortSignal.abort() })).rejects.toMatchObject({
      code: Code.Canceled
    });
  });

  it('propagates Connect errors', async () => {
    mocks.createRoomGroup.mockImplementation(() => {
      throw new ConnectError('authentication required', Code.Unauthenticated);
    });

    await expect(roomLayoutAPI().createRoomGroup({ name: 'Projects' })).rejects.toMatchObject({
      code: Code.Unauthenticated,
      rawMessage: 'authentication required'
    });
  });
});
