import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { RoomInvitationService } from '@chatto/api-types/api/v1/room_invitations_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { mockService, receivedRequest } from '$lib/test-utils';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import type { RoomsListItem } from '$lib/state/server/navigation';
import InviteRoomMemberDialog from './InviteRoomMemberDialog.svelte';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
const invitations = mockService(RoomInvitationService);
const users = mockService(UserService);
const rooms = mockService(RoomService);
const room: RoomsListItem = {
  id: 'room-1',
  name: 'Project',
  type: RoomKind.CHANNEL,
  isUniversal: false,
  viewerIsMember: true,
  viewerCanJoinRoom: true,
  viewerCanManageRoom: false,
  viewerCanInvite: true,
  viewerNotificationCount: 0,
  viewerImportantNotificationCount: 0,
  members: []
};

beforeEach(() => {
  vi.clearAllMocks();
  createTestServerScope({
    routes: (router) => {
      router.service(RoomInvitationService, invitations);
      router.service(UserService, users);
      router.service(RoomService, rooms);
    }
  });
  users.listUsers.mockReturnValue({
    users: [{ user: { id: 'recipient-1', login: 'bob', displayName: 'Bob' } }],
    page: { hasMore: false, totalCount: 1n }
  });
  rooms.batchGetMembers.mockReturnValue({ members: [] });
  invitations.createRoomInvitation.mockReturnValue({ invitation: { id: 'invite-1' } });
});

describe('Invite room member dialog', () => {
  it('searches existing members and submits the selected recipient', async () => {
    const onclose = vi.fn();
    render(InviteRoomMemberDialog, { room, onclose });
    const submit = page.getByRole('button', { name: 'Invite member', exact: true });
    await expect.element(submit).toBeDisabled();
    await page.getByRole('combobox', { name: 'Invite member' }).fill('bob');
    await page.getByRole('option').click();
    await submit.click();
    await vi.waitFor(() => expect(invitations.createRoomInvitation).toHaveBeenCalledOnce());
    expect(receivedRequest(invitations.createRoomInvitation)).toMatchObject({
      roomId: 'room-1',
      recipientId: 'recipient-1'
    });
    expect(onclose).toHaveBeenCalledOnce();
  });
});
