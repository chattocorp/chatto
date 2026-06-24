import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { PresenceStatus } from '$lib/gql/graphql';
import {
  consumePendingRoomSidebarPanel,
  getRoomSidebarPanelState,
  roomSidebarPanelStorageSuffix
} from '$lib/storage/roomSidebarPanel';
import { serverStorageKey } from '$lib/storage/serverStorage';
import CurrentUserBarTestHarness from './CurrentUserBarTestHarness.svelte';

type MockRoomMember = {
  id: string;
  login: string;
  displayName: string;
  avatarUrl: string | null;
  presenceStatus: PresenceStatus;
};

type MockRoom = {
  id: string;
  name: string;
  type: 'CHANNEL' | 'DM';
  members: MockRoomMember[];
};

const { currentUserState, voiceCallState, roomsState } = vi.hoisted(() => ({
  currentUserState: {
    user: null as {
      id: string;
      login: string;
      displayName: string;
      avatarUrl: string | null;
      presenceStatus: PresenceStatus;
      customStatus?: {
        emoji: string;
        text: string;
        expiresAt?: string | null;
      } | null;
      hasVerifiedEmail: boolean;
      settings: null;
    } | null
  },
  voiceCallState: {
    connected: false,
    roomId: null as string | null,
    isMuted: false,
    isCameraEnabled: false,
    isScreenShareEnabled: false,
    toggleMute: vi.fn(),
    toggleCamera: vi.fn(),
    toggleScreenShare: vi.fn(),
    leave: vi.fn()
  },
  roomsState: {
    currentUserId: 'user-1',
    rooms: [
      {
        id: 'room-1',
        name: 'general',
        type: 'CHANNEL',
        members: []
      }
    ] as MockRoom[]
  }
}));
const navigation = vi.hoisted(() => ({
  goto: vi.fn(),
  pushState: vi.fn()
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    connectBaseUrl: 'https://chat.example.test',
    bearerToken: 'token'
  })
}));

vi.mock('$app/navigation', () => ({
  goto: navigation.goto,
  pushState: navigation.pushState
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    isOriginServer: () => true,
    tryGetStore: () => ({
      currentUser: currentUserState,
      voiceCall: voiceCallState,
      rooms: roomsState
    })
  }
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback
}));

describe('CurrentUserBar', () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
    currentUserState.user = {
      id: 'user-1',
      login: 'alice',
      displayName: 'Alice',
      avatarUrl: null,
      presenceStatus: PresenceStatus.Offline,
      customStatus: null,
      hasVerifiedEmail: true,
      settings: null
    };
    voiceCallState.connected = false;
    voiceCallState.roomId = null;
    voiceCallState.isMuted = false;
    voiceCallState.isCameraEnabled = false;
    voiceCallState.isScreenShareEnabled = false;
    voiceCallState.toggleMute.mockClear();
    voiceCallState.toggleCamera.mockClear();
    voiceCallState.toggleScreenShare.mockClear();
    voiceCallState.leave.mockClear();
    navigation.goto.mockClear();
    roomsState.currentUserId = 'user-1';
    roomsState.rooms = [
      {
        id: 'room-1',
        name: 'general',
        type: 'CHANNEL',
        members: []
      }
    ];
  });

  it('uses the seeded presence cache instead of the first-login offline fallback', () => {
    const { container } = render(CurrentUserBarTestHarness);

    expect(q(container, '[aria-label="Online"]')).toBeTruthy();
    expect(q(container, '[aria-label="Offline"]')).toBeFalsy();
    expect(container.textContent).toContain('Alice');
    expect(container.textContent).toContain('@alice');
  });

  it('shows the custom status emoji as an avatar badge', () => {
    currentUserState.user = {
      ...currentUserState.user!,
      customStatus: {
        emoji: '🍜',
        text: 'chatto:status:out_for_lunch',
        expiresAt: null
      }
    };

    const { container } = render(CurrentUserBarTestHarness);

    expect(q(container, '[aria-label="🍜 Out for lunch"]')).toBeTruthy();
    expect(q(container, '[data-testid="current-user-identity-card"]')!.textContent).not.toContain(
      'Out for lunch'
    );
  });

  it('hides call controls when the user is not in a call', () => {
    const { container } = render(CurrentUserBarTestHarness);

    expect(container.querySelector('[data-testid="current-user-call-link"]')).toBeFalsy();
    expect(container.querySelector('[data-testid="current-user-call-mute"]')).toBeFalsy();
  });

  it('shows active call controls and links to the call room', async () => {
    voiceCallState.connected = true;
    voiceCallState.roomId = 'room-1';
    const storageEvents: StorageEvent[] = [];
    const listener = (event: StorageEvent) => storageEvents.push(event);
    window.addEventListener('storage', listener);

    const { container } = render(CurrentUserBarTestHarness);

    expect(q(container, '[data-testid="current-user-call-card"]')).toBeTruthy();
    expect(q(container, '[data-testid="current-user-identity-card"]')).toBeTruthy();
    const link = q(container, '[data-testid="current-user-call-link"]') as HTMLButtonElement;
    expect(link.textContent).toContain('# general');
    link.click();

    const muteButton = q(container, '[data-testid="current-user-call-mute"]') as HTMLButtonElement;
    const cameraButton = q(
      container,
      '[data-testid="current-user-call-camera"]'
    ) as HTMLButtonElement;
    const screenShareButton = q(
      container,
      '[data-testid="current-user-call-screen-share"]'
    ) as HTMLButtonElement;
    const leaveButton = q(container, '[data-testid="current-user-call-leave"]') as HTMLButtonElement;

    expect(muteButton.className).toContain('btn-success');
    expect(cameraButton.className).toContain('btn-secondary');
    expect(screenShareButton.className).toContain('btn-secondary');
    expect(leaveButton.className).toContain('btn-danger');

    muteButton.click();
    cameraButton.click();
    screenShareButton.click();
    leaveButton.click();

    expect(navigation.goto).toHaveBeenCalledWith('/chat/-/room-1');
    expect(getRoomSidebarPanelState('origin', 'room-1')).toBe('call');
    expect(consumePendingRoomSidebarPanel('origin', 'room-1')).toBe('call');
    expect(storageEvents).toHaveLength(1);
    expect(storageEvents[0].key).toBe(
      serverStorageKey('origin', roomSidebarPanelStorageSuffix('room-1'))
    );
    expect(storageEvents[0].newValue).toBe('call');
    expect(voiceCallState.toggleMute).toHaveBeenCalledOnce();
    expect(voiceCallState.toggleCamera).toHaveBeenCalledOnce();
    expect(voiceCallState.toggleScreenShare).toHaveBeenCalledOnce();
    expect(voiceCallState.leave).toHaveBeenCalledOnce();
    window.removeEventListener('storage', listener);
  });

  it('uses green only for active compact call media controls', () => {
    voiceCallState.connected = true;
    voiceCallState.roomId = 'room-1';
    voiceCallState.isMuted = true;
    voiceCallState.isCameraEnabled = true;
    voiceCallState.isScreenShareEnabled = true;

    const { container } = render(CurrentUserBarTestHarness);

    expect(q(container, '[data-testid="current-user-call-mute"]')!.className).toContain(
      'btn-secondary'
    );
    expect(q(container, '[data-testid="current-user-call-camera"]')!.className).toContain(
      'btn-success'
    );
    expect(q(container, '[data-testid="current-user-call-screen-share"]')!.className).toContain(
      'btn-success'
    );
    expect(q(container, '[data-testid="current-user-call-leave"]')!.className).toContain(
      'btn-danger'
    );
  });

  it('uses the DM participant label for active direct-message calls', () => {
    voiceCallState.connected = true;
    voiceCallState.roomId = 'dm-1';
    roomsState.rooms = [
      {
        id: 'dm-1',
        name: 'dm-1',
        type: 'DM',
        members: [
          {
            id: 'user-1',
            login: 'alice',
            displayName: 'Alice',
            avatarUrl: null,
            presenceStatus: PresenceStatus.Online
          },
          {
            id: 'user-2',
            login: 'bob',
            displayName: 'Bob',
            avatarUrl: null,
            presenceStatus: PresenceStatus.Online
          }
        ]
      }
    ];

    const { container } = render(CurrentUserBarTestHarness);

    const callLink = q(container, '[data-testid="current-user-call-link"]');
    expect(callLink).toBeTruthy();
    expect(callLink!.textContent ?? '').toContain('Bob');
  });
});
