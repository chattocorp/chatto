import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import { RoomInvitationService } from '@chatto/api-types/api/v1/room_invitations_connect';
import { RoomInvitationStatus } from '@chatto/api-types/api/v1/room_invitations_pb';
import { createRoomInvitationAPI } from './roomInvitations.js';
import { fakeServer, mockService, receivedRequest } from '../testing/fakeServer.js';

const mocks = mockService(RoomInvitationService);
const api = () =>
  createRoomInvitationAPI(fakeServer((router) => router.service(RoomInvitationService, mocks)));

describe('room invitation commands', () => {
  beforeEach(() => vi.resetAllMocks());
  it('creates an invitation without performing membership commands', async () => {
    mocks.createRoomInvitation.mockReturnValue({
      invitation: {
        id: 'I1',
        roomId: 'R1',
        recipientId: 'U2',
        status: RoomInvitationStatus.PENDING
      }
    });
    const invitation = await api().createInvitation('R1', 'U2');
    expect(invitation?.status).toBe(RoomInvitationStatus.PENDING);
    expect(receivedRequest(mocks.createRoomInvitation)).toMatchObject({
      roomId: 'R1',
      recipientId: 'U2'
    });
    expect(mocks.acceptRoomInvitation).not.toHaveBeenCalled();
  });
  it('preserves an authorization failure when acceptance is no longer allowed', async () => {
    mocks.acceptRoomInvitation.mockImplementation(() => {
      throw new ConnectError('Invitation unavailable', Code.PermissionDenied);
    });
    await expect(api().acceptInvitation('I1')).rejects.toMatchObject({
      code: Code.PermissionDenied
    });
    expect(mocks.declineRoomInvitation).not.toHaveBeenCalled();
  });
  it('declines using the invitation identity', async () => {
    mocks.declineRoomInvitation.mockReturnValue({
      invitation: { id: 'I1', status: RoomInvitationStatus.DECLINED }
    });
    expect((await api().declineInvitation('I1'))?.status).toBe(RoomInvitationStatus.DECLINED);
    expect(receivedRequest(mocks.declineRoomInvitation)).toMatchObject({ invitationId: 'I1' });
  });
  it('cancels reads through the caller signal', async () => {
    await expect(api().getInvitation('I1', { signal: AbortSignal.abort() })).rejects.toMatchObject({
      code: Code.Canceled
    });
  });
});
