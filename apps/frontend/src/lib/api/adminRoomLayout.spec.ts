import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminRoomLayoutItemKind } from '$lib/pb/chatto/api/v1/admin_room_layout_pb';
import { createAdminRoomLayoutAPI } from './adminRoomLayout';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
  listAdminRoomLayout: vi.fn(),
  createRoomGroup: vi.fn(),
  updateRoomGroup: vi.fn(),
  deleteRoomGroup: vi.fn(),
  reorderRoomGroups: vi.fn(),
  moveRoomToGroup: vi.fn(),
  reorderSidebarItemsInGroup: vi.fn(),
  createSidebarLink: vi.fn(),
  updateSidebarLink: vi.fn(),
  deleteSidebarLink: vi.fn(),
  moveSidebarLinkToGroup: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return {
    ...actual,
    createClient: mocks.createClient
  };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    handleAuthenticationRequired: mocks.handleAuthenticationRequired
  }
}));

describe('createAdminRoomLayoutAPI', () => {
  beforeEach(() => {
    for (const mock of Object.values(mocks)) mock.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      listAdminRoomLayout: mocks.listAdminRoomLayout,
      createRoomGroup: mocks.createRoomGroup,
      updateRoomGroup: mocks.updateRoomGroup,
      deleteRoomGroup: mocks.deleteRoomGroup,
      reorderRoomGroups: mocks.reorderRoomGroups,
      moveRoomToGroup: mocks.moveRoomToGroup,
      reorderSidebarItemsInGroup: mocks.reorderSidebarItemsInGroup,
      createSidebarLink: mocks.createSidebarLink,
      updateSidebarLink: mocks.updateSidebarLink,
      deleteSidebarLink: mocks.deleteSidebarLink,
      moveSidebarLinkToGroup: mocks.moveSidebarLinkToGroup
    });
  });

  it('lists admin room layout groups and maps mixed sidebar items', async () => {
    mocks.listAdminRoomLayout.mockResolvedValue({
      groups: [
        {
          id: 'g1',
          name: 'Lobby',
          rooms: [
            {
              id: 'general',
              name: 'general',
              description: 'Public room',
              archived: false,
              universal: true
            }
          ],
          items: [
            {
              item: {
                case: 'sidebarLink',
                value: { id: 'docs', label: 'Docs', url: '/docs' }
              }
            },
            {
              item: {
                case: 'room',
                value: {
                  id: 'general',
                  name: 'general',
                  description: 'Public room',
                  archived: false,
                  universal: true
                }
              }
            }
          ]
        }
      ]
    });

    const api = createAdminRoomLayoutAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'token'
    });

    await expect(api.listAdminRoomLayout()).resolves.toEqual([
      {
        id: 'g1',
        name: 'Lobby',
        rooms: [
          {
            id: 'general',
            name: 'general',
            description: 'Public room',
            archived: false,
            isUniversal: true
          }
        ],
        items: [
          { id: 'link:docs', kind: 'link', link: { id: 'docs', label: 'Docs', url: '/docs' } },
          {
            id: 'room:general',
            kind: 'room',
            room: {
              id: 'general',
              name: 'general',
              description: 'Public room',
              archived: false,
              isUniversal: true
            }
          }
        ]
      }
    ]);
    expect(mocks.listAdminRoomLayout).toHaveBeenCalledWith(
      {},
      { headers: { Authorization: 'Bearer token' } }
    );
  });

  it('uses room fallback items and maps empty descriptions to null', async () => {
    mocks.listAdminRoomLayout.mockResolvedValue({
      groups: [
        {
          id: 'g1',
          name: 'Lobby',
          rooms: [{ id: 'general', name: 'general', description: '', archived: false }],
          items: []
        }
      ]
    });

    const api = createAdminRoomLayoutAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.listAdminRoomLayout()).resolves.toMatchObject([
      {
        rooms: [{ id: 'general', description: null, isUniversal: false }],
        items: [{ id: 'room:general', kind: 'room' }]
      }
    ]);
  });

  it('sends group, room, link, and reorder commands through Connect', async () => {
    mocks.createRoomGroup.mockResolvedValue({ group: { id: 'g2', name: 'Projects', rooms: [] } });
    mocks.updateRoomGroup.mockResolvedValue({ group: { id: 'g2', name: 'Renamed', rooms: [] } });
    mocks.deleteRoomGroup.mockResolvedValue({ deleted: true });
    mocks.reorderRoomGroups.mockResolvedValue({ groups: [] });
    mocks.moveRoomToGroup.mockResolvedValue({});
    mocks.reorderSidebarItemsInGroup.mockResolvedValue({ group: undefined });
    mocks.createSidebarLink.mockResolvedValue({
      sidebarLink: { id: 'docs', label: 'Docs', url: '/docs' }
    });
    mocks.updateSidebarLink.mockResolvedValue({
      sidebarLink: { id: 'docs', label: 'Docs', url: '/help' }
    });
    mocks.deleteSidebarLink.mockResolvedValue({ deleted: true });
    mocks.moveSidebarLinkToGroup.mockResolvedValue({});

    const api = createAdminRoomLayoutAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'token'
    });

    await api.createRoomGroup({ name: 'Projects' });
    await api.updateRoomGroup({ groupId: 'g2', name: 'Renamed' });
    await api.deleteRoomGroup('g2');
    await api.reorderRoomGroups(['g2', 'g1']);
    await api.moveRoomToGroup({ roomId: 'room-1', groupId: 'g2' });
    await api.reorderSidebarItemsInGroup({
      groupId: 'g2',
      items: [
        { kind: 'room', id: 'room-1' },
        { kind: 'link', id: 'docs' }
      ]
    });
    await api.createSidebarLink({ groupId: 'g2', label: 'Docs', url: '/docs' });
    await api.updateSidebarLink({ linkId: 'docs', label: 'Docs', url: '/help' });
    await api.deleteSidebarLink('docs');
    await api.moveSidebarLinkToGroup({ linkId: 'docs', groupId: 'g1' });

    const callOptions = { headers: { Authorization: 'Bearer token' } };
    expect(mocks.createRoomGroup).toHaveBeenCalledWith(
      { name: 'Projects', description: '' },
      callOptions
    );
    expect(mocks.updateRoomGroup).toHaveBeenCalledWith(
      { groupId: 'g2', name: 'Renamed', description: '' },
      callOptions
    );
    expect(mocks.deleteRoomGroup).toHaveBeenCalledWith({ groupId: 'g2' }, callOptions);
    expect(mocks.reorderRoomGroups).toHaveBeenCalledWith(
      { orderedGroupIds: ['g2', 'g1'] },
      callOptions
    );
    expect(mocks.moveRoomToGroup).toHaveBeenCalledWith(
      { roomId: 'room-1', groupId: 'g2' },
      callOptions
    );
    expect(mocks.reorderSidebarItemsInGroup).toHaveBeenCalledWith(
      {
        groupId: 'g2',
        items: [
          { id: 'room-1', kind: AdminRoomLayoutItemKind.ROOM },
          { id: 'docs', kind: AdminRoomLayoutItemKind.SIDEBAR_LINK }
        ]
      },
      callOptions
    );
    expect(mocks.createSidebarLink).toHaveBeenCalledWith(
      { groupId: 'g2', label: 'Docs', url: '/docs' },
      callOptions
    );
    expect(mocks.updateSidebarLink).toHaveBeenCalledWith(
      { linkId: 'docs', label: 'Docs', url: '/help' },
      callOptions
    );
    expect(mocks.deleteSidebarLink).toHaveBeenCalledWith({ linkId: 'docs' }, callOptions);
    expect(mocks.moveSidebarLinkToGroup).toHaveBeenCalledWith(
      { linkId: 'docs', groupId: 'g1' },
      callOptions
    );
  });

  it('routes unauthenticated errors through the server registry', async () => {
    const err = new ConnectError('authentication required', Code.Unauthenticated);
    mocks.listAdminRoomLayout.mockRejectedValue(err);

    const api = createAdminRoomLayoutAPI({
      serverId: 'remote',
      baseUrl: '/api/connect',
      bearerToken: null
    });

    await expect(api.listAdminRoomLayout()).rejects.toBe(err);
    expect(mocks.handleAuthenticationRequired).toHaveBeenCalledWith('remote');
  });
});
