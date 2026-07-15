import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import '../../app.css';
import { q } from '$lib/test-utils';
import { PresenceStatus } from '$lib/render/types';
import { presencePreference } from '$lib/state/presencePreference.svelte';
import {
  consumePendingRoomSidebarPanel,
  getRoomSidebarPanelState,
  roomSidebarPanelStorageSuffix
} from '$lib/storage/roomSidebarPanel';
import { serverStorageKey } from '$lib/storage/serverStorage';
import CurrentUserBarTestHarness from './CurrentUserBarTestHarness.svelte';

function computedBackgroundColor(color: string): string {
  const element = document.createElement('span');
  element.style.backgroundColor = color;
  document.body.append(element);
  const computed = window.getComputedStyle(element).backgroundColor;
  element.remove();
  return computed;
}

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
    isMicrophonePending: false,
    isCameraEnabled: false,
    isCameraPending: false,
    isScreenShareEnabled: false,
    isScreenSharePending: false,
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
    presencePreference.mode = 'auto';
    presencePreference.effectiveStatus = PresenceStatus.Online;
    voiceCallState.connected = false;
    voiceCallState.roomId = null;
    voiceCallState.isMuted = false;
    voiceCallState.isMicrophonePending = false;
    voiceCallState.isCameraEnabled = false;
    voiceCallState.isCameraPending = false;
    voiceCallState.isScreenShareEnabled = false;
    voiceCallState.isScreenSharePending = false;
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

    expect(q(container, '[aria-label="Presence: Online"]')).toBeTruthy();
    expect(q(container, '[aria-label="Offline"]')).toBeFalsy();
    const presenceDot = q(
      container,
      '[data-testid="current-user-presence-menu"] [aria-label="Online"] span'
    )!;
    expect(presenceDot.className).toContain('bg-presence-online');
    expect(container.textContent).toContain('Alice');
    expect(container.textContent).toContain('@alice');
  });

  it('uses the presence cache instead of local presence preference for the current user dot', () => {
    presencePreference.effectiveStatus = PresenceStatus.Away;

    const { container } = render(CurrentUserBarTestHarness);

    expect(q(container, '[aria-label="Presence: Online"]')).toBeTruthy();
    const presenceDot = q(
      container,
      '[data-testid="current-user-presence-menu"] [aria-label="Online"] span'
    )!;
    expect(presenceDot.className).toContain('bg-presence-online');
    expect(presenceDot.className).not.toContain('bg-presence-away');
  });

  it('renders the current user dot from the seeded away presence cache value', () => {
    presencePreference.effectiveStatus = PresenceStatus.Online;

    const { container } = render(CurrentUserBarTestHarness, {
      cachedPresence: PresenceStatus.Away
    });

    expect(q(container, '[aria-label="Presence: Away"]')).toBeTruthy();
    const presenceDot = q(
      container,
      '[data-testid="current-user-presence-menu"] [aria-label="Away"] span'
    )!;
    expect(presenceDot.className).toContain('bg-presence-away');
  });

  it('keeps the username line when display name and username match', () => {
    currentUserState.user = {
      ...currentUserState.user!,
      displayName: 'alice',
      login: 'alice'
    };

    const { container } = render(CurrentUserBarTestHarness);

    const card = q(container, '[data-testid="current-user-identity-card"]')!;
    expect(card.textContent).toContain('alice');
    expect(card.textContent).toContain('@alice');
  });

  it('opens the combined presence menu with a custom status action from the avatar', async () => {
    const { container } = render(CurrentUserBarTestHarness);

    (q(container, '[data-testid="current-user-presence-menu"]') as HTMLButtonElement).click();
    await vi.waitFor(() => {
      expect(container.textContent).toContain('Do Not Disturb');
      expect(container.textContent).toContain('Look offline');
      expect(container.textContent).toContain('Set custom status');
      expect(q(container, '[data-testid="custom-status-editor"]')).toBeFalsy();
    });
    expect(q(container, '[data-testid="current-user-edit-status"]')).toBeFalsy();
  });

  it('renders the away presence menu dot in yellow', async () => {
    const { container } = render(CurrentUserBarTestHarness);

    (q(container, '[data-testid="current-user-presence-menu"]') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      const awayOption = Array.from(container.querySelectorAll('[role="menuitemradio"]')).find(
        (item) => item.textContent?.includes('Away')
      )!;
      const awayDot = awayOption.querySelector('.rounded-full')!;
      const yellow500 = window
        .getComputedStyle(document.documentElement)
        .getPropertyValue('--color-yellow-500')
        .trim();

      expect(awayDot.className).toContain('bg-presence-away');
      expect(window.getComputedStyle(awayDot).backgroundColor).toBe(
        computedBackgroundColor(yellow500)
      );
    });
  });

  it('closes the presence menu after choosing a presence mode', async () => {
    const { container } = render(CurrentUserBarTestHarness);

    (q(container, '[data-testid="current-user-presence-menu"]') as HTMLButtonElement).click();
    await vi.waitFor(() => {
      expect(container.textContent).toContain('Away');
    });

    (q(container, '[role="menuitemradio"][aria-checked="false"]') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(container.textContent).not.toContain('Do Not Disturb');
    });
    expect(presencePreference.mode).toBe('away');
  });

  it('opens the custom status dialog from the status menu', async () => {
    currentUserState.user = {
      ...currentUserState.user!,
      customStatus: {
        emoji: '🍜',
        text: 'chatto:status:out_for_lunch',
        expiresAt: null
      }
    };

    const { container } = render(CurrentUserBarTestHarness);

    (q(container, '[data-testid="current-user-presence-menu"]') as HTMLButtonElement).click();
    await vi.waitFor(() => {
      expect(q(container, '[data-testid="current-user-custom-status-action"]')).toBeTruthy();
    });

    (
      q(container, '[data-testid="current-user-custom-status-action"]') as HTMLButtonElement
    ).click();

    await vi.waitFor(() => {
      expect(container.textContent).toContain('Set a status');
      expect(container.textContent).toContain('Suggestions');
      expect(container.textContent).toContain('Clear Status');
      expect(q(container, '[data-testid="custom-status-editor"]')).toBeTruthy();
    });
  });

  it('shows the custom status emoji next to the display name, not on the avatar', () => {
    currentUserState.user = {
      ...currentUserState.user!,
      customStatus: {
        emoji: '🍜',
        text: 'chatto:status:out_for_lunch',
        expiresAt: null
      }
    };

    const { container } = render(CurrentUserBarTestHarness);

    expect(container.querySelectorAll('[aria-label="🍜 Out for lunch"]')).toHaveLength(1);
    expect(q(container, '[data-testid="current-user-identity-card"]')!.textContent).toContain('🍜');
    expect(q(container, '[data-testid="current-user-identity-card"]')!.textContent).not.toContain(
      'Out for lunch'
    );
  });

  it('keeps the identity card at the same control height with long profile content', () => {
    currentUserState.user = {
      ...currentUserState.user!,
      login: 'alice-with-a-very-long-login-name-that-must-truncate',
      displayName: 'Alice With A Very Long Display Name That Must Stay Inside The User Card',
      customStatus: {
        emoji: '🍜',
        text: 'chatto:status:out_for_lunch',
        expiresAt: null
      }
    };

    const { container } = render(CurrentUserBarTestHarness);
    const bar = container.firstElementChild as HTMLElement;
    bar.style.width = '224px';

    const card = q(container, '[data-testid="current-user-identity-card"]')!;
    const cardRect = card.getBoundingClientRect();
    const controlReference = document.createElement('div');
    controlReference.className = 'h-12';
    document.body.append(controlReference);
    const expectedHeight = controlReference.getBoundingClientRect().height;
    controlReference.remove();

    expect(expectedHeight).toBeGreaterThan(0);
    expect(cardRect.height).toBe(expectedHeight);
    expect(card.scrollHeight).toBeLessThanOrEqual(card.clientHeight);

    for (const child of Array.from(card.children)) {
      const rect = child.getBoundingClientRect();
      expect(rect.top).toBeGreaterThanOrEqual(cardRect.top);
      expect(rect.bottom).toBeLessThanOrEqual(cardRect.bottom);
    }

    const presenceButton = q(card, '[data-testid="current-user-presence-menu"]')!;
    const avatar = q(presenceButton, '[aria-label]')!;
    const identityText = q(card, '[data-testid="current-user-identity-text"]')!;
    const settingsLink = q(card, 'a[href$="/settings"]')!;
    const presenceRect = presenceButton.getBoundingClientRect();
    const avatarRect = avatar.getBoundingClientRect();
    const textRect = identityText.getBoundingClientRect();
    const settingsRect = settingsLink.getBoundingClientRect();

    expect(presenceRect.left).toBeGreaterThanOrEqual(cardRect.left);
    expect(avatarRect.height).toBeLessThan(cardRect.height);
    expect(avatarRect.top - cardRect.top).toBeGreaterThanOrEqual(6);
    expect(cardRect.bottom - avatarRect.bottom).toBeGreaterThanOrEqual(6);
    expect(textRect.left).toBeGreaterThan(presenceRect.right);
    expect(settingsRect.left).toBeGreaterThan(textRect.right);
    expect(settingsRect.right).toBeLessThanOrEqual(cardRect.right);
    expect(textRect.left - presenceRect.right).toBeLessThanOrEqual(12);

    const settingsIcon = q(card, 'a[href$="/settings"] .iconify')!;
    const settingsIconRect = settingsIcon.getBoundingClientRect();
    expect(settingsIconRect.height).toBeLessThan(cardRect.height / 2);
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
    expect(link.getAttribute('aria-label')).toBe('Open # general');
    expect(link.textContent?.trim()).toBe('');
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
    const leaveButton = q(
      container,
      '[data-testid="current-user-call-leave"]'
    ) as HTMLButtonElement;

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

  it('aligns the equal-width call controls with the user card', () => {
    voiceCallState.connected = true;
    voiceCallState.roomId = 'room-1';

    const { container } = render(CurrentUserBarTestHarness);
    const bar = container.firstElementChild as HTMLElement;
    bar.style.width = '224px';

    const callCard = q(container, '[data-testid="current-user-call-card"]')!;
    const identityCard = q(container, '[data-testid="current-user-identity-card"]')!;
    const callCardRect = callCard.getBoundingClientRect();
    const identityCardRect = identityCard.getBoundingClientRect();
    const controlWidths = Array.from(callCard.children, (control) =>
      control.getBoundingClientRect().width
    );

    expect(callCardRect.left).toBe(identityCardRect.left);
    expect(callCardRect.right).toBe(identityCardRect.right);
    expect(controlWidths).toHaveLength(5);
    expect(controlWidths.every((width) => width === controlWidths[0])).toBe(true);
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

  it('shows spinners on pending compact call media controls', () => {
    voiceCallState.connected = true;
    voiceCallState.roomId = 'room-1';
    voiceCallState.isMicrophonePending = true;
    voiceCallState.isCameraPending = true;
    voiceCallState.isScreenSharePending = true;

    const { container } = render(CurrentUserBarTestHarness);

    for (const testId of [
      'current-user-call-mute',
      'current-user-call-camera',
      'current-user-call-screen-share'
    ]) {
      const button = q(container, `[data-testid="${testId}"]`) as HTMLButtonElement;
      expect(button.disabled).toBe(true);
      expect(button.getAttribute('aria-busy')).toBe('true');
      expect(q(button, '.animate-spin.uil--spinner')).toBeTruthy();
    }
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
    expect(callLink!.getAttribute('aria-label')).toBe('Open Bob');
    expect(callLink!.textContent?.trim()).toBe('');
  });
});
