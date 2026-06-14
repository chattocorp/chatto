import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { VoiceCallState } from './voiceCall.svelte';

const calls: string[] = [];
let lastRoomOptions: Record<string, unknown> | null = null;
let lastKeyProvider: { setKey: ReturnType<typeof vi.fn> } | null = null;
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

describe('VoiceCallState', () => {
  beforeEach(() => {
    calls.length = 0;
    lastRoomOptions = null;
    lastKeyProvider = null;
    connectFailure = null;
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sets up LiveKit E2EE before connecting', async () => {
    const client = {
      mutation: vi.fn(() => ({
        toPromise: vi.fn(async () => ({ data: { joinVoiceCall: true } }))
      })),
      query: vi.fn(() => ({
        toPromise: vi.fn(async () => ({
          data: {
            room: {
              voiceCallToken: {
                token: 'livekit-token',
                e2eeKey: 'shared-e2ee-key'
              }
            }
          }
        }))
      }))
    };

    const state = new VoiceCallState(client as never);
    await state.join('wss://livekit.example.test', 'R1');

    expect(client.mutation).toHaveBeenCalled();
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
    const client = {
      mutation: vi.fn(() => ({
        toPromise: vi.fn(async () => ({ data: { joinVoiceCall: true } }))
      })),
      query: vi.fn(() => ({
        toPromise: vi.fn(async () => ({
          data: {
            room: {
              voiceCallToken: {
                token: 'livekit-token',
                e2eeKey: 'shared-e2ee-key'
              }
            }
          }
        }))
      }))
    };

    const state = new VoiceCallState(client as never);

    await expect(state.join('wss://livekit.example.test', 'R1')).rejects.toThrow(
      'connect failed'
    );

    expect(client.mutation).toHaveBeenCalledTimes(2);
    expect(client.mutation).toHaveBeenNthCalledWith(2, expect.anything(), { roomId: 'R1' });
    expect(state.isInAnyCall).toBe(false);
  });
});
