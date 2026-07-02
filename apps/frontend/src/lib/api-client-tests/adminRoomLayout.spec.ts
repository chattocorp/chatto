import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { configureApiClientHooks } from '$lib/api-client/hooks';
import { AdminRoomLayoutItemKind } from '@chatto/api-types/admin/v1/room_layout_pb';
import { createAdminRoomLayoutAPI } from '$lib/api-client/adminRoomLayout';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
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

describe('createAdminRoomLayoutAPI', () => {
  beforeEach(() => {
    for (const mock of Object.values(mocks)) mock.mockReset();
    configureApiClientHooks({ onAuthenticationRequired: mocks.handleAuthenticationRequired });
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
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

  it('sends group, room, link, and reorder commands through Connect', async () => {
    mocks.createRoomGroup.mockResolvedValue({ group: { id: 'g2', name: 'Projects', items: [] } });
    mocks.updateRoomGroup.mockResolvedValue({ group: { id: 'g2', name: 'Renamed', items: [] } });
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
    mocks.createRoomGroup.mockRejectedValue(err);

    const api = createAdminRoomLayoutAPI({
      serverId: 'remote',
      baseUrl: '/api/connect',
      bearerToken: null
    });

    await expect(api.createRoomGroup({ name: 'Projects' })).rejects.toBe(err);
    expect(mocks.handleAuthenticationRequired).toHaveBeenCalledWith('remote');
  });
});
