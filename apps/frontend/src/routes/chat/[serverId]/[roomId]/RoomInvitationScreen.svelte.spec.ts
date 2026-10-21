import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { ConnectError, Code } from '@connectrpc/connect';
import { RoomInvitationService } from '@chatto/api-types/api/v1/room_invitations_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { mockService, receivedRequest } from '$lib/test-utils';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import type { RoomsListItem } from '$lib/state/server/navigation';
import RoomInvitationScreen from './RoomInvitationScreen.svelte';

const { goto, startDMWith } = vi.hoisted(() => ({ goto: vi.fn(), startDMWith: vi.fn() }));
vi.mock('$lib/dm/startDM', () => ({ startDMWith }));
vi.mock('$app/navigation', () => ({
  goto,
  pushState: vi.fn(),
  replaceState: vi.fn(),
  beforeNavigate: vi.fn(),
  afterNavigate: vi.fn()
}));
vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
// The route normally supplies live-profile context; these menu tests use the fetched sender snapshot.
vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveBotOwnerUserId: (_id: string, fallback: string | null) => fallback,
  getLiveBio: (_id: string, fallback: string | null) => fallback,
  getLiveTimezone: (_id: string, fallback: string | null) => fallback,
  getLiveDisplayName: (_id: string, fallback: string) => fallback,
  getLiveLogin: (_id: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_id: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_id: string, fallback: unknown) => fallback
}));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
const invitations = mockService(RoomInvitationService);
const users = mockService(UserService);

function room(canAccept = true): RoomsListItem {
  return {
    id: 'room-1',
    name: 'Private project',
    type: RoomKind.CHANNEL,
    isUniversal: false,
    viewerIsMember: false,
    viewerCanJoinRoom: false,
    viewerCanManageRoom: false,
    viewerCanInvite: false,
    viewerNotificationCount: 0,
    viewerImportantNotificationCount: 0,
    members: [],
    viewerInvitation: {
      id: 'invite-1',
      senderId: 'sender-1',
      expiresAt: new Date('2099-01-01'),
      viewerCanAccept: canAccept
    }
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  createTestServerScope({
    routes: (router) => {
      router.service(RoomInvitationService, invitations);
      router.service(UserService, users);
    }
  });
  users.getUser.mockReturnValue({
    user: { user: { id: 'sender-1', login: 'alice', displayName: 'Alice' } }
  });
  invitations.acceptRoomInvitation.mockReturnValue({ invitation: { id: 'invite-1' } });
  invitations.declineRoomInvitation.mockReturnValue({ invitation: { id: 'invite-1' } });
});

describe('Room invitation welcome screen', () => {
  it('shows the inviter and accepts without requiring the ordinary join permission', async () => {
    render(RoomInvitationScreen, { room: room(), serverSegment: '-' });
    await expect.element(page.getByTestId('user-identity')).toBeVisible();
    await expect.element(page.getByText('Expires on', { exact: false })).toBeVisible();
    await page.getByRole('button', { name: 'Accept invitation' }).click();
    await vi.waitFor(() => expect(invitations.acceptRoomInvitation).toHaveBeenCalledOnce());
    expect(receivedRequest(invitations.acceptRoomInvitation)).toMatchObject({
      invitationId: 'invite-1'
    });
    expect(invitations.declineRoomInvitation).not.toHaveBeenCalled();
    expect(goto).not.toHaveBeenCalled();
  });

  it.each([false, true])(
    'explains later access before declining, join permission=%s',
    async (canJoin) => {
      render(RoomInvitationScreen, {
        room: { ...room(), viewerCanJoinRoom: canJoin },
        serverSegment: '-'
      });
      await page.getByRole('button', { name: 'Decline', exact: true }).click();
      const dialog = page.getByRole('dialog');
      await expect
        .element(dialog)
        .toHaveTextContent(
          canJoin
            ? 'You can join this room later.'
            : 'You will need a new invitation to join this room.'
        );
      await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
      await expect.element(dialog).not.toBeInTheDocument();
      expect(invitations.declineRoomInvitation).not.toHaveBeenCalled();
    }
  );

  it.each([false, true])(
    'offers inviter conversation only when permitted=%s',
    async (canStartDMs) => {
      createTestServerScope({
        serverId: 'invitation-server',
        permissions: { canStartDMs },
        routes: (router) => {
          router.service(RoomInvitationService, invitations);
          router.service(UserService, users);
        }
      });
      render(RoomInvitationScreen, { room: room(), serverSegment: '-' });
      await page.getByTestId('user-identity').click();
      await expect
        .element(page.getByRole('button', { name: 'View profile', exact: true }))
        .toBeVisible();
      const conversation = page.getByRole('button', { name: 'Open conversation', exact: true });
      if (canStartDMs) {
        await conversation.click();
        expect(startDMWith).toHaveBeenCalledWith('invitation-server', 'sender-1');
      } else {
        await expect.element(conversation).not.toBeInTheDocument();
        expect(startDMWith).not.toHaveBeenCalled();
      }
    }
  );

  it('keeps decline available when acceptance is unavailable', async () => {
    render(RoomInvitationScreen, { room: room(false), serverSegment: '-' });
    await expect.element(page.getByRole('button', { name: 'Accept invitation' })).toBeDisabled();
    await expect
      .element(page.getByText('This invitation cannot be accepted at present.', { exact: false }))
      .toBeVisible();
    await page.getByRole('button', { name: 'Decline', exact: true }).click();
    expect(invitations.declineRoomInvitation).not.toHaveBeenCalled();
    await page.getByRole('dialog').getByRole('button', { name: 'Decline', exact: true }).click();
    await vi.waitFor(() => expect(invitations.declineRoomInvitation).toHaveBeenCalledOnce());
    expect(receivedRequest(invitations.declineRoomInvitation)).toMatchObject({
      invitationId: 'invite-1'
    });
    await vi.waitFor(() => expect(goto).toHaveBeenCalled());
  });

  it('keeps the invitation screen when decline fails', async () => {
    invitations.declineRoomInvitation.mockRejectedValue(
      new ConnectError('Try again', Code.Unavailable)
    );
    render(RoomInvitationScreen, { room: room(), serverSegment: '-' });
    await page.getByRole('button', { name: 'Decline', exact: true }).click();
    expect(invitations.declineRoomInvitation).not.toHaveBeenCalled();
    await page.getByRole('dialog').getByRole('button', { name: 'Decline', exact: true }).click();
    await expect.element(page.getByRole('alert')).toBeVisible();
    await expect
      .element(page.getByRole('dialog').getByRole('button', { name: 'Decline', exact: true }))
      .toBeEnabled();
    expect(goto).not.toHaveBeenCalled();
  });

  it('does not navigate when a decline finishes after the session ends', async () => {
    let finish!: () => void;
    invitations.declineRoomInvitation.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = () => resolve({ invitation: { id: 'invite-1' } });
        })
    );
    const scope = createTestServerScope({
      routes: (router) => {
        router.service(RoomInvitationService, invitations);
        router.service(UserService, users);
      }
    });
    render(RoomInvitationScreen, { room: room(), serverSegment: '-' });
    await page.getByRole('button', { name: 'Decline', exact: true }).click();
    expect(invitations.declineRoomInvitation).not.toHaveBeenCalled();
    await page.getByRole('dialog').getByRole('button', { name: 'Decline', exact: true }).click();
    await vi.waitFor(() => expect(invitations.declineRoomInvitation).toHaveBeenCalledOnce());
    scope.current = false;
    finish();
    await vi.waitFor(() =>
      expect(invitations.declineRoomInvitation.mock.results[0]?.value).resolves.toBeDefined()
    );
    expect(goto).not.toHaveBeenCalled();
  });

  it('shows a failed acceptance and allows retry or decline', async () => {
    invitations.acceptRoomInvitation.mockRejectedValue(
      new ConnectError('Invitation unavailable', Code.FailedPrecondition)
    );
    render(RoomInvitationScreen, { room: room(), serverSegment: '-' });
    await page.getByRole('button', { name: 'Accept invitation' }).click();
    await expect.element(page.getByRole('alert')).toBeVisible();
    await expect.element(page.getByRole('button', { name: 'Decline', exact: true })).toBeEnabled();
    expect(goto).not.toHaveBeenCalled();
  });
});
