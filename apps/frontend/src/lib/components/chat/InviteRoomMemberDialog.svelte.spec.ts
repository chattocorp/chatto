import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { getToasts, toast } from '$lib/ui/toast';
import { ConnectError, Code } from '@connectrpc/connect';
import { RoomInvitationService } from '@chatto/api-types/api/v1/room_invitations_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { mockService, receivedRequest } from '$lib/test-utils';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import type { RoomsListItem } from '$lib/state/server/navigation';
import InviteRoomMemberDialog from './InviteRoomMemberDialog.svelte';
import { UserStore } from '@chatto/client/server/users';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';

let profiles: UserStore;

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
  toast.clear();
  profiles = new UserStore();
  createTestServerScope({
    store: { projection: { users: profiles } },
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
  it('hides and restores cached candidates on account status changes', async () => {
    render(InviteRoomMemberDialog, { room, onclose: vi.fn() });
    await page.getByRole('combobox', { name: 'Members' }).fill('bob');
    await expect.element(page.getByRole('option')).toBeVisible();
    profiles.set('recipient-1', new DirectoryMember({ user: { id: 'recipient-1', login: 'bob' } }));
    profiles.setDeactivated('recipient-1', true);
    await expect.element(page.getByRole('option')).not.toBeInTheDocument();
    profiles.setDeactivated('recipient-1', false);
    await expect.element(page.getByRole('option')).toBeVisible();
  });
  it('searches existing members and submits the selected recipient', async () => {
    const onclose = vi.fn();
    render(InviteRoomMemberDialog, { room, onclose });
    const submit = page.getByRole('button', { name: 'Send invitations', exact: true });
    await expect.element(submit).toBeDisabled();
    await page.getByRole('combobox', { name: 'Members' }).fill('bob');
    await page.getByRole('option').click();
    const pill = page.getByTestId('combobox-selection');
    await expect.element(pill).toBeVisible();
    expect(pill.element().querySelector('[role="img"][aria-label="bob"]')).not.toBeNull();
    await submit.click();
    await vi.waitFor(() => expect(invitations.createRoomInvitation).toHaveBeenCalledOnce());
    expect(receivedRequest(invitations.createRoomInvitation)).toMatchObject({
      roomId: 'room-1',
      recipientId: 'recipient-1'
    });
    expect(onclose).toHaveBeenCalledOnce();
  });
  it('shows current room members as unavailable with an explanation', async () => {
    rooms.batchGetMembers.mockReturnValue({
      members: [{ user: { id: 'recipient-1', login: 'bob', displayName: 'Bob' } }]
    });
    render(InviteRoomMemberDialog, { room, onclose: vi.fn() });
    await page.getByRole('combobox', { name: 'Members' }).fill('bob');
    const option = page.getByRole('option');
    await expect.element(option).toBeDisabled();
    await expect.element(option).toHaveTextContent('Already in this room');
    await expect.element(page.getByRole('button', { name: 'Send invitations' })).toBeDisabled();
    expect(invitations.createRoomInvitation).not.toHaveBeenCalled();
  });

  it('keeps failed recipients selected and retries only them', async () => {
    const onclose = vi.fn();
    users.listUsers.mockReturnValue({
      users: [
        { user: { id: 'bob', login: 'bob', displayName: 'Bob' } },
        { user: { id: 'carol', login: 'carol', displayName: 'Carol' } }
      ],
      page: { hasMore: false, totalCount: 2n }
    });
    invitations.createRoomInvitation.mockImplementation((request) => {
      if (request.recipientId === 'carol')
        throw new ConnectError('Please try again', Code.Unavailable);
      return { invitation: { id: 'invite-bob' } };
    });
    render(InviteRoomMemberDialog, { room, onclose });
    await expect
      .element(page.getByRole('dialog', { name: 'Invite members to #Project' }))
      .toBeVisible();
    await expect
      .element(page.getByText('Invited users will receive a notification', { exact: false }))
      .toBeVisible();
    const search = page.getByRole('combobox', { name: 'Members' });
    await search.fill('b');
    await page.getByRole('option', { name: 'Bob', exact: false }).click();
    await search.fill('c');
    await page.getByRole('option', { name: 'Carol', exact: false }).click();
    await page.getByRole('button', { name: 'Send invitations' }).click();
    await expect.element(page.getByRole('alert')).toHaveTextContent('Could not invite Carol');
    await expect.element(page.getByRole('button', { name: 'Remove Bob' })).not.toBeInTheDocument();
    await expect.element(page.getByRole('button', { name: 'Remove Carol' })).toBeVisible();
    expect(onclose).not.toHaveBeenCalled();
    expect(
      invitations.createRoomInvitation.mock.calls.map(([request]) => request.recipientId)
    ).toEqual(['bob', 'carol']);
    invitations.createRoomInvitation.mockReturnValue({ invitation: { id: 'invite-carol' } });
    await page.getByRole('button', { name: 'Send invitations' }).click();
    await vi.waitFor(() => expect(onclose).toHaveBeenCalledOnce());
    expect(
      invitations.createRoomInvitation.mock.calls.map(([request]) => request.recipientId)
    ).toEqual(['bob', 'carol', 'carol']);
  });
  it('stops the batch when the dialog closes during its first request', async () => {
    const onclose = vi.fn();
    users.listUsers.mockReturnValue({
      users: [
        { user: { id: 'bob', login: 'bob', displayName: 'Bob' } },
        { user: { id: 'carol', login: 'carol', displayName: 'Carol' } }
      ],
      page: { hasMore: false, totalCount: 2n }
    });
    let finish!: () => void;
    invitations.createRoomInvitation.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = () => resolve({ invitation: { id: 'invite-bob' } });
        })
    );
    render(InviteRoomMemberDialog, { room, onclose });
    const search = page.getByRole('combobox', { name: 'Members' });
    await search.fill('b');
    await page.getByRole('option', { name: 'Bob', exact: false }).click();
    await search.fill('c');
    await page.getByRole('option', { name: 'Carol', exact: false }).click();
    await page.getByRole('button', { name: 'Send invitations' }).click();
    await vi.waitFor(() => expect(invitations.createRoomInvitation).toHaveBeenCalledOnce());
    await expect.element(search).toBeDisabled();
    (
      page.getByRole('button', { name: 'Close', exact: true }).element() as HTMLButtonElement
    ).click();
    expect(onclose).not.toHaveBeenCalled();
    finish();
    await vi.waitFor(() =>
      expect(invitations.createRoomInvitation.mock.settledResults[0]?.type).toBe('fulfilled')
    );
    // Let the browser flush the remaining Connect response and render microtasks.
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    expect(invitations.createRoomInvitation).toHaveBeenCalledOnce();
    expect(getToasts()).toEqual([]);
    await vi.waitFor(() => expect(onclose).toHaveBeenCalledOnce());
  });
});
