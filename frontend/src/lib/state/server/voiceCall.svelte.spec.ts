import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  GetVoiceCallTokenResponse,
  JoinVoiceCallResponse,
  LeaveVoiceCallResponse,
  VoiceCallTokenView
} from '$lib/pb/chatto/api/v1/chat_pb';
import { VoiceCallState } from './voiceCall.svelte';

const calls: string[] = [];
let lastRoomOptions: Record<string, unknown> | null = null;
let lastKeyProvider: { setKey: ReturnType<typeof vi.fn> } | null = null;
let lastRoom: { disconnect: ReturnType<typeof vi.fn> } | null = null;
let connectFailure: Error | null = null;

vi.mock('livekit-client', () => {
  class MockExternalE2EEKeyProvider {
    setKey: ReturnType<typeof vi.fn>;

    constructor() {
      const setKey = vi.fn(async (key: string) => {
        calls.push(`setKey:${key}`);
      });
      this.setKey = setKey;
      lastKeyProvider = { setKey };
    }
  }

  class MockRoom {
    static getLocalDevices = vi.fn(async () => []);

    localParticipant = {
      setMicrophoneEnabled: vi.fn(async () => {
        calls.push('setMicrophoneEnabled');
      }),
      getTrackPublication: vi.fn(),
      identity: 'local-user',
      name: 'Local User',
      metadata: '',
      connectionQuality: 'excellent',
      isSpeaking: false,
      audioLevel: 0,
      getTrackPublications: vi.fn(() => [])
    };
    remoteParticipants = new Map();

    constructor(options: Record<string, unknown>) {
      lastRoomOptions = options;
      lastRoom = { disconnect: this.disconnect };
    }

    on = vi.fn();
    connect = vi.fn(async () => {
      calls.push('connect');
      if (connectFailure) {
        throw connectFailure;
      }
    });
    setE2EEEnabled = vi.fn(async (enabled: boolean) => {
      calls.push(`setE2EEEnabled:${enabled}`);
    });
    disconnect = vi.fn();
    removeAllListeners = vi.fn();
  }

  return {
    Room: MockRoom,
    ExternalE2EEKeyProvider: MockExternalE2EEKeyProvider,
    RoomEvent: {
      ParticipantConnected: 'ParticipantConnected',
      ParticipantDisconnected: 'ParticipantDisconnected',
      TrackMuted: 'TrackMuted',
      TrackUnmuted: 'TrackUnmuted',
      Disconnected: 'Disconnected',
      MediaDevicesChanged: 'MediaDevicesChanged',
      ConnectionQualityChanged: 'ConnectionQualityChanged',
      TrackSubscribed: 'TrackSubscribed',
      TrackUnsubscribed: 'TrackUnsubscribed',
      TrackPublished: 'TrackPublished',
      TrackUnpublished: 'TrackUnpublished'
    },
    Track: {
      Kind: { Audio: 'audio' },
      Source: { Microphone: 'microphone', Camera: 'camera' }
    },
    AudioPresets: { speech: {} },
    VideoPresets: { h720: { resolution: {} } }
  };
});

vi.mock('livekit-client/e2ee-worker?worker', () => ({
  default: class MockE2EEWorker {
    terminate = vi.fn();
  }
}));

function makeWireClient(overrides: Record<string, unknown> = {}) {
  return {
    joinVoiceCall: vi.fn(async () => new JoinVoiceCallResponse({ joined: true })),
    getVoiceCallToken: vi.fn(
      async () =>
        new GetVoiceCallTokenResponse({
          token: new VoiceCallTokenView({
            token: 'livekit-token',
            e2eeKey: 'shared-e2ee-key',
            callId: 'call-1'
          })
        })
    ),
    leaveVoiceCall: vi.fn(async () => new LeaveVoiceCallResponse({ left: true })),
    ...overrides
  };
}

describe('VoiceCallState', () => {
  beforeEach(() => {
    calls.length = 0;
    lastRoomOptions = null;
    lastKeyProvider = null;
    lastRoom = null;
    connectFailure = null;
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sets up LiveKit E2EE before connecting', async () => {
    const client = makeWireClient();

    const state = new VoiceCallState('server_1', () => client as never);
    await state.join('wss://livekit.example.test', 'R1');

    expect(client.joinVoiceCall).toHaveBeenCalledOnce();
    expect(lastKeyProvider?.setKey).toHaveBeenCalledWith('shared-e2ee-key');
    expect(lastRoomOptions?.encryption).toMatchObject({
      keyProvider: lastKeyProvider
    });
    expect(calls.indexOf('setKey:shared-e2ee-key')).toBeLessThan(
      calls.indexOf('setE2EEEnabled:true')
    );
    expect(calls.indexOf('setE2EEEnabled:true')).toBeLessThan(calls.indexOf('connect'));
  });

  it('records a compensating leave when LiveKit connect fails after join intent', async () => {
    connectFailure = new Error('connect failed');
    const client = makeWireClient();

    const state = new VoiceCallState('server_1', () => client as never);

    await expect(state.join('wss://livekit.example.test', 'R1')).rejects.toThrow(
      'connect failed'
    );

    expect(client.joinVoiceCall).toHaveBeenCalledOnce();
    expect(client.leaveVoiceCall).toHaveBeenCalledOnce();
    expect(client.leaveVoiceCall).toHaveBeenCalledWith(expect.objectContaining({ roomId: 'R1' }));
    expect(state.isInAnyCall).toBe(false);
  });

  it('disconnects without recording leave when the backend ends the current call', async () => {
    const client = makeWireClient();

    const state = new VoiceCallState('server_1', () => client as never);
    await state.join('wss://livekit.example.test', 'R1');

    state.handleCallEndedEvent('R1', 'old-call');
    expect(lastRoom?.disconnect).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(true);

    state.handleCallEndedEvent('R1', 'call-1');

    expect(lastRoom?.disconnect).toHaveBeenCalledOnce();
    expect(client.joinVoiceCall).toHaveBeenCalledOnce();
    expect(client.leaveVoiceCall).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(false);
  });

  it('disconnects only for the current user participant leave event', async () => {
    const client = makeWireClient();

    const state = new VoiceCallState('server_1', () => client as never);
    await state.join('wss://livekit.example.test', 'R1');

    state.handleParticipantLeftEvent('R1', 'call-1', 'remote-user', 'local-user');
    expect(lastRoom?.disconnect).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(true);

    state.handleParticipantLeftEvent('R1', 'old-call', 'local-user', 'local-user');
    expect(lastRoom?.disconnect).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(true);

    state.handleParticipantLeftEvent('R1', 'call-1', 'local-user', 'local-user');
    expect(lastRoom?.disconnect).toHaveBeenCalledOnce();
    expect(client.joinVoiceCall).toHaveBeenCalledOnce();
    expect(client.leaveVoiceCall).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(false);
  });
});
