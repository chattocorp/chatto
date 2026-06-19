import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import { q } from '$lib/test-utils';
import type { RoomMember } from '$lib/state/room';
import type { PresenceCache } from '$lib/state/presenceCache.svelte';
import type { RoomData } from '$lib/hooks/useRoomData.svelte';
import { PresenceStatus } from '$lib/gql/graphql';
import RoomSidebarTestHarness from './RoomSidebarTestHarness.svelte';

const queryMock = vi.hoisted(() => vi.fn());
const callStore = vi.hoisted(() => ({
  voiceCall: {
    roomId: null as string | null,
    connecting: false,
    connected: false,
    isInAnyCall: false,
    isMuted: false,
    isCameraEnabled: false,
    participants: [] as Array<{
      identity: string;
      name: string;
      login: string;
      avatarUrl: string | null;
      isMuted: boolean;
      isLocal: boolean;
      connectionQuality: 'excellent' | 'good' | 'poor' | 'lost' | 'unknown';
      isCameraEnabled: boolean;
      videoTrack: unknown;
    }>,
    audioDevices: [],
    audioOutputDevices: [],
    videoDevices: [],
    selectedDeviceId: null,
    selectedOutputDeviceId: null,
    selectedVideoDeviceId: null,
    isInCall: vi.fn((roomId: string) => callStore.voiceCall.connected && callStore.voiceCall.roomId === roomId),
    join: vi.fn().mockResolvedValue(undefined),
    leave: vi.fn().mockResolvedValue(undefined),
    toggleMute: vi.fn().mockResolvedValue(undefined),
    toggleCamera: vi.fn().mockResolvedValue(undefined),
    refreshDevices: vi.fn().mockResolvedValue(undefined),
    getAudioLevel: vi.fn(() => ({ isSpeaking: false, audioLevel: 0 })),
    handleParticipantLeftEvent: vi.fn(),
    handleCallEndedEvent: vi.fn()
  },
  activeCallRooms: {
    active: false,
    load: vi.fn().mockResolvedValue(undefined),
    has: vi.fn(() => callStore.activeCallRooms.active),
    handleEnd: vi.fn()
  },
  callParticipants: {
    participants: [] as Array<{
      userId: string;
      displayName: string;
      login: string;
      avatarUrl: string | null;
    }>,
    load: vi.fn().mockResolvedValue(undefined),
    clear: vi.fn(),
    handleJoin: vi.fn(),
    handleLeave: vi.fn(),
    handleEnd: vi.fn()
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

vi.mock('$lib/hooks/useEvent.svelte', () => ({
  useEvent: vi.fn(),
  usePresenceChange: vi.fn()
}));

vi.mock('$lib/hooks', () => ({
  useEvent: vi.fn()
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
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
    room: { id: 'room-1', name: 'general', type: 'CHANNEL' },
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
  queryMock.mockResolvedValue({
    data: {
      room: {
        members: {
          users: members,
          totalCount,
          hasMore
        }
      }
    },
    error: null
  });
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
  beforeEach(() => {
    queryMock.mockReset();
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
    callStore.voiceCall.participants = [];
    callStore.voiceCall.isInCall.mockClear();
    callStore.voiceCall.join.mockClear();
    callStore.voiceCall.leave.mockClear();
    callStore.voiceCall.toggleMute.mockClear();
    callStore.voiceCall.toggleCamera.mockClear();
    callStore.voiceCall.refreshDevices.mockClear();
    callStore.voiceCall.getAudioLevel.mockClear();
    callStore.activeCallRooms.active = false;
    callStore.activeCallRooms.load.mockClear();
    callStore.activeCallRooms.has.mockClear();
    callStore.callParticipants.participants = [];
    callStore.callParticipants.load.mockClear();
    callStore.callParticipants.clear.mockClear();
    callStore.callParticipants.handleJoin.mockClear();
    callStore.callParticipants.handleLeave.mockClear();
    callStore.callParticipants.handleEnd.mockClear();
    callStore.handleVoiceCallJoinFailed.mockClear();
  });

  it('shows the exact total count and automatically loads additional member pages', async () => {
    const firstPage = Array.from({ length: 50 }, (_, index) => member(index + 1));
    const secondPage = Array.from({ length: 92 }, (_, index) => member(index + 51));

    queryMock
      .mockResolvedValueOnce({
        data: {
          room: {
            members: {
              users: firstPage,
              totalCount: 142,
              hasMore: true
            }
          }
        },
        error: null
      })
      .mockResolvedValueOnce({
        data: {
          room: {
            members: {
              users: secondPage,
              totalCount: 142,
              hasMore: false
            }
          }
        },
        error: null
      });

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false)
      }
    });

    await expect.element(q(container, 'h1')).toHaveTextContent('Members (142)');
    await vi.waitFor(() => {
      expect(renderedMemberTitles(container)).toHaveLength(50);
    });
    await vi.waitFor(() => {
      expect(
        container.querySelector('[data-testid="room-members-load-more-sentinel"]')
      ).toBeTruthy();
      expect(MockIntersectionObserver.instances).toHaveLength(1);
    });

    MockIntersectionObserver.instances[0].trigger();
    await tick();

    await vi.waitFor(() => {
      expect(queryMock).toHaveBeenCalledWith(expect.anything(), {
        roomId: 'room-1',
        search: null,
        limit: 50,
        offset: 0
      });
      expect(queryMock).toHaveBeenCalledWith(expect.anything(), {
        roomId: 'room-1',
        search: null,
        limit: 50,
        offset: 50
      });
    });

    await expect.element(q(container, 'h1')).toHaveTextContent('Members (142)');
    await vi.waitFor(() => {
      expect(
        container.querySelector('[data-testid="room-members-load-more-sentinel"]')
      ).toBeFalsy();
    });

    const renderedTitles = renderedMemberTitles(container);
    expect(renderedTitles).toHaveLength(142);
    for (let index = 1; index <= 142; index++) {
      expect(renderedTitles).toContain(`View profile of User ${index}`);
    }
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
    await expect.element(q(container, '[data-testid="call-join-button"]')).toHaveTextContent(
      'Start call'
    );
    expect(container.textContent).not.toContain('No active call');
    expect(container.textContent).not.toContain("Start one when you're ready.");

    (q(container, '[data-testid="call-join-button"]') as HTMLButtonElement).click();
    await tick();

    expect(callStore.voiceCall.join).toHaveBeenCalledWith('wss://livekit.example.test', 'room-1');
  });

  it('renders projected call participants before joining', async () => {
    callStore.activeCallRooms.active = true;
    callStore.callParticipants.participants = [
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
    await expect.element(q(container, '[data-testid="call-participants-list"]')).toBeInTheDocument();
    await vi.waitFor(() => {
      expect(callStore.callParticipants.load).toHaveBeenCalledWith('room-1');
    });
  });

  it('refreshes active-call room state when the call tab opens for an observer', async () => {
    render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false),
        activePanel: 'call',
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    await vi.waitFor(() => {
      expect(callStore.activeCallRooms.load).toHaveBeenCalledOnce();
    });
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
        videoTrack
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
        videoTrack: null
      }
    ];

    const { container } = render(RoomSidebarTestHarness, {
      props: {
        roomData: roomData([], 0, false),
        activePanel: 'call',
        livekitUrl: 'wss://livekit.example.test'
      }
    });

    await expect.element(q(container, '[data-testid="call-participant-panel"]')).toBeInTheDocument();
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

    (q(container, '[data-testid="call-mute-toggle"]') as HTMLButtonElement).click();
    (q(container, '[data-testid="call-camera-toggle"]') as HTMLButtonElement).click();
    (q(container, '[data-testid="call-leave-button"]') as HTMLButtonElement).click();
    await tick();

    expect(callStore.voiceCall.toggleMute).toHaveBeenCalledOnce();
    expect(callStore.voiceCall.toggleCamera).toHaveBeenCalledOnce();
    expect(callStore.voiceCall.leave).toHaveBeenCalledOnce();
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
        videoTrack
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
        videoTrack: videoTrackA
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
        videoTrack: videoTrackB
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

  it('keeps existing pagination state when automatic pagination fails and allows retry', async () => {
    const firstPage = Array.from({ length: 50 }, (_, index) => member(index + 1));
    const secondPage = Array.from({ length: 92 }, (_, index) => member(index + 51));
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    queryMock
      .mockResolvedValueOnce({
        data: {
          room: {
            members: {
              users: firstPage,
              totalCount: 142,
              hasMore: true
            }
          }
        },
        error: null
      })
      .mockResolvedValueOnce({
        data: {
          room: null
        },
        error: new Error('network failed')
      })
      .mockResolvedValueOnce({
        data: {
          room: {
            members: {
              users: secondPage,
              totalCount: 142,
              hasMore: false
            }
          }
        },
        error: null
      });

    try {
      const { container } = render(RoomSidebarTestHarness, {
        props: {
          roomData: roomData([], 0, false)
        }
      });

      await expect.element(q(container, 'h1')).toHaveTextContent('Members (142)');
      await vi.waitFor(() => {
        expect(renderedMemberTitles(container)).toHaveLength(50);
      });

      await vi.waitFor(() => {
        expect(MockIntersectionObserver.instances).toHaveLength(1);
      });

      MockIntersectionObserver.instances[0].trigger();
      await tick();

      await vi.waitFor(() => {
        expect(queryMock).toHaveBeenCalledWith(expect.anything(), {
          roomId: 'room-1',
          search: null,
          limit: 50,
          offset: 50
        });
      });

      await expect.element(q(container, 'h1')).toHaveTextContent('Members (142)');
      expect(renderedMemberTitles(container)).toHaveLength(50);
      await vi.waitFor(() => {
        expect(
          container.querySelector('[data-testid="room-members-load-more-sentinel"]')
        ).toBeTruthy();
      });

      MockIntersectionObserver.instances[0].trigger();
      await tick();

      await vi.waitFor(() => {
        expect(queryMock).toHaveBeenCalledTimes(3);
      });

      await vi.waitFor(() => {
        expect(renderedMemberTitles(container)).toHaveLength(142);
        expect(
          container.querySelector('[data-testid="room-members-load-more-sentinel"]')
        ).toBeFalsy();
      });
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });

  it('searches room members and resets the rendered member page', async () => {
    queryMock
      .mockResolvedValueOnce({
        data: {
          room: {
            members: {
              users: [member(1), member(2)],
              totalCount: 2,
              hasMore: false
            }
          }
        },
        error: null
      })
      .mockResolvedValueOnce({
        data: {
          room: {
            members: {
              users: [{ ...member(2), displayName: 'Boris Member' }],
              totalCount: 1,
              hasMore: false
            }
          }
        },
        error: null
      });

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
      expect(queryMock).toHaveBeenCalledWith(expect.anything(), {
        roomId: 'room-1',
        search: 'bor',
        limit: 50,
        offset: 0
      });
      expect(renderedMemberTitles(container)).toEqual(['View profile of Boris Member']);
      expect(q(container, 'h1')?.textContent).toContain('Members (1)');
    });
  });

  it('shows an empty result instead of retrying forever when search returns no members', async () => {
    queryMock
      .mockResolvedValueOnce({
        data: {
          room: {
            members: {
              users: [member(1), member(2)],
              totalCount: 2,
              hasMore: false
            }
          }
        },
        error: null
      })
      .mockResolvedValueOnce({
        data: {
          room: {
            members: {
              users: [],
              totalCount: 0,
              hasMore: false
            }
          }
        },
        error: null
      });

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
      expect(q(container, 'h1')?.textContent).toContain('Members (0)');
      expect(renderedMemberTitles(container)).toEqual([]);
    });

    await new Promise((resolve) => setTimeout(resolve, 350));
    expect(queryMock).toHaveBeenCalledTimes(2);
  });

  it('falls back to loaded-page filtering when room member search is unsupported', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    queryMock
      .mockResolvedValueOnce({
        data: {
          room: {
            members: {
              users: [member(1), { ...member(2), displayName: 'Boris Member' }],
              totalCount: 2,
              hasMore: false
            }
          }
        },
        error: null
      })
      .mockResolvedValueOnce({
        data: null,
        error: {
          graphQLErrors: [{ message: 'Unknown argument "search" on field "Room.members".' }]
        }
      })
      .mockResolvedValueOnce({
        data: {
          room: {
            members: {
              users: [member(1), { ...member(2), displayName: 'Boris Member' }],
              totalCount: 2,
              hasMore: false
            }
          }
        },
        error: null
      });

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
        expect(queryMock).toHaveBeenCalledWith(expect.anything(), {
          roomId: 'room-1',
          search: 'bor',
          limit: 50,
          offset: 0
        });
        expect(queryMock).toHaveBeenCalledWith(expect.anything(), {
          roomId: 'room-1',
          limit: 50,
          offset: 0
        });
        expect(renderedMemberTitles(container)).toEqual(['View profile of Boris Member']);
      });
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
    presenceCache!.update(user.id, PresenceStatus.Away);
    await tick();

    expect(presenceBadge(container, 'Away')).toBeTruthy();
    expect(buttonByText(container, 'Online (1)')).toBeTruthy();

    presenceCache!.update(user.id, PresenceStatus.Online);
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
    queryMock.mockResolvedValue({
      data: {
        room: {
          attachments: {
            items: [],
            totalCount: 0,
            hasMore: false
          }
        }
      },
      error: null
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

  it('keeps the files panel usable when the optional attachments field is unsupported', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    queryMock.mockResolvedValue({
      data: null,
      error: {
        graphQLErrors: [
          {
            message: 'Cannot query field "attachments" on type "Room".'
          }
        ]
      }
    });

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
      expect(consoleErrorSpy).not.toHaveBeenCalled();
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });

  it('renders room files, opens their message anchors, and automatically loads more', async () => {
    const onOpenFile = vi.fn();
    queryMock
      .mockResolvedValueOnce({
        data: {
          room: {
            attachments: {
              items: [roomFile('root-message', null, 'root.txt')],
              totalCount: 2,
              hasMore: true
            }
          }
        },
        error: null
      })
      .mockResolvedValueOnce({
        data: {
          room: {
            attachments: {
              items: [roomFile('thread-message', 'thread-root', 'thread.txt')],
              totalCount: 2,
              hasMore: false
            }
          }
        },
        error: null
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
      expect(queryMock).toHaveBeenCalledWith(expect.anything(), {
        roomId: 'room-1',
        limit: 50,
        offset: 1
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

    queryMock
      .mockResolvedValueOnce({
        data: {
          room: {
            attachments: {
              items: [
                roomFile('today-message', null, 'today.txt', '2026-06-17T08:00:00Z'),
                roomFile('yesterday-message', null, 'yesterday.txt', '2026-06-16T08:00:00Z')
              ],
              totalCount: 5,
              hasMore: true
            }
          }
        },
        error: null
      })
      .mockResolvedValueOnce({
        data: {
          room: {
            attachments: {
              items: [
                roomFile('week-message', null, 'week.txt', '2026-06-15T08:00:00Z'),
                roomFile('month-message', null, 'month.txt', '2026-06-10T08:00:00Z'),
                roomFile('older-month-message', null, 'older-month.txt', '2026-05-21T08:00:00Z')
              ],
              totalCount: 5,
              hasMore: false
            }
          }
        },
        error: null
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

  it('falls back to a file icon when a video thumbnail fails to load', async () => {
    queryMock
      .mockResolvedValueOnce({
        data: {
          room: {
            attachments: {
              items: [roomVideoFile('clip.mp4')],
              totalCount: 1,
              hasMore: false
            }
          }
        },
        error: null
      })
      .mockResolvedValueOnce({
        data: {
          room: {
            event: {
              event: {
                __typename: 'MessagePostedEvent',
                attachments: []
              }
            }
          }
        },
        error: null
      });

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
    queryMock.mockResolvedValueOnce({
      data: {
        room: {
          attachments: {
            items: [roomAudioFile('song.mp3')],
            totalCount: 1,
            hasMore: false
          }
        }
      },
      error: null
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
