import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import { q } from '$lib/test-utils';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { ROOM_MEMBERS_PAGE_SIZE, type RoomMember } from '$lib/state/room/members.svelte';
import type { PresenceCache } from '$lib/state/presenceCache.svelte';
import type { RoomData } from '$lib/hooks/useRoomData.svelte';
import { PresenceStatus } from '$lib/render/types';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import RoomSidebarTestHarness from './RoomSidebarTestHarness.svelte';

const queryMock = vi.hoisted(() => vi.fn());
const memberDirectoryMocks = vi.hoisted(() => ({
  listRoomMembers: vi.fn()
}));
const attachmentMocks = vi.hoisted(() => ({
  listRoomAttachments: vi.fn(),
  refreshAssetUrls: vi.fn()
}));
const callStore = vi.hoisted(() => ({
  voiceCall: {
    roomId: null as string | null,
    connecting: false,
    connected: false,
    isInAnyCall: false,
    isMuted: false,
    isCameraEnabled: false,
    isScreenShareEnabled: false,
    participants: [] as Array<{
      identity: string;
      name: string;
      login: string;
      avatarUrl: string | null;
      isMuted: boolean;
      isLocal: boolean;
      isLocallyMuted?: boolean;
      connectionQuality: 'excellent' | 'good' | 'poor' | 'lost' | 'unknown';
      isCameraEnabled: boolean;
      videoTrack: unknown;
      isScreenShareEnabled: boolean;
      screenShareTrack: unknown;
    }>,
    audioDevices: [],
    audioOutputDevices: [],
    videoDevices: [],
    selectedDeviceId: null,
    selectedOutputDeviceId: null,
    selectedVideoDeviceId: null,
    isInCall: vi.fn(
      (roomId: string) => callStore.voiceCall.connected && callStore.voiceCall.roomId === roomId
    ),
    join: vi.fn().mockResolvedValue(undefined),
    leave: vi.fn().mockResolvedValue(undefined),
    toggleMute: vi.fn().mockResolvedValue(undefined),
    toggleCamera: vi.fn().mockResolvedValue(undefined),
    toggleScreenShare: vi.fn().mockResolvedValue(undefined),
    toggleParticipantLocalMute: vi.fn(),
    refreshDevices: vi.fn().mockResolvedValue(undefined),
    getAudioLevel: vi.fn((_identity?: string) => ({ isSpeaking: false, audioLevel: 0 })),
    handleParticipantLeftEvent: vi.fn(),
    handleCallEndedEvent: vi.fn()
  },
  activeCallRooms: {
    active: false,
    participants: [] as Array<{
      userId: string;
      displayName: string;
      login: string;
      avatarUrl: string | null;
    }>,
    has: vi.fn(() => callStore.activeCallRooms.active),
    getParticipants: vi.fn(() => callStore.activeCallRooms.participants),
    getParticipantCallPresenceInAnyRoom: vi.fn(
      (_userId: string): 'voice' | 'video' | null => null
    )
  },
  rooms: {
    currentUserId: 'viewer'
  },
  handleVoiceCallJoinFailed: vi.fn()
}));

class MockIntersectionObserver {
  static instances: MockIntersectionObserver[] = [];

  readonly callback: IntersectionObserverCallback;
  readonly elements = new Set<Element>();

  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
    MockIntersectionObserver.instances.push(this);
  }

  observe(element: Element) {
    this.elements.add(element);
  }

  unobserve(element: Element) {
    this.elements.delete(element);
  }

  disconnect() {
    this.elements.clear();
  }

  trigger(isIntersecting = true) {
    const entries = Array.from(this.elements).map((target) => ({
      isIntersecting,
      target
    }));
    this.callback(entries as IntersectionObserverEntry[], this as unknown as IntersectionObserver);
  }
}

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    serverId: 'test-server',
    connectBaseUrl: 'https://chat.example.test/api/connect',
    bearerToken: 'test-token',
    isConnected: true,
    showConnectionLostBanner: false,
    client: {
      query: (...args: unknown[]) => {
        const result = queryMock(...args);
        return Object.assign(result, {
          toPromise: () => result
        });
      },
      mutation: vi.fn(),
      subscription: vi.fn()
    }
  })
}));

vi.mock('$lib/api-client/attachments', () => ({
  createAttachmentAPI: vi.fn(() => ({
    listRoomAttachments: attachmentMocks.listRoomAttachments,
    refreshAssetUrls: attachmentMocks.refreshAssetUrls
  }))
}));

vi.mock('$lib/api-client/memberDirectory', async (importActual) => ({
  ...(await importActual<typeof import('$lib/api-client/memberDirectory')>()),
  createMemberDirectoryAPI: vi.fn(() => ({
    listRoomMembers: memberDirectoryMocks.listRoomMembers
  }))
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'test-server'
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => callStore,
    getServer: () => ({ id: 'test-server', url: 'https://chat.example.test' })
  }
}));

vi.mock('$lib/state/server/permissions.svelte', () => ({
  getServerPermissions: () => ({
    current: {
      canStartDMs: false
    }
  })
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveLogin: (_userId: string, fallback: string) => fallback
}));

function member(index: number): RoomMember {
  return {
    id: `user-${index}`,
    login: `user${index}`,
    displayName: `User ${index}`,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Online
  };
}

function buttonByText(container: Element, text: string): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll('button')).find((button) =>
    button.textContent?.includes(text)
  );
}

function renderedMemberTitles(container: Element): string[] {
  return Array.from(container.querySelectorAll('[title^="View profile of "]')).map(
    (element) => element.getAttribute('title') ?? ''
  );
}

function presenceBadge(container: Element, label: string): Element | null {
  return container.querySelector(`[aria-label="${label}"]`);
}

function roomFileGroupHeadings(container: Element): string[] {
  return Array.from(container.querySelectorAll('[data-testid="room-file-group-heading"]')).map(
    (element) => element.textContent?.trim() ?? ''
  );
}

function roomFileRowLabels(container: Element): string[] {
  return Array.from(container.querySelectorAll('[data-testid="room-file-row"]')).map(
    (element) => element.textContent?.trim() ?? ''
  );
}

async function flushRoomFilesPanel(): Promise<void> {
  await tick();
  await Promise.resolve();
  await tick();
  await Promise.resolve();
  await tick();
}

async function waitForMemberSearchDebounce(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 300));
  await tick();
}

function roomData(members: RoomMember[], totalCount: number, hasMore: boolean): RoomData {
  void members;
  void totalCount;
  void hasMore;
  return {
    room: { id: 'room-1', name: 'general', type: RoomKind.CHANNEL, isUniversal: false },
    spaceName: 'Test Server',
    canPostMessage: true,
    canPostInThread: true,
    canAttach: true,
    canReact: true,
    canManageOthersMessage: false,
    canEchoMessage: false,
    canManageRoom: false,
    canBanRoomMembers: false
  };
}

function mockRoomMembers(members: RoomMember[], totalCount = members.length, hasMore = false) {
  memberDirectoryMocks.listRoomMembers.mockResolvedValue(memberPage(members, totalCount, hasMore));
}

function memberPage(members: RoomMember[], totalCount = members.length, hasMore = false) {
  return {
    members: members.map((member) => ({
      ...member,
      deleted: member.deleted ?? false,
      avatarUrl: member.avatarUrl ?? null,
      customStatus: member.customStatus ?? null,
      roles: [],
      createdAt: null
    })),
    totalCount,
    hasMore
  };
}

function roomFile(
  messageEventId: string,
  threadRootEventId: string | null,
  filename: string,
  createdAt = '2026-06-15T12:00:00Z'
) {
  return {
    messageEventId,
    threadRootEventId,
    createdAt,
    attachment: {
      id: `att-${filename}`,
      filename,
      contentType: 'text/plain',
      width: 0,
      height: 0,
      assetUrl: {
        url: `/assets/files/att-${filename}?access=ticket`,
        expiresAt: '2099-01-01T00:00:00Z'
      },
      thumbnailAssetUrl: null,
      videoProcessing: null
    }
  };
}

function roomVideoFile(filename: string) {
  const base = roomFile('video-message', null, filename);
  return {
    ...base,
    attachment: {
      ...base.attachment,
      contentType: 'video/mp4',
      thumbnailAssetUrl: {
        url: `/assets/files/att-${filename}/image/120x120/cover?access=broken`,
        expiresAt: '2099-01-01T00:00:00Z'
      },
      videoProcessing: {
        status: 'COMPLETED',
        thumbnailAssetUrl: {
          url: 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==',
          expiresAt: '2099-01-01T00:00:00Z'
        }
      }
    }
  };
}

function roomAudioFile(filename: string) {
  const base = roomFile('audio-message', null, filename);
  return {
    ...base,
    attachment: {
      ...base.attachment,
      contentType: 'audio/mpeg',
      thumbnailAssetUrl: {
        url: `/assets/files/att-${filename}/image/120x120/cover?access=broken`,
        expiresAt: '2099-01-01T00:00:00Z'
      }
    }
  };
}

describe('RoomSidebar', () => {
  beforeEach(async () => {
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
    queryMock.mockReset();
    memberDirectoryMocks.listRoomMembers.mockReset();
    attachmentMocks.listRoomAttachments.mockReset();
    attachmentMocks.refreshAssetUrls.mockReset();
    memberDirectoryMocks.listRoomMembers.mockResolvedValue(memberPage([member(1)]));
    attachmentMocks.listRoomAttachments.mockResolvedValue({
      items: [],
      totalCount: 0,
      hasMore: false
    });
    attachmentMocks.refreshAssetUrls.mockResolvedValue(new Map());
    queryMock.mockResolvedValue({
      data: {
        room: {
          members: {
            users: [member(1)],
            totalCount: 1,
            hasMore: false
          }
        }
      },
      error: null
    });
    localStorage.clear();
    MockIntersectionObserver.instances = [];
    vi.stubGlobal('IntersectionObserver', MockIntersectionObserver);
    callStore.voiceCall.roomId = null;
    callStore.voiceCall.connecting = false;
    callStore.voiceCall.connected = false;
    callStore.voiceCall.isInAnyCall = false;
    callStore.voiceCall.isMuted = false;
    callStore.voiceCall.isCameraEnabled = false;
    callStore.voiceCall.isScreenShareEnabled = false;
    callStore.voiceCall.participants = [];
    callStore.voiceCall.isInCall.mockClear();
    callStore.voiceCall.join.mockClear();
    callStore.voiceCall.leave.mockClear();
    callStore.voiceCall.toggleMute.mockClear();
    callStore.voiceCall.toggleCamera.mockClear();
    callStore.voiceCall.toggleScreenShare.mockClear();
    callStore.voiceCall.toggleParticipantLocalMute.mockClear();
    callStore.voiceCall.refreshDevices.mockClear();
    callStore.voiceCall.getAudioLevel.mockClear();
    callStore.voiceCall.getAudioLevel.mockImplementation(() => ({ isSpeaking: false, audioLevel: 0 }));
    callStore.activeCallRooms.active = false;
    callStore.activeCallRooms.participants = [];
    callStore.activeCallRooms.has.mockClear();
    callStore.activeCallRooms.getParticipants.mockClear();
    callStore.activeCallRooms.getParticipantCallPresenceInAnyRoom.mockClear();
    callStore.activeCallRooms.getParticipantCallPresenceInAnyRoom.mockReturnValue(null);
    callStore.handleVoiceCallJoinFailed.mockClear();
  });

  it('shows the exact total count and eagerly loads all member pages', async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) => member(index + 1));
    const secondPage = Array.from({ length: 42 }, (_, index) => member(index + 101));

    memberDirectoryMocks.listRoomMembers
      .mockResolvedValueOnce(memberPage(firstPage, 142, true))
      .mockResolvedValueOnce(memberPage(secondPage, 142, false));

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false)
      }
    });

    await expect.element(q(container, 'h1')).toHaveTextContent('Members (142)');
    await vi.waitFor(() => {
      expect(memberDirectoryMocks.listRoomMembers).toHaveBeenCalledWith(
        'room-1',
        '',
        ROOM_MEMBERS_PAGE_SIZE,
        0
      );
      expect(memberDirectoryMocks.listRoomMembers).toHaveBeenCalledWith(
        'room-1',
        '',
        ROOM_MEMBERS_PAGE_SIZE,
        100
      );
    });

    await vi.waitFor(() => {
      expect(renderedMemberTitles(container)).toHaveLength(142);
    });
    for (let index = 1; index <= 142; index++) {
      expect(renderedMemberTitles(container)).toContain(`View profile of User ${index}`);
    }
    expect(container.querySelector('[data-testid="room-members-load-more-sentinel"]')).toBeFalsy();
  });

  it('renders deleted members with an italicized placeholder', async () => {
    mockRoomMembers([{ ...member(1), deleted: true }]);

    const { container } = render(RoomSidebarTestHarness, {
      props: { roomData: roomData([], 0, false) }
    });

    await vi.waitFor(() => {
      expect(container.textContent).toContain('[deleted user]');
    });
    expect(container.querySelector('em')?.textContent).toBe('[deleted user]');
  });

  it('shows call presence for members active in any room call on the server', async () => {
    mockRoomMembers([member(1), member(2)]);
    callStore.activeCallRooms.getParticipantCallPresenceInAnyRoom.mockImplementation(
      (userId: string) => (userId === 'user-2' ? 'voice' : null)
    );

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false)
      }
    });

    await vi.waitFor(() => {
      expect(q(container, '[data-testid="member-call-presence-voice"]')).toBeTruthy();
    });
    expect(callStore.activeCallRooms.getParticipantCallPresenceInAnyRoom).toHaveBeenCalledWith('user-2');
  });

  it('renders the call tab empty state and starts a call', async () => {
    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false),
        activePanel: 'call',
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    await expect.element(q(container, 'h1')).toHaveTextContent('Call');
    await expect
      .element(q(container, '[data-testid="call-join-button"]'))
      .toHaveTextContent('Start call');
    expect(container.textContent).not.toContain('No active call');
    expect(container.textContent).not.toContain("Start one when you're ready.");

    (q(container, '[data-testid="call-join-button"]') as HTMLButtonElement).click();
    await tick();

    expect(callStore.voiceCall.join).toHaveBeenCalledWith('wss://livekit.example.test', 'room-1');
  });

  it('renders projected call participants before joining', async () => {
    callStore.activeCallRooms.active = true;
    callStore.activeCallRooms.participants = [
      {
        userId: 'user-2',
        login: 'bob',
        displayName: 'Bob',
        avatarUrl: null
      }
    ];

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false),
        activePanel: 'call',
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    await expect.element(q(container, '[data-testid="call-observer-panel"]')).toBeInTheDocument();
    expect(container.textContent).not.toContain('1 in call');
    expect(container.textContent).not.toContain('Voice (1)');
    expect(container.textContent).not.toContain('Video (1)');
    expect(container.textContent).toContain('Bob');
    await expect.element(q(container, '[data-testid="call-participant-card"]')).toBeInTheDocument();
    await expect
      .element(q(container, '[data-testid="call-participants-list"]'))
      .toBeInTheDocument();
  });

  it('renders connected participant cards video-first and exposes call controls', async () => {
    const videoTrack = {
      attach: vi.fn(),
      detach: vi.fn()
    };
    callStore.voiceCall.connected = true;
    callStore.voiceCall.isInAnyCall = true;
    callStore.voiceCall.roomId = 'room-1';
    callStore.voiceCall.participants = [
      {
        identity: 'viewer',
        login: 'alice',
        name: 'Alice',
        avatarUrl: null,
        isMuted: false,
        isLocal: true,
        connectionQuality: 'excellent',
        isCameraEnabled: true,
        videoTrack,
        isScreenShareEnabled: false,
        screenShareTrack: null
      },
      {
        identity: 'user-2',
        login: 'bob',
        name: 'Bob',
        avatarUrl: null,
        isMuted: true,
        isLocal: false,
        connectionQuality: 'good',
        isCameraEnabled: false,
        videoTrack: null,
        isScreenShareEnabled: false,
        screenShareTrack: null
      }
    ];

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false),
        activePanel: 'call',
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    await expect
      .element(q(container, '[data-testid="call-participant-panel"]'))
      .toBeInTheDocument();
    expect(container.textContent).not.toContain('Video (1)');
    expect(container.textContent).not.toContain('Voice (1)');
    expect(container.textContent).toContain('Bob');
    expect(q(container, '[data-testid="call-device-menu-button"]')).toBeTruthy();
    const participantCards = Array.from(
      container.querySelectorAll<HTMLElement>('[data-testid="call-participant-card"]')
    );
    expect(participantCards).toHaveLength(2);
    expect(participantCards[0].className).toContain('participant-card-video');
    expect(participantCards[1].className).toContain('participant-card-compact');
    const mutedIndicator = q(participantCards[1], '[data-testid="call-muted-indicator"]');
    const voiceLocalMuteButton = q(
      participantCards[1],
      '[data-testid="call-feed-local-mute-button"]'
    ) as HTMLButtonElement;
    expect(mutedIndicator).toBeTruthy();
    expect(participantCards[1].hasAttribute('data-speaking-ring')).toBe(true);
    expect(q(participantCards[1], '[data-testid="call-speaking-indicator"]')).toBeFalsy();
    expect(voiceLocalMuteButton).toBeTruthy();
    expect(voiceLocalMuteButton.getAttribute('aria-label')).toBe('Mute locally');

    const deviceButton = q(
      container,
      '[data-testid="call-device-menu-button"]'
    ) as HTMLButtonElement;
    const muteButton = q(container, '[data-testid="call-mute-toggle"]') as HTMLButtonElement;
    const cameraButton = q(container, '[data-testid="call-camera-toggle"]') as HTMLButtonElement;
    const screenShareButton = q(
      container,
      '[data-testid="call-screen-share-toggle"]'
    ) as HTMLButtonElement;
    const leaveButton = q(container, '[data-testid="call-leave-button"]') as HTMLButtonElement;

    expect(deviceButton.className).toContain('btn-secondary');
    expect(muteButton.className).toContain('btn-success');
    expect(cameraButton.className).toContain('btn-secondary');
    expect(screenShareButton.className).toContain('btn-secondary');
    expect(leaveButton.className).toContain('btn-danger');

    muteButton.click();
    cameraButton.click();
    screenShareButton.click();
    voiceLocalMuteButton.click();
    leaveButton.click();
    await tick();

    expect(callStore.voiceCall.toggleMute).toHaveBeenCalledOnce();
    expect(callStore.voiceCall.toggleCamera).toHaveBeenCalledOnce();
    expect(callStore.voiceCall.toggleScreenShare).toHaveBeenCalledOnce();
    expect(callStore.voiceCall.toggleParticipantLocalMute).toHaveBeenCalledWith('user-2');
    expect(callStore.voiceCall.leave).toHaveBeenCalledOnce();
  });

  it('uses green only for active call media controls', async () => {
    callStore.voiceCall.connected = true;
    callStore.voiceCall.isInAnyCall = true;
    callStore.voiceCall.roomId = 'room-1';
    callStore.voiceCall.isMuted = true;
    callStore.voiceCall.isCameraEnabled = true;
    callStore.voiceCall.isScreenShareEnabled = true;

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false),
        activePanel: 'call',
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    expect(q(container, '[data-testid="call-mute-toggle"]')!.className).toContain(
      'btn-secondary'
    );
    expect(q(container, '[data-testid="call-camera-toggle"]')!.className).toContain(
      'btn-success'
    );
    expect(q(container, '[data-testid="call-screen-share-toggle"]')!.className).toContain(
      'btn-success'
    );
    expect(q(container, '[data-testid="call-leave-button"]')!.className).toContain('btn-danger');
  });

  it('shows an accent speaking ring for active speakers', async () => {
    callStore.voiceCall.connected = true;
    callStore.voiceCall.isInAnyCall = true;
    callStore.voiceCall.roomId = 'room-1';
    callStore.voiceCall.getAudioLevel.mockImplementation((identity?: string) => ({
      isSpeaking: identity === 'viewer',
      audioLevel: identity === 'viewer' ? 0.5 : 0
    }));
    callStore.voiceCall.participants = [
      {
        identity: 'viewer',
        login: 'alice',
        name: 'Alice',
        avatarUrl: null,
        isMuted: false,
        isLocal: true,
        connectionQuality: 'excellent',
        isCameraEnabled: false,
        videoTrack: null,
        isScreenShareEnabled: false,
        screenShareTrack: null
      }
    ];

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false),
        activePanel: 'call',
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    const card = q(container, '[data-testid="call-participant-card"]') as HTMLElement;

    await vi.waitFor(() => {
      expect(callStore.voiceCall.getAudioLevel).toHaveBeenCalledWith('viewer');
      expect(card.dataset.callSpeaking).toBe('true');
      expect(Number(card.style.getPropertyValue('--call-speaking-ring-opacity'))).toBeGreaterThan(0);
    });
    expect(card.className).toContain('call-speaking-card');
    expect(q(card, '[data-testid="call-speaking-indicator"]')).toBeFalsy();
  });

  it('renders one participant list without empty section labels', async () => {
    const videoTrack = {
      attach: vi.fn(),
      detach: vi.fn()
    };
    callStore.voiceCall.connected = true;
    callStore.voiceCall.isInAnyCall = true;
    callStore.voiceCall.roomId = 'room-1';
    callStore.voiceCall.participants = [
      {
        identity: 'viewer',
        login: 'alice',
        name: 'Alice',
        avatarUrl: null,
        isMuted: false,
        isLocal: true,
        connectionQuality: 'excellent',
        isCameraEnabled: true,
        videoTrack,
        isScreenShareEnabled: false,
        screenShareTrack: null
      }
    ];

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false),
        activePanel: 'call',
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    expect(container.textContent).not.toContain('Video (1)');
    expect(container.textContent).not.toContain('Voice (0)');
    expect(container.textContent).not.toContain('No voice-only participants.');
    const participantList = q(container, '[data-testid="call-participants-list"]');
    expect(participantList).toBeTruthy();
    expect(participantList!.className).not.toContain('@min-[368px]:grid-cols-2');
  });

  it('pins screen-share tiles before camera and voice participant cards', async () => {
    const screenShareTrack = {
      attach: vi.fn(),
      detach: vi.fn()
    };
    const cameraTrack = {
      attach: vi.fn(),
      detach: vi.fn()
    };
    callStore.voiceCall.connected = true;
    callStore.voiceCall.isInAnyCall = true;
    callStore.voiceCall.roomId = 'room-1';
    callStore.voiceCall.participants = [
      {
        identity: 'viewer',
        login: 'alice',
        name: 'Alice',
        avatarUrl: null,
        isMuted: false,
        isLocal: true,
        connectionQuality: 'excellent',
        isCameraEnabled: false,
        videoTrack: null,
        isScreenShareEnabled: true,
        screenShareTrack
      },
      {
        identity: 'user-2',
        login: 'bob',
        name: 'Bob',
        avatarUrl: null,
        isMuted: false,
        isLocal: false,
        connectionQuality: 'good',
        isCameraEnabled: true,
        videoTrack: cameraTrack,
        isScreenShareEnabled: false,
        screenShareTrack: null
      },
      {
        identity: 'user-3',
        login: 'carol',
        name: 'Carol',
        avatarUrl: null,
        isMuted: false,
        isLocal: false,
        connectionQuality: 'good',
        isCameraEnabled: false,
        videoTrack: null,
        isScreenShareEnabled: false,
        screenShareTrack: null
      }
    ];

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false),
        activePanel: 'call',
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    const participantList = q(container, '[data-testid="call-participants-list"]');
    expect(participantList).toBeTruthy();
    const cards = Array.from(participantList!.children);
    expect(cards[0].getAttribute('data-testid')).toBe('call-screen-share-card');
    expect(cards[0].textContent).toContain("Alice's screen");
    expect(cards[0].querySelector('video')?.className).toContain('object-contain');
    expect(cards[1].getAttribute('data-testid')).toBe('call-participant-card');
    expect(cards[1].textContent).toContain('Bob');
    expect(cards[1].querySelector('video')?.className).toContain('object-cover');
    expect(cards[2].getAttribute('data-testid')).toBe('call-participant-card');
    expect(cards[2].textContent).toContain('Alice');
    expect(cards[3].getAttribute('data-testid')).toBe('call-participant-card');
    expect(cards[3].textContent).toContain('Carol');
    expect(participantList!.className).toContain('@min-[368px]:grid-cols-2');
  });

  it('uses a screen share as the featured maximized call stage', async () => {
    const screenShareTrack = {
      attach: vi.fn(),
      detach: vi.fn()
    };
    const cameraTrack = {
      attach: vi.fn(),
      detach: vi.fn()
    };
    callStore.voiceCall.connected = true;
    callStore.voiceCall.isInAnyCall = true;
    callStore.voiceCall.roomId = 'room-1';
    callStore.voiceCall.participants = [
      {
        identity: 'viewer',
        login: 'alice',
        name: 'Alice',
        avatarUrl: null,
        isMuted: false,
        isLocal: true,
        connectionQuality: 'excellent',
        isCameraEnabled: false,
        videoTrack: null,
        isScreenShareEnabled: true,
        screenShareTrack
      },
      {
        identity: 'user-2',
        login: 'bob',
        name: 'Bob',
        avatarUrl: null,
        isMuted: false,
        isLocal: false,
        connectionQuality: 'good',
        isCameraEnabled: true,
        videoTrack: cameraTrack,
        isScreenShareEnabled: false,
        screenShareTrack: null
      },
      {
        identity: 'user-3',
        login: 'carol',
        name: 'Carol',
        avatarUrl: null,
        isMuted: false,
        isLocal: false,
        connectionQuality: 'good',
        isCameraEnabled: false,
        videoTrack: null,
        isScreenShareEnabled: false,
        screenShareTrack: null
      }
    ];

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        activePanel: 'call',
        maximized: true,
        roomData: roomData([], 0, false),
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    const featured = q(container, '[data-testid="call-featured-stage-card"]');
    expect(featured).toBeTruthy();
    expect(featured!.textContent).toContain("Alice's screen");
    expect(featured!.querySelector('video')?.className).toContain('object-contain');
    const localMuteButton = q(
      featured!,
      '[data-testid="call-feed-local-mute-button"]'
    ) as HTMLButtonElement;
    expect(localMuteButton).toBeTruthy();
    expect(localMuteButton.getAttribute('aria-label')).toBe('Mute');
    localMuteButton.click();
    expect(callStore.voiceCall.toggleMute).toHaveBeenCalledOnce();
    const controlsBar = q(container, '[data-testid="call-controls-bar"]');
    expect(controlsBar).toBeTruthy();
    expect(q(container, '[data-testid="call-device-menu-button"]')).toBeTruthy();
    expect(featured!.compareDocumentPosition(controlsBar!) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING
    );

    const secondaryList = q(container, '[data-testid="call-secondary-stage-list"]');
    expect(secondaryList).toBeTruthy();
    const secondaryCards = Array.from(secondaryList!.children);
    expect(secondaryCards).toHaveLength(3);
    expect(secondaryCards[0].textContent).toContain('Bob');
    expect(secondaryCards[0].querySelector('video')?.className).toContain('object-cover');
    expect(secondaryCards[1].textContent).toContain('Alice');
    expect(secondaryCards[2].textContent).toContain('Carol');
  });

  it('shows fullscreen and local mute controls on call media tiles', async () => {
    const fullscreenTargets: Element[] = [];
    const requestFullscreen = vi
      .spyOn(HTMLElement.prototype, 'requestFullscreen')
      .mockImplementation(function (this: HTMLElement) {
        fullscreenTargets.push(this);
        return Promise.resolve();
      });
    const cameraTrack = {
      attach: vi.fn(),
      detach: vi.fn()
    };
    callStore.voiceCall.connected = true;
    callStore.voiceCall.isInAnyCall = true;
    callStore.voiceCall.roomId = 'room-1';
    callStore.voiceCall.participants = [
      {
        identity: 'viewer',
        login: 'alice',
        name: 'Alice',
        avatarUrl: null,
        isMuted: false,
        isLocal: true,
        connectionQuality: 'excellent',
        isCameraEnabled: false,
        videoTrack: null,
        isScreenShareEnabled: false,
        screenShareTrack: null
      },
      {
        identity: 'user-2',
        login: 'bob',
        name: 'Bob',
        avatarUrl: null,
        isMuted: false,
        isLocal: false,
        isLocallyMuted: true,
        connectionQuality: 'good',
        isCameraEnabled: true,
        videoTrack: cameraTrack,
        isScreenShareEnabled: false,
        screenShareTrack: null
      }
    ];

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        activePanel: 'call',
        maximized: true,
        roomData: roomData([], 0, false),
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    const featured = q(container, '[data-testid="call-featured-stage-card"]')!;
    const mediaActions = q(featured, '[data-testid="call-media-actions"]')!;
    const fullscreenButton = q(featured, '[data-testid="call-feed-fullscreen-button"]') as HTMLButtonElement;
    const localMuteButton = q(featured, '[data-testid="call-feed-local-mute-button"]') as HTMLButtonElement;

    expect(mediaActions.className).toContain('border-text/10');
    expect(mediaActions.className).toContain('bg-surface');
    expect(mediaActions.className).toContain('flex');
    expect(mediaActions.className).not.toContain('absolute');
    expect(fullscreenButton).toBeTruthy();
    expect(fullscreenButton.className).toContain('text-muted');
    expect(fullscreenButton.className).not.toContain('bg-black');
    expect(fullscreenButton.querySelector('.mdi--fullscreen')).toBeTruthy();
    expect(localMuteButton).toBeTruthy();
    expect(localMuteButton.getAttribute('aria-label')).toBe('Unmute locally');
    expect(q(featured, '[data-testid="call-locally-muted-indicator"]')).toBeTruthy();

    fullscreenButton.click();
    await Promise.resolve();

    expect(requestFullscreen).toHaveBeenCalledOnce();
    expect(fullscreenTargets[0]).toBe(featured);

    localMuteButton.click();

    expect(callStore.voiceCall.toggleParticipantLocalMute).toHaveBeenCalledWith('user-2');

    requestFullscreen.mockRestore();
  });

  it('falls back to a camera participant for the maximized call stage', async () => {
    const cameraTrack = {
      attach: vi.fn(),
      detach: vi.fn()
    };
    callStore.voiceCall.connected = true;
    callStore.voiceCall.isInAnyCall = true;
    callStore.voiceCall.roomId = 'room-1';
    callStore.voiceCall.participants = [
      {
        identity: 'viewer',
        login: 'alice',
        name: 'Alice',
        avatarUrl: null,
        isMuted: false,
        isLocal: true,
        connectionQuality: 'excellent',
        isCameraEnabled: false,
        videoTrack: null,
        isScreenShareEnabled: false,
        screenShareTrack: null
      },
      {
        identity: 'user-2',
        login: 'bob',
        name: 'Bob',
        avatarUrl: null,
        isMuted: false,
        isLocal: false,
        connectionQuality: 'good',
        isCameraEnabled: true,
        videoTrack: cameraTrack,
        isScreenShareEnabled: false,
        screenShareTrack: null
      }
    ];

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        activePanel: 'call',
        maximized: true,
        roomData: roomData([], 0, false),
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    const featured = q(container, '[data-testid="call-featured-stage-card"]');
    expect(featured).toBeTruthy();
    expect(featured!.textContent).toContain('Bob');
    expect(featured!.querySelector('video')?.className).toContain('object-cover');
    const secondaryList = q(container, '[data-testid="call-secondary-stage-list"]');
    expect(secondaryList).toBeTruthy();
    expect(secondaryList!.children[0].textContent).toContain('Alice');
  });

  it('falls back to a voice participant for the maximized call stage with speaking and quality indicators', async () => {
    callStore.voiceCall.connected = true;
    callStore.voiceCall.isInAnyCall = true;
    callStore.voiceCall.roomId = 'room-1';
    callStore.voiceCall.getAudioLevel.mockImplementation((identity?: string) => ({
      isSpeaking: identity === 'viewer',
      audioLevel: identity === 'viewer' ? 0.6 : 0
    }));
    callStore.voiceCall.participants = [
      {
        identity: 'viewer',
        login: 'alice',
        name: 'Alice',
        avatarUrl: null,
        isMuted: false,
        isLocal: true,
        connectionQuality: 'poor',
        isCameraEnabled: false,
        videoTrack: null,
        isScreenShareEnabled: false,
        screenShareTrack: null
      }
    ];

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        activePanel: 'call',
        maximized: true,
        roomData: roomData([], 0, false),
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    const featured = q(container, '[data-testid="call-featured-stage-card"]');
    expect(featured).toBeTruthy();
    expect(featured!.textContent).toContain('Alice');
    expect(featured!.querySelector('video')).toBeFalsy();
    await vi.waitFor(() => {
      expect(callStore.voiceCall.getAudioLevel).toHaveBeenCalledWith('viewer');
      expect(featured!.dataset.callSpeaking).toBe('true');
      expect(Number(featured!.style.getPropertyValue('--call-speaking-ring-opacity'))).toBeGreaterThan(
        0
      );
    });
    expect(featured!.hasAttribute('data-speaking-ring')).toBe(true);
    expect(q(featured!, '[aria-label="Poor connection"]')).toBeTruthy();
    const localMuteButton = q(
      featured!,
      '[data-testid="call-feed-local-mute-button"]'
    ) as HTMLButtonElement;
    expect(localMuteButton).toBeTruthy();
    expect(localMuteButton.getAttribute('aria-label')).toBe('Mute');
    localMuteButton.click();
    expect(callStore.voiceCall.toggleMute).toHaveBeenCalledOnce();
    expect(container.querySelector('[data-testid="call-secondary-stage-list"]')).toBeFalsy();
  });

  it('uses a two-column video grid when multiple videos have room', async () => {
    const videoTrackA = {
      attach: vi.fn(),
      detach: vi.fn()
    };
    const videoTrackB = {
      attach: vi.fn(),
      detach: vi.fn()
    };
    callStore.voiceCall.connected = true;
    callStore.voiceCall.isInAnyCall = true;
    callStore.voiceCall.roomId = 'room-1';
    callStore.voiceCall.participants = [
      {
        identity: 'viewer',
        login: 'alice',
        name: 'Alice',
        avatarUrl: null,
        isMuted: false,
        isLocal: true,
        connectionQuality: 'excellent',
        isCameraEnabled: true,
        videoTrack: videoTrackA,
        isScreenShareEnabled: false,
        screenShareTrack: null
      },
      {
        identity: 'user-2',
        login: 'bob',
        name: 'Bob',
        avatarUrl: null,
        isMuted: false,
        isLocal: false,
        connectionQuality: 'good',
        isCameraEnabled: true,
        videoTrack: videoTrackB,
        isScreenShareEnabled: false,
        screenShareTrack: null
      }
    ];

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false),
        activePanel: 'call',
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    expect(container.textContent).not.toContain('Video (2)');
    const participantList = q(container, '[data-testid="call-participants-list"]');
    expect(participantList).toBeTruthy();
    expect(participantList!.className).toContain('@min-[368px]:grid-cols-2');
  });

  it('disables joining this room while connected to another call', async () => {
    callStore.activeCallRooms.active = true;
    callStore.voiceCall.connected = true;
    callStore.voiceCall.isInAnyCall = true;
    callStore.voiceCall.roomId = 'other-room';

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false),
        activePanel: 'call',
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    const joinButton = q(container, '[data-testid="call-join-button"]') as HTMLButtonElement;
    expect(joinButton.disabled).toBe(true);
    expect(joinButton.title).toBe('Already in another call');
  });

  it('filters room members locally without changing the canonical total count', async () => {
    memberDirectoryMocks.listRoomMembers.mockResolvedValueOnce(
      memberPage([member(1), { ...member(2), displayName: 'Boris Member' }])
    );

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false)
      }
    });

    await vi.waitFor(() => {
      expect(renderedMemberTitles(container)).toHaveLength(2);
    });

    const input = container.querySelector('#room-member-search') as HTMLInputElement;
    input.value = 'bor';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await waitForMemberSearchDebounce();

    await vi.waitFor(() => {
      expect(renderedMemberTitles(container)).toEqual(['View profile of Boris Member']);
      expect(q(container, 'h1')?.textContent).toContain('Members (2)');
    });
    expect(memberDirectoryMocks.listRoomMembers).toHaveBeenCalledTimes(1);
  });

  it('clears the member search with the Chatto-styled clear button without refetching', async () => {
    memberDirectoryMocks.listRoomMembers.mockResolvedValueOnce(
      memberPage([member(1), { ...member(2), displayName: 'Boris Member' }])
    );

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false)
      }
    });

    await vi.waitFor(() => {
      expect(renderedMemberTitles(container)).toHaveLength(2);
    });

    const input = container.querySelector('#room-member-search') as HTMLInputElement;
    input.value = 'bor';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await waitForMemberSearchDebounce();

    await vi.waitFor(() => {
      expect(renderedMemberTitles(container)).toEqual(['View profile of Boris Member']);
    });

    const clearButton = q(
      container,
      'button[aria-label="Clear member search"]'
    ) as HTMLButtonElement;
    expect(clearButton.className).toContain('pane-header-icon-button');
    clearButton.click();
    await tick();

    await vi.waitFor(() => {
      expect(input.value).toBe('');
      expect(renderedMemberTitles(container)).toHaveLength(2);
      expect(q(container, 'h1')?.textContent).toContain('Members (2)');
      expect(document.activeElement).toBe(input);
    });

    await new Promise((resolve) => setTimeout(resolve, 350));
    expect(memberDirectoryMocks.listRoomMembers).toHaveBeenCalledTimes(1);
  });

  it('shows an empty local search result without changing the canonical total count', async () => {
    memberDirectoryMocks.listRoomMembers.mockResolvedValueOnce(memberPage([member(1), member(2)]));

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false)
      }
    });

    await vi.waitFor(() => {
      expect(renderedMemberTitles(container)).toHaveLength(2);
    });

    const input = container.querySelector('#room-member-search') as HTMLInputElement;
    input.value = 'no-match';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await waitForMemberSearchDebounce();

    await vi.waitFor(() => {
      expect(container.textContent).toContain('No members found.');
      expect(q(container, 'h1')?.textContent).toContain('Members (2)');
      expect(renderedMemberTitles(container)).toEqual([]);
    });

    await new Promise((resolve) => setTimeout(resolve, 350));
    expect(memberDirectoryMocks.listRoomMembers).toHaveBeenCalledTimes(1);
  });

  it('filters the loaded member directory without refetching', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    memberDirectoryMocks.listRoomMembers.mockResolvedValueOnce(
      memberPage([member(1), { ...member(2), displayName: 'Boris Member' }])
    );

    try {
      const { container } = render(RoomSidebarTestHarness, {
        props: {
          roomData: roomData([], 0, false)
        }
      });

      await vi.waitFor(() => {
        expect(renderedMemberTitles(container)).toHaveLength(2);
      });

      const input = container.querySelector('#room-member-search') as HTMLInputElement;
      input.value = 'bor';
      input.dispatchEvent(new Event('input', { bubbles: true }));
      await waitForMemberSearchDebounce();

      await vi.waitFor(() => {
        expect(renderedMemberTitles(container)).toEqual(['View profile of Boris Member']);
      });
      expect(memberDirectoryMocks.listRoomMembers).toHaveBeenCalledTimes(1);
      expect(consoleErrorSpy).not.toHaveBeenCalled();
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });

  it('keeps away members present while showing the global away badge', async () => {
    let presenceCache: PresenceCache | null = null;
    const [user] = [member(1)];

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([user], 1, false),
        onPresenceCacheReady: (cache: PresenceCache) => {
          presenceCache = cache;
        }
      }
    });

    await expect.element(q(container, 'h1')).toHaveTextContent('Members (1)');
    expect(presenceBadge(container, 'Online')).toBeTruthy();
    await vi.waitFor(() => {
      expect(buttonByText(container, 'Online (1)')).toBeTruthy();
    });

    await vi.waitFor(() => {
      expect(presenceCache).toBeTruthy();
    });
    presenceCache!.update({ serverId: 'test-server', userId: user.id }, PresenceStatus.Away);
    await tick();

    expect(presenceBadge(container, 'Away')).toBeTruthy();
    expect(buttonByText(container, 'Online (1)')).toBeTruthy();

    presenceCache!.update({ serverId: 'test-server', userId: user.id }, PresenceStatus.Online);
    await tick();

    expect(presenceBadge(container, 'Online')).toBeTruthy();
    expect(buttonByText(container, 'Online (1)')).toBeTruthy();
  });

  it('calls onClose when the room extras close button is clicked', async () => {
    const onClose = vi.fn();
    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([member(1)], 1, false),
        onClose
      }
    });

    const closeButton = container.querySelector(
      '[aria-label="Hide room extras"]'
    ) as HTMLButtonElement | null;
    expect(closeButton).toBeTruthy();

    closeButton!.click();
    await tick();

    expect(onClose).toHaveBeenCalledOnce();
  });

  it('shows a desktop call maximize action and toggles to minimize copy', async () => {
    const onToggleMaximized = vi.fn();
    const fullscreenTargets: Element[] = [];
    const requestFullscreen = vi
      .spyOn(HTMLElement.prototype, 'requestFullscreen')
      .mockImplementation(function (this: HTMLElement) {
        fullscreenTargets.push(this);
        return Promise.resolve();
      });
    const { container, rerender } = render(RoomSidebarTestHarness, {
      props: {
        activePanel: 'call',
        hasActiveCall: true,
        livekitUrl: 'wss://livekit.example.test',
        roomData: roomData([member(1)], 1, false),
        onToggleMaximized
      }
    });

    const maximizeButton = container.querySelector(
      '[aria-label="Maximise call"]'
    ) as HTMLButtonElement | null;
    expect(maximizeButton).toBeTruthy();
    expect(maximizeButton!.querySelector('.mdi--arrow-expand-left')).toBeTruthy();
    const normalFullscreenButton = container.querySelector(
      '[aria-label="Fullscreen call"]'
    ) as HTMLButtonElement | null;
    expect(normalFullscreenButton).toBeTruthy();
    expect(normalFullscreenButton!.querySelector('.mdi--monitor-share')).toBeTruthy();

    maximizeButton!.click();
    await tick();

    expect(onToggleMaximized).toHaveBeenCalledOnce();

    await rerender({
      activePanel: 'call',
      hasActiveCall: true,
      livekitUrl: 'wss://livekit.example.test',
      roomData: roomData([member(1)], 1, false),
      maximized: true,
      onToggleMaximized
    });

    const minimizeButton = container.querySelector(
      '[aria-label="Minimise call"]'
    ) as HTMLButtonElement | null;
    expect(minimizeButton).toBeTruthy();
    expect(minimizeButton!.querySelector('.mdi--arrow-collapse-right')).toBeTruthy();
    const fullscreenButton = container.querySelector('[aria-label="Fullscreen call"]') as HTMLButtonElement | null;
    expect(fullscreenButton).toBeTruthy();
    expect(fullscreenButton!.querySelector('.mdi--monitor-share')).toBeTruthy();

    fullscreenButton!.click();
    await Promise.resolve();

    expect(requestFullscreen).toHaveBeenCalledOnce();
    expect(fullscreenTargets[0].getAttribute('aria-label')).toBe('Room extras');
    requestFullscreen.mockRestore();
  });

  it('hides call maximize and fullscreen actions until the call is active', async () => {
    const { container } = render(RoomSidebarTestHarness, {
      props: {
        activePanel: 'call',
        livekitUrl: 'wss://livekit.example.test',
        roomData: roomData([member(1)], 1, false),
        onToggleMaximized: vi.fn()
      }
    });

    expect(container.querySelector('[aria-label="Maximise call"]')).toBeFalsy();
    expect(container.querySelector('[aria-label="Fullscreen call"]')).toBeFalsy();
  });

  it('keeps call fullscreen available in overlay but maximizes only on desktop', async () => {
    const onToggleMaximized = vi.fn();
    const { container, rerender } = render(RoomSidebarTestHarness, {
      props: {
        activePanel: 'members',
        roomData: roomData([member(1)], 1, false),
        onToggleMaximized
      }
    });

    expect(container.querySelector('[aria-label="Maximise call"]')).toBeFalsy();
    expect(container.querySelector('[aria-label="Fullscreen call"]')).toBeFalsy();

    await rerender({
      activePanel: 'files',
      roomData: roomData([member(1)], 1, false),
      onToggleMaximized
    });
    expect(container.querySelector('[aria-label="Maximise call"]')).toBeFalsy();
    expect(container.querySelector('[aria-label="Fullscreen call"]')).toBeFalsy();

    await rerender({
      activePanel: 'call',
      hasActiveCall: true,
      presentation: 'overlay',
      livekitUrl: 'wss://livekit.example.test',
      roomData: roomData([member(1)], 1, false),
      onToggleMaximized
    });
    expect(container.querySelector('[aria-label="Maximise call"]')).toBeFalsy();
    expect(container.querySelector('[aria-label="Fullscreen call"]')).toBeTruthy();
  });

  it('renders overlay presentation without desktop resizing chrome', async () => {
    const { container } = render(RoomSidebarTestHarness, {
      props: {
        presentation: 'overlay',
        roomData: roomData([member(1)], 1, false)
      }
    });

    const sidebar = container.querySelector('[aria-label="Room extras"]') as HTMLElement | null;
    expect(sidebar).toBeTruthy();
    expect(sidebar!.style.width).toBe('');
    expect(container.querySelector('[aria-label="Resize room extras pane"]')).toBeFalsy();
  });

  it('renders an empty files panel', async () => {
    attachmentMocks.listRoomAttachments.mockResolvedValue({
      items: [],
      totalCount: 0,
      hasMore: false
    });

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        activePanel: 'files',
        roomData: roomData([member(1)], 1, false)
      }
    });

    await expect.element(q(container, 'h1')).toHaveTextContent('Files');
    await vi.waitFor(() => {
      expect(container.textContent).toContain('No files in this room yet.');
    });
    expect(container.querySelector('[aria-label="Members"]')).toBeFalsy();
  });

  it('keeps the files panel usable when attachment loading fails', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    attachmentMocks.listRoomAttachments.mockRejectedValue(new Error('attachments unavailable'));

    try {
      const { container } = render(RoomSidebarTestHarness, {
        props: {
          activePanel: 'files',
          roomData: roomData([member(1)], 1, false)
        }
      });

      await expect.element(q(container, 'h1')).toHaveTextContent('Files');
      await vi.waitFor(() => {
        expect(container.textContent).toContain('No files in this room yet.');
      });
      expect(container.querySelector('[data-testid="room-files-load-more-sentinel"]')).toBeFalsy();
      expect(consoleErrorSpy).toHaveBeenCalled();
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });

  it('renders room files, opens their message anchors, and automatically loads more', async () => {
    const onOpenFile = vi.fn();
    attachmentMocks.listRoomAttachments
      .mockResolvedValueOnce({
        items: [roomFile('root-message', null, 'root.txt')],
        totalCount: 2,
        hasMore: true
      })
      .mockResolvedValueOnce({
        items: [roomFile('thread-message', 'thread-root', 'thread.txt')],
        totalCount: 2,
        hasMore: false
      });

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        activePanel: 'files',
        roomData: roomData([member(1)], 1, false),
        onOpenFile
      }
    });

    await expect.element(q(container, 'h1')).toHaveTextContent('Files');
    await vi.waitFor(() => {
      expect(container.textContent).toContain('root.txt');
      expect(container.querySelector('[data-testid="room-files-load-more-sentinel"]')).toBeTruthy();
      expect(MockIntersectionObserver.instances).toHaveLength(1);
    });

    buttonByText(container, 'root.txt')!.click();
    await tick();
    expect(onOpenFile).toHaveBeenCalledWith('root-message', null);

    MockIntersectionObserver.instances[0].trigger();
    await tick();

    await vi.waitFor(() => {
      expect(attachmentMocks.listRoomAttachments).toHaveBeenCalledWith({
        roomId: 'room-1',
        limit: 50,
        offset: 1,
        thumbnail: {
          width: 120,
          height: 120,
          fit: 'COVER'
        }
      });
      expect(container.textContent).toContain('thread.txt');
      expect(container.querySelector('[data-testid="room-files-load-more-sentinel"]')).toBeFalsy();
    });

    buttonByText(container, 'thread.txt')!.click();
    await tick();
    expect(onOpenFile).toHaveBeenCalledWith('thread-message', 'thread-root');
  });

  it('groups room files by date and appends loaded pages into the matching groups', async () => {
    const fileGroupingNow = new Date('2026-06-17T12:00:00Z');

    attachmentMocks.listRoomAttachments
      .mockResolvedValueOnce({
        items: [
          roomFile('today-message', null, 'today.txt', '2026-06-17T08:00:00Z'),
          roomFile('yesterday-message', null, 'yesterday.txt', '2026-06-16T08:00:00Z')
        ],
        totalCount: 5,
        hasMore: true
      })
      .mockResolvedValueOnce({
        items: [
          roomFile('week-message', null, 'week.txt', '2026-06-15T08:00:00Z'),
          roomFile('month-message', null, 'month.txt', '2026-06-10T08:00:00Z'),
          roomFile('older-month-message', null, 'older-month.txt', '2026-05-21T08:00:00Z')
        ],
        totalCount: 5,
        hasMore: false
      });

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        activePanel: 'files',
        roomData: roomData([member(1)], 1, false),
        fileGroupingNow
      }
    });

    await flushRoomFilesPanel();
    expect(roomFileGroupHeadings(container)).toEqual(['Today', 'Yesterday']);
    expect(roomFileRowLabels(container)).toHaveLength(2);
    expect(roomFileRowLabels(container)[0]).toContain('today.txt');
    expect(roomFileRowLabels(container)[1]).toContain('yesterday.txt');

    MockIntersectionObserver.instances[0].trigger();
    await flushRoomFilesPanel();

    expect(roomFileGroupHeadings(container)).toEqual([
      'Today',
      'Yesterday',
      'This week',
      'This month',
      'May 2026'
    ]);
    const labels = roomFileRowLabels(container);
    expect(labels).toHaveLength(5);
    expect(labels.filter((label) => label.includes('today.txt'))).toHaveLength(1);
    expect(labels[2]).toContain('week.txt');
    expect(labels[3]).toContain('month.txt');
    expect(labels[4]).toContain('older-month.txt');
  });

  it('localizes room file date groups with the active locale', async () => {
    await loadLocaleMessages('de-DE');
    setReactiveLocale('de-DE');
    const fileGroupingNow = new Date('2026-06-17T12:00:00Z');

    attachmentMocks.listRoomAttachments.mockResolvedValueOnce({
      items: [
        roomFile('today-message', null, 'today.txt', '2026-06-17T08:00:00Z'),
        roomFile('yesterday-message', null, 'yesterday.txt', '2026-06-16T08:00:00Z'),
        roomFile('week-message', null, 'week.txt', '2026-06-15T08:00:00Z'),
        roomFile('month-message', null, 'month.txt', '2026-06-10T08:00:00Z'),
        roomFile('older-month-message', null, 'older-month.txt', '2026-05-21T08:00:00Z')
      ],
      totalCount: 5,
      hasMore: false
    });

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        activePanel: 'files',
        roomData: roomData([member(1)], 1, false),
        fileGroupingNow
      }
    });

    await flushRoomFilesPanel();

    expect(roomFileGroupHeadings(container)).toEqual([
      'Heute',
      'Gestern',
      'Diese Woche',
      'Dieser Monat',
      'Mai 2026'
    ]);
  });

  it('falls back to a file icon when a video thumbnail fails to load', async () => {
    attachmentMocks.listRoomAttachments
      .mockResolvedValueOnce({
        items: [roomVideoFile('clip.mp4')],
        totalCount: 1,
        hasMore: false
      });
    attachmentMocks.refreshAssetUrls.mockResolvedValueOnce(new Map());

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        activePanel: 'files',
        roomData: roomData([member(1)], 1, false)
      }
    });

    await vi.waitFor(() => {
      const image = container.querySelector('img[src^="data:image/gif"]');
      expect(image).toBeTruthy();
      image!.dispatchEvent(new Event('error'));
    });

    await vi.waitFor(() => {
      expect(container.querySelector('img[src^="data:image/gif"]')).toBeFalsy();
      expect(container.querySelector('.mdi--file-video-outline')).toBeTruthy();
    });
  });

  it('renders an icon instead of a broken thumbnail for audio files', async () => {
    attachmentMocks.listRoomAttachments.mockResolvedValueOnce({
      items: [roomAudioFile('song.mp3')],
      totalCount: 1,
      hasMore: false
    });

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        activePanel: 'files',
        roomData: roomData([member(1)], 1, false)
      }
    });

    await vi.waitFor(() => {
      expect(container.textContent).toContain('song.mp3');
      expect(container.querySelector('img')).toBeFalsy();
      expect(container.querySelector('.mdi--file-music-outline')).toBeTruthy();
    });
  });

  it('shows the room-ban action for other members when allowed', async () => {
    mockRoomMembers([
      { ...member(0), id: 'viewer', displayName: 'Viewer' },
      { ...member(1), id: 'other', displayName: 'Other Member' }
    ]);

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        currentUserId: 'viewer',
        canBanRoomMembers: true,
        roomData: roomData([], 0, false)
      }
    });

    await vi.waitFor(() => {
      expect(buttonByText(container, 'Other Member')).toBeTruthy();
    });
    buttonByText(container, 'Other Member')!.click();
    await tick();

    expect(container.textContent).toContain('Ban from room');
  });

  it('hides the room-ban action when member moderation is disabled', async () => {
    mockRoomMembers([
      { ...member(0), id: 'viewer', displayName: 'Viewer' },
      { ...member(1), id: 'other', displayName: 'Other Member' }
    ]);

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        currentUserId: 'viewer',
        canBanRoomMembers: false,
        roomData: roomData([], 0, false)
      }
    });

    await vi.waitFor(() => {
      expect(buttonByText(container, 'Other Member')).toBeTruthy();
    });
    buttonByText(container, 'Other Member')!.click();
    await tick();

    expect(container.textContent).not.toContain('Ban from room');
  });
});
