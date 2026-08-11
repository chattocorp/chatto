import { describe, expect, it, vi } from 'vitest';
import { RoomKind } from '$lib/api-client/roomDirectory';
import type { DMData, RoomData } from '$lib/hooks/useRoomData.svelte';
import { buildRoomPresentation } from './roomPresentation';

function roomData(overrides: Partial<RoomData> = {}): RoomData {
  return {
    room: {
      id: 'room-1',
      name: 'general',
      type: RoomKind.CHANNEL,
      description: ' Room description ',
      isUniversal: false,
      slowModeSeconds: 0
    },
    spaceName: 'Test Space',
    canPostMessage: true,
    canPostInThread: true,
    canAttach: true,
    canReact: true,
    canManageOthersMessage: false,
    canEchoMessage: true,
    canManageRoom: false,
    canBanRoomMembers: false,
    slowModeNextPostAt: null,
    ...overrides
  };
}

function build(room: RoomData | null | undefined, isDM = false, dmData: DMData | null = null) {
  return buildRoomPresentation({
    roomData: room,
    isDM,
    dmData,
    directMessageLabel: 'Direct message',
    currentUserLabel: 'You',
    getDisplayName: (_userId, fallback) => `Live ${fallback}`
  });
}

describe('buildRoomPresentation', () => {
  it('builds channel header, description, and page title', () => {
    expect(build(roomData())).toEqual({
      title: '# general',
      description: 'Room description',
      pageTitle: '#general - Test Space'
    });
  });

  it('omits blank channel descriptions and uses the header title without a space name', () => {
    const room = roomData({
      room: {
        id: 'room-1',
        name: 'general',
        type: RoomKind.CHANNEL,
        description: ' ',
        isUniversal: false,
        slowModeSeconds: 0
      },
      spaceName: null
    });

    expect(build(room)).toEqual({
      title: '# general',
      description: undefined,
      pageTitle: '# general'
    });
  });

  it('builds a direct-message title from the other participants', () => {
    const getDisplayName = vi.fn((_userId: string, fallback: string) => `Live ${fallback}`);
    const presentation = buildRoomPresentation({
      roomData: roomData(),
      isDM: true,
      dmData: {
        currentUserId: 'self',
        participants: [
          {
            id: 'self',
            login: 'me',
            displayName: 'Me',
            presenceStatus: 0
          },
          {
            id: 'other',
            login: 'friend',
            displayName: 'Friend',
            presenceStatus: 0
          }
        ]
      },
      directMessageLabel: 'Direct message',
      currentUserLabel: 'You',
      getDisplayName
    });

    expect(presentation).toEqual({
      title: 'Live Friend',
      description: undefined,
      pageTitle: 'Live Friend'
    });
    expect(getDisplayName).toHaveBeenCalledWith('other', 'Friend');
  });

  it('uses the localized current-user label for a self direct message', () => {
    expect(
      build(roomData(), true, {
        currentUserId: 'self',
        participants: [
          {
            id: 'self',
            login: 'me',
            displayName: 'Me',
            presenceStatus: 0
          }
        ]
      })
    ).toEqual({
      title: 'You',
      description: undefined,
      pageTitle: 'You'
    });
  });

  it('uses the direct-message label while participant data is empty', () => {
    expect(build(roomData(), true, { currentUserId: 'self', participants: [] })).toEqual({
      title: 'Direct message',
      description: undefined,
      pageTitle: 'Direct message'
    });
  });

  it('returns an empty loading presentation without room data', () => {
    expect(build(undefined)).toEqual({
      title: '',
      description: undefined,
      pageTitle: ''
    });
    expect(build(null)).toEqual({
      title: '',
      description: undefined,
      pageTitle: ''
    });
  });
});
