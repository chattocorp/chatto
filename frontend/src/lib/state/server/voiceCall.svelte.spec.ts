import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  getVoiceCallJoinErrorMessage,
  VoiceCallJoinError,
  VoiceCallState
} from './voiceCall.svelte';

const calls: string[] = [];
let lastRoomOptions: Record<string, unknown> | null = null;
let lastKeyProvider: { setKey: ReturnType<typeof vi.fn> } | null = null;
let lastRoom: {
  disconnect: ReturnType<typeof vi.fn>;
  localParticipant: {
    setScreenShareEnabled: ReturnType<typeof vi.fn>;
    setCameraEnabled: ReturnType<typeof vi.fn>;
  };
} | null = null;
let connectFailure: Error | null = null;
let screenShareFailure: Error | null = null;
let roomEventHandlers = new Map<string, () => void>();
let localTrackPublications: Array<{
  isMuted: boolean;
  track: { source: string; mediaStreamTrack?: MediaStreamTrack };
}> = [];

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
      setCameraEnabled: vi.fn(async (enabled: boolean) => {
        calls.push(`setCameraEnabled:${enabled}`);
        localTrackPublications = localTrackPublications.filter((pub) => pub.track.source !== 'camera');
        if (enabled) {
          localTrackPublications.push({
            isMuted: false,
            track: { source: 'camera' }
          });
        }
      }),
      setScreenShareEnabled: vi.fn(async (enabled: boolean) => {
        calls.push(`setScreenShareEnabled:${enabled}`);
        if (screenShareFailure) {
          throw screenShareFailure;
        }
        localTrackPublications = localTrackPublications.filter(
          (pub) => pub.track.source !== 'screen_share'
        );
        if (enabled) {
          localTrackPublications.push({
            isMuted: false,
            track: { source: 'screen_share' }
          });
        }
      }),
      getTrackPublication: vi.fn(),
      identity: 'local-user',
      name: 'Local User',
      metadata: '',
      connectionQuality: 'excellent',
      isSpeaking: false,
      audioLevel: 0,
      getTrackPublications: vi.fn(() => localTrackPublications)
    };
    remoteParticipants = new Map();

    constructor(options: Record<string, unknown>) {
      lastRoomOptions = options;
      lastRoom = { disconnect: this.disconnect, localParticipant: this.localParticipant };
    }

    on = vi.fn((event: string, handler: () => void) => {
      roomEventHandlers.set(event, handler);
      return this;
    });
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
      TrackUnpublished: 'TrackUnpublished',
      LocalTrackPublished: 'LocalTrackPublished',
      LocalTrackUnpublished: 'LocalTrackUnpublished'
    },
    Track: {
      Kind: { Audio: 'audio' },
      Source: { Microphone: 'microphone', Camera: 'camera', ScreenShare: 'screen_share' }
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

function createVoiceCallClient() {
  return {
    mutation: vi.fn(() => ({
      toPromise: vi.fn(async () => ({ data: { joinVoiceCall: true } }))
    })),
    query: vi.fn(() => ({
      toPromise: vi.fn(async () => ({
        data: {
          room: {
            voiceCallToken: {
              token: 'livekit-token',
              e2eeKey: 'shared-e2ee-key',
              callId: 'call-1'
            }
          }
        }
      }))
    }))
  };
}

describe('VoiceCallState', () => {
  beforeEach(() => {
    calls.length = 0;
    lastRoomOptions = null;
    lastKeyProvider = null;
    lastRoom = null;
    connectFailure = null;
    screenShareFailure = null;
    roomEventHandlers = new Map();
    localTrackPublications = [];
    vi.stubGlobal('Worker', class MockWorker {});
    vi.stubGlobal('TransformStream', class MockTransformStream {});
    vi.stubGlobal('ReadableStream', class MockReadableStream {});
    vi.stubGlobal('WritableStream', class MockWritableStream {});
    vi.stubGlobal('RTCRtpScriptTransform', class MockRTCRtpScriptTransform {});
    vi.stubGlobal('crypto', { subtle: {} });
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
                e2eeKey: 'shared-e2ee-key',
                callId: 'call-1'
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

  it('fails before recording join intent when encrypted calls are unsupported', async () => {
    vi.stubGlobal('RTCRtpScriptTransform', undefined);
    vi.stubGlobal('RTCRtpSender', class MockRTCRtpSender {});

    const client = {
      mutation: vi.fn(() => ({
        toPromise: vi.fn(async () => ({ data: { joinVoiceCall: true } }))
      })),
      query: vi.fn()
    };

    const state = new VoiceCallState(client as never);

    await expect(state.join('wss://livekit.example.test', 'R1')).rejects.toThrow(
      VoiceCallJoinError
    );

    expect(client.mutation).not.toHaveBeenCalled();
    expect(client.query).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(false);
  });

  it('maps signaling failures to an actionable join error message', () => {
    const error = new Error('could not establish signal connection: Abort handler called');

    expect(getVoiceCallJoinErrorMessage(error)).toBe(
      'Could not reach the voice server. Check your network and try again.'
    );
  });

  it('coalesces duplicate joins for the same room while connecting', async () => {
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
                e2eeKey: 'shared-e2ee-key',
                callId: 'call-1'
              }
            }
          }
        }))
      }))
    };

    const state = new VoiceCallState(client as never);
    await Promise.all([
      state.join('wss://livekit.example.test', 'R1'),
      state.join('wss://livekit.example.test', 'R1')
    ]);

    expect(client.mutation).toHaveBeenCalledTimes(1);
    expect(client.query).toHaveBeenCalledTimes(1);
    expect(calls.filter((call) => call === 'connect')).toHaveLength(1);
  });

  it('coalesces duplicate leave actions while the leave intent is in flight', async () => {
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
                e2eeKey: 'shared-e2ee-key',
                callId: 'call-1'
              }
            }
          }
        }))
      }))
    };

    const state = new VoiceCallState(client as never);
    await state.join('wss://livekit.example.test', 'R1');

    await Promise.all([state.leave(), state.leave()]);

    expect(client.mutation).toHaveBeenCalledTimes(2);
    expect(lastRoom?.disconnect).toHaveBeenCalledOnce();
    expect(state.isInAnyCall).toBe(false);
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
                e2eeKey: 'shared-e2ee-key',
                callId: 'call-1'
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

  it('disconnects without recording leave when the backend ends the current call', async () => {
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
                e2eeKey: 'shared-e2ee-key',
                callId: 'call-1'
              }
            }
          }
        }))
      }))
    };

    const state = new VoiceCallState(client as never);
    await state.join('wss://livekit.example.test', 'R1');

    state.handleCallEndedEvent('R1', 'old-call');
    expect(lastRoom?.disconnect).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(true);

    state.handleCallEndedEvent('R1', 'call-1');

    expect(lastRoom?.disconnect).toHaveBeenCalledOnce();
    expect(client.mutation).toHaveBeenCalledTimes(1);
    expect(state.isInAnyCall).toBe(false);
  });

  it('disconnects only for the current user participant leave event', async () => {
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
                e2eeKey: 'shared-e2ee-key',
                callId: 'call-1'
              }
            }
          }
        }))
      }))
    };

    const state = new VoiceCallState(client as never);
    await state.join('wss://livekit.example.test', 'R1');

    state.handleParticipantLeftEvent('R1', 'call-1', 'remote-user', 'local-user');
    expect(lastRoom?.disconnect).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(true);

    state.handleParticipantLeftEvent('R1', 'old-call', 'local-user', 'local-user');
    expect(lastRoom?.disconnect).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(true);

    state.handleParticipantLeftEvent('R1', 'call-1', 'local-user', 'local-user');
    expect(lastRoom?.disconnect).toHaveBeenCalledOnce();
    expect(client.mutation).toHaveBeenCalledTimes(1);
    expect(state.isInAnyCall).toBe(false);
  });

  it('toggles video-only screen sharing through LiveKit', async () => {
    const client = createVoiceCallClient();
    const state = new VoiceCallState(client as never);
    await state.join('wss://livekit.example.test', 'R1');

    await state.toggleScreenShare();

    expect(lastRoom?.localParticipant.setScreenShareEnabled).toHaveBeenCalledWith(true);
    expect(state.isScreenShareEnabled).toBe(true);
    expect(state.participants[0]).toMatchObject({
      identity: 'local-user',
      isCameraEnabled: false,
      isScreenShareEnabled: true
    });
    expect(state.participants[0].videoTrack).toBeNull();
    expect(state.participants[0].screenShareTrack).toMatchObject(localTrackPublications[0].track);

    await state.toggleScreenShare();

    expect(lastRoom?.localParticipant.setScreenShareEnabled).toHaveBeenCalledWith(false);
    expect(state.isScreenShareEnabled).toBe(false);
    expect(state.participants[0].screenShareTrack).toBeNull();
  });

  it('keeps the call connected when screen capture fails', async () => {
    const client = createVoiceCallClient();
    const state = new VoiceCallState(client as never);
    await state.join('wss://livekit.example.test', 'R1');
    screenShareFailure = new Error('permission denied');

    await state.toggleScreenShare();

    expect(lastRoom?.localParticipant.setScreenShareEnabled).toHaveBeenCalledWith(true);
    expect(state.isScreenShareEnabled).toBe(false);
    expect(state.isInAnyCall).toBe(true);
    expect(state.roomId).toBe('R1');
  });

  it('keeps camera and screen-share tracks separate', async () => {
    const client = createVoiceCallClient();
    const state = new VoiceCallState(client as never);
    await state.join('wss://livekit.example.test', 'R1');

    await state.toggleCamera();
    const cameraTrack = localTrackPublications.find((pub) => pub.track.source === 'camera')!.track;
    await state.toggleScreenShare();
    const screenShareTrack = localTrackPublications.find(
      (pub) => pub.track.source === 'screen_share'
    )!.track;

    expect(state.participants[0]).toMatchObject({
      isCameraEnabled: true,
      isScreenShareEnabled: true
    });
    expect(state.participants[0].videoTrack).toMatchObject(cameraTrack);
    expect(state.participants[0].screenShareTrack).toMatchObject(screenShareTrack);
    expect(cameraTrack).not.toBe(screenShareTrack);
  });

  it('clears screen-share state on leave', async () => {
    const client = createVoiceCallClient();
    const state = new VoiceCallState(client as never);
    await state.join('wss://livekit.example.test', 'R1');
    await state.toggleScreenShare();

    await state.leave();

    expect(state.isScreenShareEnabled).toBe(false);
    expect(state.participants).toEqual([]);
  });

  it('updates screen-share state when LiveKit reports local unpublish', async () => {
    const client = createVoiceCallClient();
    const state = new VoiceCallState(client as never);
    await state.join('wss://livekit.example.test', 'R1');
    await state.toggleScreenShare();
    expect(state.isScreenShareEnabled).toBe(true);

    localTrackPublications = [];
    roomEventHandlers.get('LocalTrackUnpublished')?.();

    expect(state.isScreenShareEnabled).toBe(false);
    expect(state.participants[0].screenShareTrack).toBeNull();
  });
});
