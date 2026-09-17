import { CallPreferencesState } from './callPreferences.svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { VoiceCallAPI } from '$lib/api-client/voiceCalls';

const { gameCaptureMocks, soundMocks, toastMocks } = vi.hoisted(() => ({
  gameCaptureMocks: {
    start: vi.fn()
  },
  soundMocks: {
    playCallSound: vi.fn(() => Promise.resolve())
  },
  toastMocks: {
    error: vi.fn()
  }
}));

vi.mock('$lib/desktop/nativeScreenSharePublisher', () => ({
  NativeScreenSharePublisherSession: { start: gameCaptureMocks.start }
}));

vi.mock('$lib/audio/callSounds', () => ({
  playCallSound: soundMocks.playCallSound
}));

vi.mock('$lib/ui/toast', () => ({
  toast: toastMocks
}));

import {
  getVoiceCallMediaDeviceErrorMessage,
  getVoiceCallJoinErrorMessage,
  VoiceCallJoinError,
  VoiceCallState
} from './voiceCall.svelte';
import { Room } from 'livekit-client';
import { TrackAudioLevels } from '$lib/audio/trackAudioLevels';
import { participantVolumeGain } from '$lib/audio/participantVolume';

const calls: string[] = [];
let lastRoomOptions: Record<string, unknown> | null = null;
let lastKeyProvider: { setKey: ReturnType<typeof vi.fn> } | null = null;
let lastRoom: {
  disconnect: ReturnType<typeof vi.fn>;
  localParticipant: {
    setMicrophoneEnabled: ReturnType<typeof vi.fn>;
    setScreenShareEnabled: ReturnType<typeof vi.fn>;
    setCameraEnabled: ReturnType<typeof vi.fn>;
    publishTrack: ReturnType<typeof vi.fn>;
    unpublishTrack: ReturnType<typeof vi.fn>;
  };
  switchActiveDevice: ReturnType<typeof vi.fn>;
} | null = null;
let mockMicrophonePublication:
  | {
      source?: string;
      isMuted?: boolean;
      audioTrack: {
        getProcessor: ReturnType<typeof vi.fn>;
        setProcessor: ReturnType<typeof vi.fn>;
        mediaStream?: MediaStream;
      };
    }
  | undefined;
let connectFailure: Error | null = null;
let connectGate: { promise: Promise<void>; resolve: () => void } | null = null;
let microphoneGate: { promise: Promise<void>; resolve: () => void } | null = null;
let microphoneFailure: Error | null = null;
let cameraGate: { promise: Promise<void>; resolve: () => void } | null = null;
let cameraFailure: Error | null = null;
let screenShareGate: { promise: Promise<void>; resolve: () => void } | null = null;
let screenShareFailure: Error | null = null;
let screenShareFailureAfterUpdate: Error | null = null;
let switchActiveDeviceFailure: Error | null = null;
let activeMicrophone: string | undefined;
let roomEventHandlers = new Map<string, (...args: unknown[]) => void>();
let localTrackPublications: Array<{
  isMuted: boolean;
  track: { source: string; mediaStreamTrack?: MediaStreamTrack };
}> = [];
let mockRemoteParticipants = new Map<string, unknown>();

let processorConstructionFails = false;
vi.mock('$lib/audio/microphoneProcessor', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/audio/microphoneProcessor')>();
  return {
    ...actual,
    MicrophoneProcessor: class extends actual.MicrophoneProcessor {
      constructor(...args: ConstructorParameters<typeof actual.MicrophoneProcessor>) {
        if (processorConstructionFails) throw new Error('Processor unavailable');
        super(...args);
      }
    }
  };
});

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
    static getLocalDevices = vi.fn(async (kind?: MediaDeviceKind) => {
      if (kind === 'audioinput') {
        return [{ deviceId: 'audio-input-1', kind, label: 'Microphone' }];
      }
      if (kind === 'audiooutput') {
        return [{ deviceId: 'audio-output-1', kind, label: 'Speaker' }];
      }
      if (kind === 'videoinput') {
        return [{ deviceId: 'video-input-1', kind, label: 'Camera' }];
      }
      return [];
    });

    localParticipant = {
      setMicrophoneEnabled: vi.fn(async (enabled: boolean) => {
        calls.push('setMicrophoneEnabled');
        await microphoneGate?.promise;
        if (enabled && microphoneFailure) {
          roomEventHandlers.get('MediaDevicesError')?.(microphoneFailure, 'audioinput');
          throw microphoneFailure;
        }
      }),
      setCameraEnabled: vi.fn(async (enabled: boolean) => {
        calls.push(`setCameraEnabled:${enabled}`);
        await cameraGate?.promise;
        if (enabled && cameraFailure) {
          roomEventHandlers.get('MediaDevicesError')?.(cameraFailure, 'videoinput');
          throw cameraFailure;
        }
        localTrackPublications = localTrackPublications.filter(
          (pub) => pub.track.source !== 'camera'
        );
        if (enabled) {
          localTrackPublications.push({
            isMuted: false,
            track: { source: 'camera' }
          });
        }
      }),
      setScreenShareEnabled: vi.fn(async (enabled: boolean) => {
        calls.push(`setScreenShareEnabled:${enabled}`);
        await screenShareGate?.promise;
        if (screenShareFailure) {
          roomEventHandlers.get('MediaDevicesError')?.(screenShareFailure, 'videoinput');
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
        if (screenShareFailureAfterUpdate) {
          roomEventHandlers.get('MediaDevicesError')?.(screenShareFailureAfterUpdate, 'videoinput');
          throw screenShareFailureAfterUpdate;
        }
      }),
      publishTrack: vi.fn(async (track: MediaStreamTrack, options: { source: string }) => {
        calls.push(`publishTrack:${options.source}`);
        localTrackPublications.push({
          isMuted: false,
          track: { source: options.source, mediaStreamTrack: track }
        });
      }),
      unpublishTrack: vi.fn(async (track: MediaStreamTrack) => {
        calls.push(`unpublishTrack:${track.kind}`);
        localTrackPublications = localTrackPublications.filter(
          (publication) => publication.track.mediaStreamTrack !== track
        );
      }),
      getTrackPublication: vi.fn(() => mockMicrophonePublication),
      identity: 'local-user',
      name: 'Local User',
      metadata: '',
      connectionQuality: 'excellent',
      isSpeaking: false,
      audioLevel: 0,
      getTrackPublications: vi.fn(() => localTrackPublications)
    };
    remoteParticipants = mockRemoteParticipants;

    constructor(options: Record<string, unknown>) {
      lastRoomOptions = options;
      lastRoom = {
        disconnect: this.disconnect,
        localParticipant: this.localParticipant,
        switchActiveDevice: this.switchActiveDevice
      };
    }

    on = vi.fn((event: string, handler: () => void) => {
      roomEventHandlers.set(event, handler);
      return this;
    });
    switchActiveDevice = vi.fn(async (kind: MediaDeviceKind, deviceId: string) => {
      calls.push(`switchActiveDevice:${kind}:${deviceId}`);
      if (switchActiveDeviceFailure) {
        roomEventHandlers.get('MediaDevicesError')?.(switchActiveDeviceFailure, kind);
        throw switchActiveDeviceFailure;
      }
      if (kind === 'audioinput') activeMicrophone = deviceId;
    });
    getActiveDevice(kind: MediaDeviceKind) {
      return kind === 'audioinput' ? activeMicrophone : undefined;
    }
    connect = vi.fn(async () => {
      calls.push('connect');
      await connectGate?.promise;
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
      AudioPlaybackStatusChanged: 'AudioPlaybackStatusChanged',
      Reconnected: 'Reconnected',
      ParticipantConnected: 'ParticipantConnected',
      ParticipantDisconnected: 'ParticipantDisconnected',
      TrackMuted: 'TrackMuted',
      TrackUnmuted: 'TrackUnmuted',
      Disconnected: 'Disconnected',
      MediaDevicesChanged: 'MediaDevicesChanged',
      MediaDevicesError: 'MediaDevicesError',
      ConnectionQualityChanged: 'ConnectionQualityChanged',
      TrackSubscribed: 'TrackSubscribed',
      TrackUnsubscribed: 'TrackUnsubscribed',
      TrackPublished: 'TrackPublished',
      TrackUnpublished: 'TrackUnpublished',
      LocalTrackPublished: 'LocalTrackPublished',
      LocalAudioSilenceDetected: 'LocalAudioSilenceDetected',
      ActiveDeviceChanged: 'ActiveDeviceChanged',
      LocalTrackUnpublished: 'LocalTrackUnpublished'
    },
    Track: {
      Kind: { Audio: 'audio' },
      Source: {
        Microphone: 'microphone',
        Camera: 'camera',
        ScreenShare: 'screen_share',
        ScreenShareAudio: 'screen_share_audio'
      }
    },
    AudioPresets: {
      speech: { maxBitrate: 24_000 },
      musicStereo: { maxBitrate: 64_000 }
    },
    VideoPresets: { h720: { resolution: {} } }
  };
});

vi.mock('livekit-client/e2ee-worker?worker', () => ({
  default: class MockE2EEWorker {
    terminate = vi.fn();
  }
}));

function createVoiceCallClient(overrides: Partial<VoiceCallAPI> = {}): VoiceCallAPI {
  return {
    joinCall: vi.fn(async () => true),
    createCallToken: vi.fn(async () => ({
      token: 'livekit-token',
      e2eeKey: 'shared-e2ee-key',
      callId: 'call-1'
    })),
    createGameSharePublisherToken: vi.fn(async () => ({
      token: 'publisher-token',
      e2eeKey: 'shared-e2ee-key',
      callId: 'call-1'
    })),
    leaveCall: vi.fn(async () => true),
    ...overrides
  };
}

function deferredVoid(): { promise: Promise<void>; resolve: () => void } {
  let resolve!: () => void;
  const promise = new Promise<void>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

async function flushPromises(times = 5): Promise<void> {
  for (let i = 0; i < times; i++) {
    await Promise.resolve();
  }
}

function createPermittedCallState(api: VoiceCallAPI) {
  return new VoiceCallState(api, () => ({
    start: true,
    join: true,
    voice: true,
    camera: true,
    screenshare: true
  }));
}

describe('VoiceCallState', () => {
  beforeEach(() => {
    mockMicrophonePublication = undefined;
    calls.length = 0;
    lastRoomOptions = null;
    lastKeyProvider = null;
    lastRoom = null;
    connectFailure = null;
    connectGate = null;
    microphoneGate = null;
    microphoneFailure = null;
    cameraGate = null;
    cameraFailure = null;
    screenShareGate = null;
    screenShareFailure = null;
    screenShareFailureAfterUpdate = null;
    switchActiveDeviceFailure = null;
    activeMicrophone = undefined;
    roomEventHandlers = new Map();
    localTrackPublications = [];
    mockRemoteParticipants = new Map();
    gameCaptureMocks.start.mockReset();
    vi.stubGlobal(
      'AudioContext',
      class {
        state = 'running';
        setSinkId = vi.fn(async () => undefined);
        close = vi.fn(async () => {
          this.state = 'closed';
        });
        resume = vi.fn(async () => {
          this.state = 'running';
        });
      }
    );
    vi.stubGlobal('Worker', class MockWorker {});
    vi.stubGlobal('TransformStream', class MockTransformStream {});
    vi.stubGlobal('ReadableStream', class MockReadableStream {});
    vi.stubGlobal('WritableStream', class MockWritableStream {});
    vi.stubGlobal('RTCRtpScriptTransform', class MockRTCRtpScriptTransform {});
    vi.stubGlobal('crypto', { subtle: {} });
    soundMocks.playCallSound.mockClear();
    toastMocks.error.mockClear();
    vi.mocked(Room.getLocalDevices).mockClear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('denies a join when permission data is absent', async () => {
    const client = createVoiceCallClient();
    const state = new VoiceCallState(client);
    await expect(state.join('wss://livekit.example.test', 'R1')).rejects.toThrow(
      'Call join denied'
    );
    expect(client.joinCall).not.toHaveBeenCalled();
  });

  it('joins listen-only without requesting capture and rejects media actions', async () => {
    const state = new VoiceCallState(createVoiceCallClient(), () => ({
      start: true,
      join: true,
      voice: false,
      camera: false,
      screenshare: false
    }));
    await state.join('wss://livekit.example.test', 'R1');
    expect(lastRoom!.localParticipant.setMicrophoneEnabled).not.toHaveBeenCalledWith(true);
    expect(state.isMuted).toBe(true);
    expect(Room.getLocalDevices).toHaveBeenCalledWith('audioinput', false);
    expect(Room.getLocalDevices).toHaveBeenCalledWith('audiooutput', false);
    expect(Room.getLocalDevices).toHaveBeenCalledWith('videoinput', false);
    await state.toggleMute();
    await state.toggleCamera();
    await state.toggleScreenShare();
    await state.startNativeScreenShare('source', 'Application');
    expect(lastRoom!.localParticipant.setMicrophoneEnabled).not.toHaveBeenCalledWith(true);
    expect(lastRoom!.localParticipant.setCameraEnabled).not.toHaveBeenCalledWith(true);
    expect(lastRoom!.localParticipant.setScreenShareEnabled).not.toHaveBeenCalledWith(
      true,
      expect.anything()
    );
    expect(gameCaptureMocks.start).not.toHaveBeenCalled();
    await state.leave();
  });

  it('stops revoked microphone and camera but retains the call', async () => {
    const permissions = { start: true, join: true, voice: true, camera: true, screenshare: true };
    const state = new VoiceCallState(createVoiceCallClient(), () => permissions);
    await state.join('wss://livekit.example.test', 'R1');
    await state.toggleCamera();
    permissions.voice = false;
    permissions.camera = false;
    await state.reconcilePermissions();
    expect(lastRoom!.localParticipant.setMicrophoneEnabled).toHaveBeenLastCalledWith(false);
    expect(lastRoom!.localParticipant.setCameraEnabled).toHaveBeenLastCalledWith(false);
    expect(state.isMuted).toBe(true);
    expect(state.connected).toBe(true);
    permissions.join = false;
    await state.reconcilePermissions();
    expect(state.connected).toBe(false);
  });

  it('stops a capture that completes after permission loss', async () => {
    const permissions = { start: true, join: true, voice: true, camera: true, screenshare: true };
    const state = new VoiceCallState(createVoiceCallClient(), () => permissions);
    await state.join('wss://livekit.example.test', 'R1');
    let release!: () => void;
    cameraGate = {
      promise: new Promise<void>((resolve) => {
        release = resolve;
      }),
      resolve: () => release()
    };
    const enabling = state.toggleCamera();
    permissions.camera = false;
    const reconcile = state.reconcilePermissions();
    release();
    await enabling;
    await reconcile;
    expect(lastRoom!.localParticipant.setCameraEnabled).toHaveBeenLastCalledWith(false);
    expect(state.isCameraEnabled).toBe(false);
    await state.leave();
  });

  it('does not restore connected state when join access is lost during initial capture', async () => {
    const permissions = { start: true, join: true, voice: true, camera: true, screenshare: true };
    const state = new VoiceCallState(createVoiceCallClient(), () => permissions);
    let release!: () => void;
    microphoneGate = {
      promise: new Promise<void>((resolve) => {
        release = resolve;
      }),
      resolve: () => release()
    };
    const joining = state.join('wss://livekit.example.test', 'R1');
    await vi.waitFor(() =>
      expect(lastRoom?.localParticipant.setMicrophoneEnabled).toHaveBeenCalledWith(true)
    );
    const room = lastRoom!;
    permissions.join = false;
    await state.reconcilePermissions();
    release();
    await joining;
    expect(state.connected).toBe(false);
    expect(state.roomId).toBeNull();
    expect(room.disconnect).toHaveBeenCalled();
  });

  it('keeps a replacement listen-only call muted when old capture completes', async () => {
    const permissions = { start: true, join: true, voice: true, camera: true, screenshare: true };
    const state = new VoiceCallState(createVoiceCallClient(), () => permissions);
    const gate = deferredVoid();
    microphoneGate = gate;
    const firstJoin = state.join('wss://livekit.example.test', 'R1');
    await vi.waitFor(() =>
      expect(lastRoom?.localParticipant.setMicrophoneEnabled).toHaveBeenCalledWith(true)
    );
    permissions.join = false;
    await state.reconcilePermissions();
    permissions.join = true;
    permissions.voice = false;
    microphoneGate = null;
    await state.join('wss://livekit.example.test', 'R2');
    gate.resolve();
    await firstJoin;
    expect(state.roomId).toBe('R2');
    expect(state.connected).toBe(true);
    expect(state.isMuted).toBe(true);
    await state.leave();
  });

  it('rechecks native sharing permission after waiting for browser capture', async () => {
    const permissions = { start: true, join: true, voice: true, camera: true, screenshare: true };
    const state = new VoiceCallState(createVoiceCallClient(), () => permissions);
    await state.join('wss://livekit.example.test', 'R1');
    screenShareGate = deferredVoid();
    const browserShare = state.toggleScreenShare();
    const nativeShare = state.startNativeScreenShare('window:42', 'Application');
    permissions.screenshare = false;
    const reconcile = state.reconcilePermissions();
    screenShareGate.resolve();
    await browserShare;
    await nativeShare;
    await reconcile;
    expect(gameCaptureMocks.start).not.toHaveBeenCalled();
    expect(state.isScreenShareEnabled).toBe(false);
    await state.leave();
  });

  it('sets up LiveKit E2EE before connecting', async () => {
    const client = createVoiceCallClient();

    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');

    expect(client.joinCall).toHaveBeenCalledWith('R1');
    expect(lastKeyProvider?.setKey).toHaveBeenCalledWith('shared-e2ee-key');
    expect(lastRoomOptions?.encryption).toMatchObject({
      keyProvider: lastKeyProvider
    });
    expect(calls.indexOf('setKey:shared-e2ee-key')).toBeLessThan(
      calls.indexOf('setE2EEEnabled:true')
    );
    expect(calls.indexOf('setE2EEEnabled:true')).toBeLessThan(calls.indexOf('connect'));
  });

  it('keeps calls usable when optional processor construction fails', async () => {
    processorConstructionFails = true;
    const state = createPermittedCallState(createVoiceCallClient());
    try {
      await state.join('wss://livekit.example.test', 'R1');
      expect(state.connected).toBe(true);
      expect(state.isMuted).toBe(false);
      expect(state.microphoneGateUnavailable).toBe(true);
    } finally {
      processorConstructionFails = false;
      await state.leave();
    }
  });

  it.each([false, true])(
    'keeps a call usable when optional processor failure is %s',
    async (fails) => {
      const setProcessor = fails
        ? vi.fn().mockRejectedValue(new Error('Unavailable'))
        : vi.fn().mockResolvedValue(undefined);
      mockMicrophonePublication = { audioTrack: { getProcessor: vi.fn(), setProcessor } };
      const state = createPermittedCallState(createVoiceCallClient());
      await state.join('wss://livekit.example.test', 'R1');
      expect(state.connected).toBe(true);
      expect(state.isMuted).toBe(false);
      expect(setProcessor).toHaveBeenCalledOnce();
      expect(state.microphoneGateUnavailable).toBe(fails);
      expect(lastRoomOptions?.audioCaptureDefaults).not.toHaveProperty('processor');
      await state.leave();
    }
  );

  it.each([false, true])(
    'monitors sustained raw silence without SDK events (processor failed: %s)',
    async (failed) => {
      processorConstructionFails = failed;
      let input = 0.0001; // Tiny hardware noise must not suppress the hint.
      const track = { enabled: true, readyState: 'live' } as MediaStreamTrack;
      const sync = vi.spyOn(TrackAudioLevels.prototype, 'sync').mockImplementation(() => {});
      const has = vi.spyOn(TrackAudioLevels.prototype, 'has').mockReturnValue(true);
      const level = vi.spyOn(TrackAudioLevels.prototype, 'get').mockImplementation(() => input);
      let now = 100;
      const clock = vi.spyOn(performance, 'now').mockImplementation(() => now);
      mockMicrophonePublication = {
        source: 'microphone',
        isMuted: false,
        audioTrack: {
          getProcessor: vi.fn(),
          setProcessor: vi.fn().mockResolvedValue(undefined),
          mediaStream: { getAudioTracks: () => [track] } as MediaStream
        }
      };
      const state = createPermittedCallState(createVoiceCallClient());
      try {
        await state.join('wss://livekit.example.test', 'R1');
        const sample = async () => {
          state.microphoneLevel = 0.5;
          await expect.poll(() => state.microphoneLevel).toBe(0);
        };
        await sample();
        expect(sync).toHaveBeenCalledWith(expect.anything(), [['microphone', track]]);
        now += 3000;
        await sample();
        expect(state.microphoneSilent).toBe(false);
        now += 7100;
        await expect.poll(() => state.microphoneSilent).toBe(true);
        input = 0.001; // Pre-gate audio clears even when below the configured speech threshold.
        await expect.poll(() => state.microphoneSilent).toBe(false);
        expect(state.getAudioLevel('local-user')).toEqual({ isSpeaking: true, audioLevel: 0.001 });
        input = 0;
        await sample();
        now += 10100;
        await expect.poll(() => state.microphoneSilent).toBe(true);
        await state.toggleMute();
        expect(state.microphoneSilent).toBe(false);
        now += 20000;
        expect(state.microphoneSilent).toBe(false);
        await state.toggleMute();
        expect(state.microphoneSilent).toBe(false);
        await sample();
        now += 10100;
        await expect.poll(() => state.microphoneSilent).toBe(true);
        await state.setAudioDevice('audio-input-2');
        expect(state.microphoneSilent).toBe(false);
        await sample();
        now += 10100;
        await expect.poll(() => state.microphoneSilent).toBe(true);
        has.mockReturnValue(false);
        await expect.poll(() => state.microphoneSilent).toBe(false);
        await state.leave();
        expect(state.microphoneSilent).toBe(false);
      } finally {
        await state.leave();
        level.mockRestore();
        sync.mockRestore();
        has.mockRestore();
        processorConstructionFails = false;
        clock.mockRestore();
      }
    }
  );

  it('does not diagnose silence when there is no microphone capture', async () => {
    const state = createPermittedCallState(createVoiceCallClient());
    await state.join('wss://livekit.example.test', 'R1');
    try {
      expect(state.microphoneSilent).toBe(false);
      // An absent meter is not a silent microphone.
      const clock = vi.spyOn(performance, 'now').mockReturnValue(performance.now() + 10000);
      state.microphoneLevel = 0.5;
      await expect.poll(() => state.microphoneLevel).toBe(0);
      expect(state.microphoneSilent).toBe(false);
      clock.mockRestore();
    } finally {
      await state.leave();
    }
  });

  it('applies saved processing settings and live changes to the call processor', async () => {
    const { MicrophoneProcessor } = await import('$lib/audio/microphoneProcessor');
    const applied = vi.spyOn(MicrophoneProcessor.prototype, 'setEffects');
    const preferences = new CallPreferencesState('call-effects');
    preferences.setVoiceAmount(100);
    const state = new VoiceCallState(
      createVoiceCallClient(),
      () => ({
        start: true,
        join: true,
        voice: true,
        camera: true,
        screenshare: true
      }),
      preferences
    );
    try {
      await state.join('wss://livekit.example.test', 'R1');
      expect(applied).toHaveBeenCalledWith(
        expect.objectContaining({ bass: 8, treble: 10, compressor: true })
      );
      preferences.setVoiceAmount(0);
      await vi.waitFor(() =>
        expect(applied).toHaveBeenCalledWith(
          expect.objectContaining({ bass: 0, compressor: false })
        )
      );
    } finally {
      await state.leave();
      applied.mockRestore();
    }
  });

  it('restores saved devices and joins muted without capture prompts', async () => {
    const preferences = new CallPreferencesState('call-device-restore');
    preferences.setDevice('audioinput', 'preferred-mic');
    preferences.setDevice('audiooutput', 'missing-speaker');
    preferences.setDevice('videoinput', 'preferred-camera');
    preferences.setJoinMuted(true);
    const state = new VoiceCallState(
      createVoiceCallClient(),
      () => ({
        start: true,
        join: true,
        voice: true,
        camera: true,
        screenshare: true
      }),
      new CallPreferencesState('call-device-restore')
    );
    await state.join('wss://livekit.example.test', 'R1');
    expect(lastRoomOptions?.audioCaptureDefaults).toMatchObject({
      deviceId: { exact: 'preferred-mic' },
      autoGainControl: false
    });
    expect(lastRoomOptions?.videoCaptureDefaults).toMatchObject({
      deviceId: { ideal: 'preferred-camera' }
    });
    expect(lastRoomOptions?.audioOutput).toBeUndefined();
    expect(lastRoom?.localParticipant.setMicrophoneEnabled).not.toHaveBeenCalledWith(true);
    expect(lastRoom?.localParticipant.setCameraEnabled).not.toHaveBeenCalledWith(true);
    expect(Room.getLocalDevices).toHaveBeenCalledWith('audioinput', false);
    expect(Room.getLocalDevices).toHaveBeenCalledWith('videoinput', false);
    expect(preferences.speaker).toBe('missing-speaker');
    await state.toggleMute();
    expect(lastRoom?.localParticipant.setMicrophoneEnabled).toHaveBeenCalledWith(true);
    await state.leave();
  });

  it('reports the active microphone rather than the remembered preference', async () => {
    const preferences = new CallPreferencesState('actual-microphone');
    preferences.setDevice('audioinput', 'preferred-mic');
    activeMicrophone = 'actual-mic';
    const state = new VoiceCallState(
      createVoiceCallClient(),
      () => ({
        start: true,
        join: true,
        voice: true,
        camera: true,
        screenshare: true
      }),
      preferences
    );
    await state.join('wss://livekit.example.test', 'R1');
    expect(state.selectedDeviceId).toBe('actual-mic');
    expect(preferences.microphone).toBe('preferred-mic');
    roomEventHandlers.get('ActiveDeviceChanged')?.('audioinput', 'reported-input');
    expect(state.selectedDeviceId).toBe('reported-input');
    await state.setAudioDevice('');
    expect(lastRoom?.switchActiveDevice).toHaveBeenCalledWith('audioinput', 'default');
    expect(preferences.microphone).toBe('');
    await state.leave();
  });

  it('falls back from a missing saved microphone without forgetting it', async () => {
    const preferences = new CallPreferencesState('missing-microphone');
    preferences.setDevice('audioinput', 'missing-mic');
    preferences.setJoinMuted(true);
    const state = new VoiceCallState(
      createVoiceCallClient(),
      () => ({
        start: true,
        join: true,
        voice: true,
        camera: true,
        screenshare: true
      }),
      preferences
    );
    await state.join('wss://livekit.example.test', 'R1');
    lastRoom!.localParticipant.setMicrophoneEnabled.mockRejectedValueOnce(
      Object.assign(new Error('missing input'), {
        name: 'OverconstrainedError',
        constraint: 'deviceId'
      })
    );
    await state.toggleMute();
    expect(state.isMuted).toBe(false);
    expect(lastRoom?.switchActiveDevice).toHaveBeenCalledWith('audioinput', 'default');
    expect(preferences.microphone).toBe('missing-mic');
    await state.refreshDevices();
    expect(state.selectedDeviceId).toBe('default');
    await state.leave();
  });

  it('restores an available speaker and keeps saved media subordinate to permission', async () => {
    const preferences = new CallPreferencesState('call-device-permission');
    preferences.setDevice('audioinput', 'audio-input-1');
    preferences.setDevice('audiooutput', 'audio-output-1');
    preferences.setDevice('videoinput', 'video-input-1');
    const state = new VoiceCallState(
      createVoiceCallClient(),
      () => ({
        start: true,
        join: true,
        voice: false,
        camera: false,
        screenshare: false
      }),
      preferences
    );
    await state.join('wss://livekit.example.test', 'R1');
    expect(lastRoomOptions?.audioOutput).toEqual({ deviceId: 'audio-output-1' });
    expect(lastRoom?.localParticipant.setMicrophoneEnabled).not.toHaveBeenCalledWith(true);
    expect(lastRoom?.localParticipant.setCameraEnabled).not.toHaveBeenCalledWith(true);
    await state.leave();
  });

  it('does not save a device when LiveKit reports an unsuccessful switch', async () => {
    const preferences = new CallPreferencesState('call-device-false');
    preferences.setDevice('audioinput', 'working');
    const state = new VoiceCallState(
      createVoiceCallClient(),
      () => ({
        start: true,
        join: true,
        voice: true,
        camera: true,
        screenshare: true
      }),
      preferences
    );
    await state.join('wss://livekit.example.test', 'R1');
    lastRoom!.switchActiveDevice.mockResolvedValueOnce(false);
    await state.setAudioDevice('unavailable');
    expect(new CallPreferencesState('call-device-false').microphone).toBe('working');
    await state.leave();
  });

  it('remembers successful call device switches but not failed switches', async () => {
    const preferences = new CallPreferencesState('call-device-switch');
    const state = new VoiceCallState(
      createVoiceCallClient(),
      () => ({
        start: true,
        join: true,
        voice: true,
        camera: true,
        screenshare: true
      }),
      preferences
    );
    await state.join('wss://livekit.example.test', 'R1');
    await state.setAudioDevice('mic-two');
    await state.setAudioOutputDevice('speaker-two');
    await state.setVideoDevice('camera-two');
    switchActiveDeviceFailure = new Error('unavailable');
    await state.setAudioDevice('missing');
    const restored = new CallPreferencesState('call-device-switch');
    expect([restored.microphone, restored.speaker, restored.camera]).toEqual([
      'mic-two',
      'speaker-two',
      'camera-two'
    ]);
    await state.leave();
  });

  it('configures microphone capture and publication as mono', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);

    await state.join('wss://livekit.example.test', 'R1');

    expect(lastRoomOptions?.audioCaptureDefaults).toMatchObject({
      channelCount: { ideal: 1 }
    });
    expect(lastRoomOptions?.publishDefaults).toMatchObject({
      forceStereo: false
    });
  });

  it('does not play a join sound without the participant join event', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);

    await state.join('wss://livekit.example.test', 'R1');

    expect(soundMocks.playCallSound).not.toHaveBeenCalled();
  });

  it('joins with microphone enabled but does not request camera permission while refreshing devices', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);

    await state.join('wss://livekit.example.test', 'R1');

    expect(lastRoom?.localParticipant.setMicrophoneEnabled).toHaveBeenCalledWith(true);
    expect(lastRoom?.localParticipant.setCameraEnabled).not.toHaveBeenCalled();
    expect(Room.getLocalDevices).toHaveBeenCalledWith('audioinput', true);
    expect(Room.getLocalDevices).toHaveBeenCalledWith('audiooutput', true);
    expect(Room.getLocalDevices).toHaveBeenCalledWith('videoinput', false);
    expect(Room.getLocalDevices).not.toHaveBeenCalledWith('videoinput');
    expect(Room.getLocalDevices).not.toHaveBeenCalledWith('videoinput', true);
  });

  it('joins muted when microphone enable fails without enabling the camera', async () => {
    microphoneFailure = new Error('microphone unavailable');
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);

    await state.join('wss://livekit.example.test', 'R1');

    expect(lastRoom?.localParticipant.setMicrophoneEnabled).toHaveBeenCalledWith(true);
    expect(lastRoom?.localParticipant.setCameraEnabled).not.toHaveBeenCalled();
    expect(state.isMuted).toBe(true);
    expect(state.isInAnyCall).toBe(true);
    expect(Room.getLocalDevices).toHaveBeenCalledWith('videoinput', false);
    expect(toastMocks.error).toHaveBeenCalledWith(
      'Could not start your microphone. You joined muted.'
    );
    expect(toastMocks.error).toHaveBeenCalledOnce();
  });

  it('plays a deferred current-user join event after connecting successfully', async () => {
    connectGate = deferredVoid();
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);

    const join = state.join('wss://livekit.example.test', 'R1');
    expect(state.connecting).toBe(true);
    expect(state.roomId).toBe('R1');
    await flushPromises();

    expect(state.callTransitionSoundDecision('join', 'R1', 'call-1', true)).toBe('defer');
    expect(soundMocks.playCallSound).not.toHaveBeenCalled();

    connectGate.resolve();
    await join;

    expect(soundMocks.playCallSound).toHaveBeenCalledOnce();
    expect(soundMocks.playCallSound).toHaveBeenCalledWith('join');
  });

  it('fails before recording join intent when encrypted calls are unsupported', async () => {
    vi.stubGlobal('RTCRtpScriptTransform', undefined);
    vi.stubGlobal('RTCRtpSender', class MockRTCRtpSender {});

    const client = createVoiceCallClient();

    const state = createPermittedCallState(client);

    await expect(state.join('wss://livekit.example.test', 'R1')).rejects.toThrow(
      VoiceCallJoinError
    );

    expect(client.joinCall).not.toHaveBeenCalled();
    expect(client.createCallToken).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(false);
    expect(soundMocks.playCallSound).not.toHaveBeenCalled();
  });

  it('maps signaling failures to an actionable join error message', () => {
    const error = new Error('could not establish signal connection: Abort handler called');

    expect(getVoiceCallJoinErrorMessage(error)).toBe(
      'Could not reach the voice server. Check your network and try again.'
    );
  });

  it('coalesces duplicate joins for the same room while connecting', async () => {
    const client = createVoiceCallClient();

    const state = createPermittedCallState(client);
    await Promise.all([
      state.join('wss://livekit.example.test', 'R1'),
      state.join('wss://livekit.example.test', 'R1')
    ]);

    expect(client.joinCall).toHaveBeenCalledTimes(1);
    expect(client.createCallToken).toHaveBeenCalledTimes(1);
    expect(calls.filter((call) => call === 'connect')).toHaveLength(1);
  });

  it('coalesces duplicate leave actions while the leave intent is in flight', async () => {
    const client = createVoiceCallClient();

    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    soundMocks.playCallSound.mockClear();

    await Promise.all([state.leave(), state.leave()]);

    expect(client.joinCall).toHaveBeenCalledTimes(1);
    expect(client.leaveCall).toHaveBeenCalledTimes(1);
    expect(lastRoom?.disconnect).toHaveBeenCalledOnce();
    expect(state.isInAnyCall).toBe(false);
    expect(soundMocks.playCallSound).not.toHaveBeenCalled();
  });

  it('records a compensating leave when LiveKit connect fails after join intent', async () => {
    connectFailure = new Error('connect failed');
    const client = createVoiceCallClient();

    const state = createPermittedCallState(client);

    await expect(state.join('wss://livekit.example.test', 'R1')).rejects.toThrow('connect failed');

    expect(client.joinCall).toHaveBeenCalledTimes(1);
    expect(client.leaveCall).toHaveBeenCalledWith('R1');
    expect(state.isInAnyCall).toBe(false);
    expect(soundMocks.playCallSound).not.toHaveBeenCalled();
  });

  it('disconnects without recording leave when the backend ends the current call', async () => {
    const client = createVoiceCallClient();

    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    soundMocks.playCallSound.mockClear();

    state.handleCallEndedEvent('R1', 'old-call');
    expect(lastRoom?.disconnect).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(true);
    expect(soundMocks.playCallSound).not.toHaveBeenCalled();

    state.handleCallEndedEvent('R1', 'call-1');

    expect(lastRoom?.disconnect).toHaveBeenCalledOnce();
    expect(client.joinCall).toHaveBeenCalledTimes(1);
    expect(client.leaveCall).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(false);
    expect(soundMocks.playCallSound).not.toHaveBeenCalled();
  });

  it('disconnects local media without recording leave when room access is revoked', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    soundMocks.playCallSound.mockClear();

    state.handleRoomAccessRevoked('R2');
    expect(state.isInAnyCall).toBe(true);

    state.handleRoomAccessRevoked('R1');

    expect(lastRoom?.disconnect).toHaveBeenCalledOnce();
    expect(client.leaveCall).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(false);
    expect(soundMocks.playCallSound).not.toHaveBeenCalled();
  });

  it('disconnects only for the current user participant leave event', async () => {
    const client = createVoiceCallClient();

    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    soundMocks.playCallSound.mockClear();

    state.handleParticipantLeftEvent('R1', 'call-1', 'remote-user', 'local-user');
    expect(lastRoom?.disconnect).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(true);
    expect(soundMocks.playCallSound).not.toHaveBeenCalled();

    state.handleParticipantLeftEvent('R1', 'old-call', 'local-user', 'local-user');
    expect(lastRoom?.disconnect).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(true);
    expect(soundMocks.playCallSound).not.toHaveBeenCalled();

    state.handleParticipantLeftEvent('R1', 'call-1', 'local-user', 'local-user');
    expect(lastRoom?.disconnect).toHaveBeenCalledOnce();
    expect(client.joinCall).toHaveBeenCalledTimes(1);
    expect(client.leaveCall).not.toHaveBeenCalled();
    expect(state.isInAnyCall).toBe(false);
    expect(soundMocks.playCallSound).not.toHaveBeenCalled();
    expect(state.callTransitionSoundDecision('leave', 'R1', 'call-1', true)).toBe('play');
  });

  it('matches only the currently connected call', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');

    expect(state.matchesActiveCall('R1', 'call-1')).toBe(true);
    expect(state.matchesActiveCall('R1', 'old-call')).toBe(false);
    expect(state.matchesActiveCall('R2', 'call-1')).toBe(false);
    expect(state.matchesActiveCall('R1', null)).toBe(false);
  });

  it('toggles screen sharing with browser-tab audio through LiveKit', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');

    await state.toggleScreenShare();

    expect(lastRoom?.localParticipant.setScreenShareEnabled).toHaveBeenCalledWith(
      true,
      {
        audio: true,
        systemAudio: 'exclude'
      },
      {
        audioPreset: { maxBitrate: 64_000 },
        forceStereo: true,
        dtx: false,
        red: false
      }
    );
    expect(state.isScreenShareEnabled).toBe(true);
    expect(state.participants[0]).toMatchObject({
      identity: 'local-user',
      isCameraEnabled: false,
      isScreenShareEnabled: true
    });
    expect(state.participants[0].videoTrack).toBeNull();
    expect(state.participants[0].screenShareTrack).toMatchObject(localTrackPublications[0].track);

    await state.toggleScreenShare();

    expect(lastRoom?.localParticipant.setScreenShareEnabled).toHaveBeenCalledWith(
      false,
      undefined,
      undefined
    );
    expect(state.isScreenShareEnabled).toBe(false);
    expect(state.participants[0].screenShareTrack).toBeNull();
  });

  it('publishes camera and one native screen share under the local participant', async () => {
    const session = {
      stop: vi.fn(),
      onEnded: null as ((error?: Error) => void) | null
    };
    gameCaptureMocks.start.mockResolvedValue(session);
    const state = createPermittedCallState(createVoiceCallClient());
    await state.join('wss://livekit.example.test', 'R1');

    await state.toggleCamera();
    await state.startNativeScreenShare('window:42', 'Moonring');

    expect(gameCaptureMocks.start).toHaveBeenCalledWith({
      sourceId: 'window:42',
      livekitUrl: 'wss://livekit.example.test',
      token: 'publisher-token',
      e2eeKey: 'shared-e2ee-key'
    });
    expect(state.isCameraEnabled).toBe(true);
    expect(state.isNativeScreenShareEnabled).toBe(true);
    expect(state.nativeScreenShareSourceName).toBe('Moonring');
    expect(state.participants[0]).toMatchObject({
      isCameraEnabled: true,
      isScreenShareEnabled: true
    });

    await state.toggleScreenShare();

    expect(session.stop).toHaveBeenCalledOnce();
    expect(state.isCameraEnabled).toBe(true);
    expect(state.isNativeScreenShareEnabled).toBe(false);
    expect(state.isScreenShareEnabled).toBe(false);
    expect(lastRoom?.localParticipant.setScreenShareEnabled).not.toHaveBeenCalled();
  });

  it('keeps native screen sharing active until the publisher acknowledges stop', async () => {
    const stopGate = deferredVoid();
    const session = {
      stop: vi.fn(() => stopGate.promise),
      onEnded: null as ((error?: Error) => void) | null
    };
    gameCaptureMocks.start.mockResolvedValue(session);
    const state = createPermittedCallState(createVoiceCallClient());
    await state.join('wss://livekit.example.test', 'R1');
    await state.startNativeScreenShare('window:42', 'Moonring');

    const stopping = state.stopNativeScreenShare();
    await flushPromises();

    expect(session.stop).toHaveBeenCalledOnce();
    expect(state.isNativeScreenSharePending).toBe(true);
    expect(state.isNativeScreenShareEnabled).toBe(true);

    stopGate.resolve();
    await stopping;

    expect(state.isNativeScreenSharePending).toBe(false);
    expect(state.isNativeScreenShareEnabled).toBe(false);
    expect(state.isScreenShareEnabled).toBe(false);
  });

  it('coalesces repeated unified-control stops without starting a browser share', async () => {
    const stopGate = deferredVoid();
    const session = {
      stop: vi.fn(() => stopGate.promise),
      onEnded: null as ((error?: Error) => void) | null
    };
    gameCaptureMocks.start.mockResolvedValue(session);
    const state = createPermittedCallState(createVoiceCallClient());
    await state.join('wss://livekit.example.test', 'R1');
    await state.startNativeScreenShare('window:42', 'Moonring');

    const firstStop = state.toggleScreenShare();
    const repeatedStop = state.toggleScreenShare();
    await flushPromises();
    stopGate.resolve();
    await Promise.all([firstStop, repeatedStop]);

    expect(session.stop).toHaveBeenCalledOnce();
    expect(lastRoom?.localParticipant.setScreenShareEnabled).not.toHaveBeenCalled();
    expect(state.isScreenShareEnabled).toBe(false);
  });

  it('replaces an existing browser screen share when native capture starts', async () => {
    const session = {
      stop: vi.fn(),
      onEnded: null as ((error?: Error) => void) | null
    };
    gameCaptureMocks.start.mockResolvedValue(session);
    const state = createPermittedCallState(createVoiceCallClient());
    await state.join('wss://livekit.example.test', 'R1');
    await state.toggleScreenShare();

    await state.startNativeScreenShare('window:42', 'Moonring');

    expect(lastRoom?.localParticipant.setScreenShareEnabled).toHaveBeenLastCalledWith(false);
    expect(
      localTrackPublications.filter((publication) => publication.track.source === 'screen_share')
    ).toHaveLength(0);
    expect(state.isNativeScreenShareEnabled).toBe(true);
  });

  it('preserves an existing browser screen share when publisher credentials fail', async () => {
    const state = createPermittedCallState(
      createVoiceCallClient({
        createGameSharePublisherToken: vi.fn(async () => {
          throw new Error('credential request failed');
        })
      })
    );
    await state.join('wss://livekit.example.test', 'R1');
    await state.toggleScreenShare();

    await expect(state.startNativeScreenShare('window:42', 'Moonring')).rejects.toThrow(
      'credential request failed'
    );

    expect(gameCaptureMocks.start).not.toHaveBeenCalled();
    expect(lastRoom?.localParticipant.setScreenShareEnabled).not.toHaveBeenCalledWith(false);
    expect(
      localTrackPublications.filter((publication) => publication.track.source === 'screen_share')
    ).toHaveLength(1);
    expect(state.isScreenShareEnabled).toBe(true);
    expect(state.isNativeScreenShareEnabled).toBe(false);
  });

  it('preserves an existing browser screen share when native publisher startup fails', async () => {
    gameCaptureMocks.start.mockRejectedValue(new Error('helper startup failed'));
    const state = createPermittedCallState(createVoiceCallClient());
    await state.join('wss://livekit.example.test', 'R1');
    await state.toggleScreenShare();

    await expect(state.startNativeScreenShare('window:42', 'Moonring')).rejects.toThrow(
      'helper startup failed'
    );

    expect(lastRoom?.localParticipant.setScreenShareEnabled).not.toHaveBeenCalledWith(false);
    expect(
      localTrackPublications.filter((publication) => publication.track.source === 'screen_share')
    ).toHaveLength(1);
    expect(state.isScreenShareEnabled).toBe(true);
    expect(state.isNativeScreenShareEnabled).toBe(false);
  });

  it('stops a started native publisher when browser share replacement fails', async () => {
    const session = {
      stop: vi.fn(async () => undefined),
      onEnded: null as ((error?: Error) => void) | null
    };
    gameCaptureMocks.start.mockResolvedValue(session);
    const state = createPermittedCallState(createVoiceCallClient());
    await state.join('wss://livekit.example.test', 'R1');
    await state.toggleScreenShare();
    screenShareFailure = new Error('browser unpublish failed');

    await expect(state.startNativeScreenShare('window:42', 'Moonring')).rejects.toThrow(
      'browser unpublish failed'
    );

    expect(session.stop).toHaveBeenCalledOnce();
    expect(
      localTrackPublications.filter((publication) => publication.track.source === 'screen_share')
    ).toHaveLength(1);
    expect(state.isScreenShareEnabled).toBe(true);
    expect(state.isNativeScreenShareEnabled).toBe(false);
  });

  it('does not activate a native publisher that ends while browser unpublish is pending', async () => {
    const session = {
      stop: vi.fn(async () => undefined),
      onEnded: null as ((error?: Error) => void) | null
    };
    gameCaptureMocks.start.mockResolvedValue(session);
    const state = createPermittedCallState(createVoiceCallClient());
    await state.join('wss://livekit.example.test', 'R1');
    await state.toggleScreenShare();
    screenShareGate = deferredVoid();

    const starting = state.startNativeScreenShare('window:42', 'Moonring');
    await expect.poll(() => session.onEnded).not.toBeNull();
    session.onEnded?.(new Error('native publisher ended'));
    screenShareGate.resolve();

    await expect(starting).rejects.toThrow('native publisher ended');
    expect(state.isNativeScreenShareEnabled).toBe(false);
    expect(state.isScreenShareEnabled).toBe(false);
    expect(state.nativeScreenShareSourceName).toBeNull();
  });

  it('keeps native capture when browser unpublish removes its track before rejecting', async () => {
    const session = {
      stop: vi.fn(async () => undefined),
      onEnded: null as ((error?: Error) => void) | null
    };
    gameCaptureMocks.start.mockResolvedValue(session);
    const state = createPermittedCallState(createVoiceCallClient());
    await state.join('wss://livekit.example.test', 'R1');
    await state.toggleScreenShare();
    screenShareFailureAfterUpdate = new Error('late browser unpublish failure');

    await state.startNativeScreenShare('window:42', 'Moonring');

    expect(session.stop).not.toHaveBeenCalled();
    expect(
      localTrackPublications.filter((publication) => publication.track.source === 'screen_share')
    ).toHaveLength(0);
    expect(state.isNativeScreenShareEnabled).toBe(true);
    expect(state.isScreenShareEnabled).toBe(true);
    expect(state.nativeScreenShareSourceName).toBe('Moonring');
  });

  it('keeps microphone pending until LiveKit applies the toggle', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    microphoneGate = deferredVoid();

    const toggle = state.toggleMute();
    await flushPromises();

    expect(state.isMicrophonePending).toBe(true);
    expect(state.isMuted).toBe(false);
    expect(lastRoom?.localParticipant.setMicrophoneEnabled).toHaveBeenLastCalledWith(false);

    microphoneGate.resolve();
    await toggle;

    expect(state.isMicrophonePending).toBe(false);
    expect(state.isMuted).toBe(true);
  });

  it('keeps camera pending until LiveKit applies the toggle', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    cameraGate = deferredVoid();

    const toggle = state.toggleCamera();
    await flushPromises();

    expect(state.isCameraPending).toBe(true);
    expect(state.isCameraEnabled).toBe(false);
    expect(lastRoom?.localParticipant.setCameraEnabled).toHaveBeenLastCalledWith(true);

    cameraGate.resolve();
    await toggle;

    expect(state.isCameraPending).toBe(false);
    expect(state.isCameraEnabled).toBe(true);
  });

  it('refreshes devices without camera permission until camera is explicitly enabled', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    vi.mocked(Room.getLocalDevices).mockClear();

    await state.refreshDevices();
    roomEventHandlers.get('MediaDevicesChanged')?.();
    await flushPromises();

    expect(Room.getLocalDevices).toHaveBeenCalledWith('videoinput', false);
    expect(Room.getLocalDevices).not.toHaveBeenCalledWith('videoinput', true);

    vi.mocked(Room.getLocalDevices).mockClear();
    await state.toggleCamera();

    expect(lastRoom?.localParticipant.setCameraEnabled).toHaveBeenCalledWith(true);
    expect(Room.getLocalDevices).toHaveBeenCalledWith('videoinput', true);
  });

  it('keeps screen share pending until LiveKit applies the toggle', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    screenShareGate = deferredVoid();

    const toggle = state.toggleScreenShare();
    await flushPromises();

    expect(state.isScreenSharePending).toBe(true);
    expect(state.isScreenShareEnabled).toBe(false);
    expect(lastRoom?.localParticipant.setScreenShareEnabled).toHaveBeenLastCalledWith(
      true,
      {
        audio: true,
        systemAudio: 'exclude'
      },
      {
        audioPreset: { maxBitrate: 64_000 },
        forceStereo: true,
        dtx: false,
        red: false
      }
    );

    screenShareGate.resolve();
    await toggle;

    expect(state.isScreenSharePending).toBe(false);
    expect(state.isScreenShareEnabled).toBe(true);
  });

  it.each(['NotAllowedError', 'PermissionDeniedError'])(
    'silently handles a dismissed screen picker (%s) and permits retry',
    async (name) => {
      const state = createPermittedCallState(createVoiceCallClient());
      await state.join('wss://livekit.example.test', 'R1');
      toastMocks.error.mockClear();
      screenShareFailure = Object.assign(new Error('Permission denied'), { name });

      await state.toggleScreenShare();

      expect(state.isScreenSharePending).toBe(false);
      expect(state.isScreenShareEnabled).toBe(false);
      expect(state.isInAnyCall).toBe(true);
      expect(toastMocks.error).not.toHaveBeenCalled();

      screenShareFailure = null;
      await state.toggleScreenShare();

      expect(state.isScreenShareEnabled).toBe(true);
      expect(state.isScreenSharePending).toBe(false);
      expect(toastMocks.error).not.toHaveBeenCalled();
    }
  );

  it('keeps the call connected when screen capture fails', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    screenShareFailure = new Error('permission denied');

    await state.toggleScreenShare();

    expect(lastRoom?.localParticipant.setScreenShareEnabled).toHaveBeenCalledWith(
      true,
      {
        audio: true,
        systemAudio: 'exclude'
      },
      {
        audioPreset: { maxBitrate: 64_000 },
        forceStereo: true,
        dtx: false,
        red: false
      }
    );
    expect(state.isScreenShareEnabled).toBe(false);
    expect(state.isInAnyCall).toBe(true);
    expect(state.roomId).toBe('R1');
    expect(toastMocks.error).toHaveBeenCalledWith('Screen sharing was cancelled or blocked.');
    expect(toastMocks.error).toHaveBeenCalledOnce();
  });

  it('reports permission failures when enabling media devices', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    toastMocks.error.mockClear();

    microphoneFailure = Object.assign(new Error('Permission denied'), {
      name: 'NotAllowedError'
    });
    await state.toggleMute();
    expect(state.isMuted).toBe(true);
    expect(toastMocks.error).not.toHaveBeenCalled();

    await state.toggleMute();
    expect(state.isMuted).toBe(true);
    expect(toastMocks.error).toHaveBeenCalledWith(
      'Microphone access was denied. Check your browser permissions and try again.'
    );
    expect(toastMocks.error).toHaveBeenCalledOnce();

    cameraFailure = Object.assign(new Error('Device unavailable'), {
      name: 'NotReadableError'
    });
    toastMocks.error.mockClear();
    await state.toggleCamera();
    expect(state.isCameraEnabled).toBe(false);
    expect(toastMocks.error).toHaveBeenCalledWith('Your camera is already in use by another app.');
    expect(toastMocks.error).toHaveBeenCalledOnce();
  });

  it('reports LiveKit media device errors without disconnecting', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    toastMocks.error.mockClear();

    roomEventHandlers.get('MediaDevicesError')?.();

    expect(toastMocks.error).toHaveBeenCalledWith('Could not access a media device.');
    expect(toastMocks.error).toHaveBeenCalledOnce();
    expect(state.isInAnyCall).toBe(true);
  });

  it('keeps selected devices unchanged when device switching fails', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    toastMocks.error.mockClear();
    switchActiveDeviceFailure = Object.assign(new Error('device not found'), {
      name: 'NotFoundError'
    });

    await state.setAudioDevice('missing-mic');
    await state.setAudioOutputDevice('missing-speaker');
    await state.setVideoDevice('missing-camera');

    expect(state.selectedDeviceId).toBe('audio-input-1');
    expect(state.selectedOutputDeviceId).toBe('audio-output-1');
    expect(state.selectedVideoDeviceId).toBe('video-input-1');
    expect(toastMocks.error).toHaveBeenCalledWith(
      'No microphone was found. Choose another input device and try again.'
    );
    expect(toastMocks.error).toHaveBeenCalledWith(
      'Could not switch speakers. This browser or device may not support speaker selection.'
    );
    expect(toastMocks.error).toHaveBeenCalledWith(
      'No camera was found. Choose another camera and try again.'
    );
    expect(toastMocks.error).toHaveBeenCalledTimes(3);
    expect(toastMocks.error).not.toHaveBeenCalledWith('Could not access a media device.');
  });

  it('maps media device failures to specific user-facing messages', () => {
    expect(
      getVoiceCallMediaDeviceErrorMessage(
        'screen',
        Object.assign(new Error('permission denied'), { name: 'NotAllowedError' }),
        'enable'
      )
    ).toBe('Screen sharing was cancelled or blocked.');
    expect(
      getVoiceCallMediaDeviceErrorMessage(
        'microphone',
        Object.assign(new Error('already in use'), { name: 'NotReadableError' }),
        'join'
      )
    ).toBe('Your microphone is already in use by another app. You joined muted.');
  });

  it('keeps camera and screen-share tracks separate', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);
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
    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    await state.toggleScreenShare();

    await state.leave();

    expect(state.isScreenShareEnabled).toBe(false);
    expect(state.participants).toEqual([]);
  });

  it('updates screen-share state when LiveKit reports local unpublish', async () => {
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    await state.toggleScreenShare();
    expect(state.isScreenShareEnabled).toBe(true);

    localTrackPublications = [];
    roomEventHandlers.get('LocalTrackUnpublished')?.();

    expect(state.isScreenShareEnabled).toBe(false);
    expect(state.participants[0].screenShareTrack).toBeNull();
  });

  it('attaches and detaches subscribed screen-share audio', async () => {
    const setVolume = vi.fn();
    mockRemoteParticipants.set('remote-user', {
      identity: 'remote-user',
      name: 'Remote User',
      metadata: '',
      connectionQuality: 'good',
      isSpeaking: false,
      audioLevel: 0,
      setVolume,
      trackPublications: new Map(),
      getTrackPublications: vi.fn(() => [{ isMuted: false, track: { source: 'microphone' } }])
    });
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);
    await state.join('wss://livekit.example.test', 'R1');
    state.toggleParticipantLocalMute('remote-user');
    setVolume.mockClear();
    const screenShareAudio = {
      kind: 'audio',
      source: 'screen_share_audio',
      attach: vi.fn(),
      detach: vi.fn()
    };

    roomEventHandlers.get('TrackSubscribed')?.(
      screenShareAudio,
      {},
      mockRemoteParticipants.get('remote-user')
    );

    expect(screenShareAudio.attach).toHaveBeenCalledOnce();
    expect(setVolume).toHaveBeenCalledWith(0, 'microphone');
    expect(setVolume).toHaveBeenCalledWith(0, 'screen_share_audio');

    roomEventHandlers.get('TrackUnsubscribed')?.(screenShareAudio, {});

    expect(screenShareAudio.detach).toHaveBeenCalledOnce();
  });

  it('awaits output selection failures and closes its playback context on leave', async () => {
    const state = createPermittedCallState(createVoiceCallClient());
    await state.join('wss://livekit.example.test', 'R1');
    const context = (
      lastRoomOptions?.webAudioMix as {
        audioContext: AudioContext & { setSinkId: ReturnType<typeof vi.fn> };
      }
    ).audioContext;
    expect(state.audioBoostAvailable).toBe(true);
    context.setSinkId.mockRejectedValueOnce(new DOMException('Unavailable', 'NotFoundError'));
    lastRoom!.switchActiveDevice.mockClear();
    await state.setAudioOutputDevice('missing');
    expect(lastRoom!.switchActiveDevice).not.toHaveBeenCalled();
    expect(state.selectedOutputDeviceId).toBe('audio-output-1');
    expect(toastMocks.error).toHaveBeenCalled();
    await state.leave();
    expect(context.close).toHaveBeenCalledOnce();
    expect(state.audioBoostAvailable).toBe(false);
  });

  it('keeps basic playback and saved boost when Web Audio is unavailable', async () => {
    vi.stubGlobal(
      'AudioContext',
      class {
        constructor() {
          throw new Error('Unavailable');
        }
      }
    );
    const state = createPermittedCallState(createVoiceCallClient());
    await state.join('wss://livekit.example.test', 'R1');
    const setVolume = vi.fn();
    mockRemoteParticipants.set('remote-user', {
      identity: 'remote-user',
      setVolume,
      trackPublications: new Map()
    } as never);
    state.setParticipantVolume('remote-user', 'voiceVolume', 200);
    expect(state.audioBoostAvailable).toBe(false);
    expect(lastRoomOptions?.webAudioMix).toBe(false);
    expect(state.getParticipantAudio('remote-user').voiceVolume).toBe(200);
    expect(setVolume).toHaveBeenCalledWith(1, 'microphone');
    state.setParticipantVolume('remote-user', 'voiceVolume', 0);
    expect(setVolume).toHaveBeenCalledWith(0, 'microphone');
    await state.leave();
  });

  it('restores boosted voice and stream levels across mute, subscriptions, reconnects, and calls', async () => {
    const setVolume = vi.fn();
    const participant = {
      identity: 'remote-user',
      name: 'Remote User',
      metadata: '',
      connectionQuality: 'good',
      isSpeaking: false,
      audioLevel: 0,
      setVolume,
      trackPublications: new Map(),
      getTrackPublications: vi.fn(() => [])
    };
    mockRemoteParticipants.set('remote-user', participant);
    const preferences = new CallPreferencesState('participant-volumes');
    preferences.setParticipantVolume('remote-user', 'voiceVolume', 175);
    preferences.setParticipantVolume('remote-user', 'streamVolume', 40);
    const state = new VoiceCallState(
      createVoiceCallClient(),
      () => ({ start: true, join: true, voice: true, camera: true, screenshare: true }),
      preferences
    );
    await state.join('wss://livekit.example.test', 'R1');
    expect(lastRoomOptions?.webAudioMix).toHaveProperty('audioContext');
    expect(setVolume).toHaveBeenCalledWith(participantVolumeGain(175), 'microphone');
    expect(setVolume).toHaveBeenCalledWith(participantVolumeGain(40), 'screen_share_audio');
    state.toggleParticipantLocalMute('remote-user');
    expect(setVolume).toHaveBeenCalledWith(0, 'microphone');
    state.setParticipantVolume('remote-user', 'voiceVolume', 150);
    expect(setVolume).toHaveBeenLastCalledWith(0, 'screen_share_audio');
    state.toggleParticipantLocalMute('remote-user');
    expect(setVolume).toHaveBeenCalledWith(participantVolumeGain(150), 'microphone');
    const companionVolume = vi.fn();
    const companion = {
      ...participant,
      identity: 'publisher',
      metadata: JSON.stringify({ ownerIdentity: 'remote-user' }),
      setVolume: companionVolume
    };
    mockRemoteParticipants.set('publisher', companion);
    roomEventHandlers.get('TrackSubscribed')?.({ kind: 'audio', attach: vi.fn() }, {}, companion);
    expect(companionVolume).toHaveBeenCalledWith(participantVolumeGain(40), 'screen_share_audio');
    setVolume.mockClear();
    roomEventHandlers.get('Reconnected')?.();
    expect(setVolume).toHaveBeenCalledWith(participantVolumeGain(150), 'microphone');
    await state.leave();
    await state.join('wss://livekit.example.test', 'R2');
    expect(state.getParticipantAudio('remote-user')).toEqual({
      voiceVolume: 150,
      streamVolume: 40
    });
    state.resetParticipantAudio('remote-user');
    expect(setVolume).toHaveBeenCalledWith(1, 'microphone');
    expect(companionVolume).toHaveBeenCalledWith(1, 'screen_share_audio');
    await state.leave();
  });

  it('locally mutes and unmutes remote participant audio for the current session only', async () => {
    const setVolume = vi.fn();
    mockRemoteParticipants.set('remote-user', {
      identity: 'remote-user',
      name: 'Remote User',
      metadata: '',
      connectionQuality: 'good',
      isSpeaking: false,
      audioLevel: 0,
      setVolume,
      trackPublications: new Map(),
      getTrackPublications: vi.fn(() => [{ isMuted: false, track: { source: 'microphone' } }])
    });
    const client = createVoiceCallClient();
    const state = createPermittedCallState(client);

    await state.join('wss://livekit.example.test', 'R1');
    setVolume.mockClear();

    state.toggleParticipantLocalMute('remote-user');

    expect(state.isParticipantLocallyMuted('remote-user')).toBe(true);
    expect(setVolume).toHaveBeenCalledWith(0, 'microphone');
    expect(setVolume).toHaveBeenCalledWith(0, 'screen_share_audio');
    expect(state.participants.find((p) => p.identity === 'remote-user')).toMatchObject({
      isLocallyMuted: true
    });

    state.toggleParticipantLocalMute('remote-user');

    expect(state.isParticipantLocallyMuted('remote-user')).toBe(false);
    expect(setVolume).toHaveBeenCalledWith(1, 'microphone');
    expect(setVolume).toHaveBeenCalledWith(1, 'screen_share_audio');

    state.toggleParticipantLocalMute('local-user');
    expect(state.isParticipantLocallyMuted('local-user')).toBe(false);

    state.toggleParticipantLocalMute('remote-user');
    expect(state.isParticipantLocallyMuted('remote-user')).toBe(true);

    await state.leave();

    expect(state.isParticipantLocallyMuted('remote-user')).toBe(false);
    expect(state.locallyMutedParticipantIds).toEqual({});
  });

  it('preserves bot identity from LiveKit participant metadata', async () => {
    mockRemoteParticipants.set('automation-bot', {
      identity: 'automation-bot',
      name: 'Automation Bot',
      metadata: `{"login":"automation_bot","isBot":true}`,
      connectionQuality: 'good',
      isSpeaking: false,
      audioLevel: 0,
      setVolume: vi.fn(),
      trackPublications: new Map(),
      getTrackPublications: vi.fn(() => [])
    });
    const state = createPermittedCallState(createVoiceCallClient());

    await state.join('wss://livekit.example.test', 'R1');

    expect(
      state.participants.find((participant) => participant.identity === 'automation-bot')
    ).toMatchObject({ login: 'automation_bot', isBot: true });
  });

  it.each(['local', 'remote', 'companion'])(
    'meters the %s screen publisher independently of its microphone',
    async (kind) => {
      const media = { id: 'screen-audio' } as MediaStreamTrack;
      const audio = {
        isMuted: false,
        track: { source: 'screen_share_audio', mediaStreamTrack: media }
      };
      const publications = [{ isMuted: false, track: { source: 'screen_share' } }, audio];
      const remote = (identity: string, metadata = '{}') => ({
        identity,
        metadata,
        name: identity,
        connectionQuality: 'good',
        isSpeaking: true,
        audioLevel: 0.9,
        setVolume: vi.fn(),
        trackPublications: new Map(),
        getTrackPublications: () =>
          identity === 'remote-user' && kind === 'companion' ? [] : publications
      });
      if (kind === 'local') localTrackPublications = publications;
      else {
        mockRemoteParticipants.set('remote-user', remote('remote-user'));
        if (kind === 'companion')
          mockRemoteParticipants.set(
            'publisher-1',
            remote('publisher-1', '{"publisherKind":"game_share","ownerIdentity":"remote-user"}')
          );
      }
      const sync = vi.spyOn(TrackAudioLevels.prototype, 'sync').mockImplementation(() => {});
      const tracks = () => new Map(sync.mock.calls.at(-1)?.[1]);
      const state = createPermittedCallState(createVoiceCallClient());
      try {
        await state.join('wss://livekit.example.test', 'R1');
        const identity = kind === 'local' ? 'local-user' : 'remote-user';
        expect(tracks().get(identity)).toBe(media);
        expect(tracks().has('publisher-1')).toBe(false);
        expect(state.getScreenShareAudioLevel(identity)).toBe(0);
        audio.isMuted = true;
        roomEventHandlers.get('TrackMuted')?.();
        expect(tracks().size).toBe(0);
        audio.isMuted = false;
        roomEventHandlers.get('TrackUnmuted')?.();
        expect(tracks().get(identity)).toBe(media);
        publications.splice(1, 1);
        roomEventHandlers.get('TrackUnsubscribed')?.({ detach: vi.fn() }, {});
        expect(tracks().size).toBe(0);
      } finally {
        await state.leave();
        sync.mockRestore();
      }
    }
  );

  it('meters received microphone tracks independently of speaker reports and playback gain', async () => {
    const first = {} as MediaStreamTrack;
    const replacement = {} as MediaStreamTrack;
    const streams = new WeakMap<TrackAudioLevels, Map<string, MediaStreamTrack>>();
    const levels = new Map([
      [first, 0.1],
      [replacement, 0.3]
    ]);
    const sync = vi.spyOn(TrackAudioLevels.prototype, 'sync').mockImplementation(function (
      this: TrackAudioLevels,
      entriesContext,
      entries
    ) {
      streams.set(this, new Map(entriesContext ? entries : []));
    });
    const get = vi.spyOn(TrackAudioLevels.prototype, 'get').mockImplementation(function (
      this: TrackAudioLevels,
      identity
    ) {
      const track = streams.get(this)?.get(identity);
      return track ? (levels.get(track) ?? 0) : 0;
    });
    const microphone = {
      isMuted: false,
      track: { source: 'microphone', mediaStreamTrack: first, detach: vi.fn() }
    };
    const publications = [microphone];
    const volume = vi.fn();
    mockRemoteParticipants.set('remote-user', {
      identity: 'remote-user',
      name: 'Remote',
      metadata: '{}',
      connectionQuality: 'good',
      isSpeaking: false,
      audioLevel: 0,
      setVolume: volume,
      trackPublications: new Map(),
      getTrackPublications: () => publications
    });
    const state = createPermittedCallState(createVoiceCallClient());
    try {
      await state.join('wss://livekit.example.test', 'R1');
      await expect.poll(() => state.getAudioLevel('remote-user').audioLevel).toBe(0.1);
      expect(state.getAudioLevel('remote-user').isSpeaking).toBe(true);
      state.toggleParticipantLocalMute('remote-user');
      expect(volume).toHaveBeenCalledWith(0, 'microphone');
      levels.set(first, 0.2);
      await expect.poll(() => state.getAudioLevel('remote-user').audioLevel).toBe(0.2);
      // Received silence wins even without any new server speaking event.
      levels.set(first, 0);
      await expect
        .poll(() => state.getAudioLevel('remote-user'))
        .toEqual({ isSpeaking: false, audioLevel: 0 });
      microphone.track.mediaStreamTrack = replacement;
      roomEventHandlers.get('TrackUnmuted')?.();
      await expect.poll(() => state.getAudioLevel('remote-user').audioLevel).toBe(0.3);
      microphone.isMuted = true;
      roomEventHandlers.get('TrackMuted')?.();
      await expect.poll(() => state.getAudioLevel('remote-user').audioLevel).toBe(0);
      microphone.isMuted = false;
      roomEventHandlers.get('TrackUnmuted')?.();
      await expect.poll(() => state.getAudioLevel('remote-user').audioLevel).toBe(0.3);
      publications.length = 0;
      roomEventHandlers.get('TrackUnsubscribed')?.(microphone.track, {});
      await expect.poll(() => state.getAudioLevel('remote-user').audioLevel).toBe(0);
      await state.leave();
      expect(state.getAudioLevel('remote-user').audioLevel).toBe(0);
    } finally {
      await state.leave();
      sync.mockRestore();
      get.mockRestore();
    }
  });

  it('merges a companion screen-share publisher into its owning participant', async () => {
    const gameVideoTrack = { source: 'screen_share' };
    const ownerSetVolume = vi.fn();
    const companionSetVolume = vi.fn();
    mockRemoteParticipants.set('remote-user', {
      identity: 'remote-user',
      name: 'Remote User',
      metadata: '{"login":"remote"}',
      connectionQuality: 'good',
      isSpeaking: false,
      audioLevel: 0,
      setVolume: ownerSetVolume,
      trackPublications: new Map(),
      getTrackPublications: vi.fn(() => [{ isMuted: false, track: { source: 'microphone' } }])
    });
    mockRemoteParticipants.set('publisher-1', {
      identity: 'publisher-1',
      name: 'Remote User',
      metadata: '{"publisherKind":"game_share","ownerIdentity":"remote-user"}',
      connectionQuality: 'good',
      isSpeaking: false,
      audioLevel: 0,
      setVolume: companionSetVolume,
      trackPublications: new Map(),
      getTrackPublications: vi.fn(() => [{ isMuted: false, track: gameVideoTrack }])
    });

    const state = createPermittedCallState(createVoiceCallClient());
    await state.join('wss://livekit.example.test', 'R1');

    expect(state.participants.map((participant) => participant.identity)).toEqual([
      'local-user',
      'remote-user'
    ]);
    expect(state.participants[1]).toMatchObject({
      login: 'remote',
      isScreenShareEnabled: true,
      screenShareTrack: gameVideoTrack
    });

    state.toggleParticipantLocalMute('remote-user');
    expect(ownerSetVolume).toHaveBeenCalledWith(0, 'microphone');
    expect(companionSetVolume).toHaveBeenCalledWith(0, 'microphone');
  });

  it('does not attach native screen-share audio published by the local companion', async () => {
    const companion = {
      identity: 'publisher-1',
      name: 'Local User',
      metadata: '{"publisherKind":"game_share","ownerIdentity":"local-user"}',
      connectionQuality: 'good',
      isSpeaking: false,
      audioLevel: 0,
      setVolume: vi.fn(),
      trackPublications: new Map(),
      getTrackPublications: vi.fn(() => [])
    };
    mockRemoteParticipants.set('publisher-1', companion);
    const state = createPermittedCallState(createVoiceCallClient());
    await state.join('wss://livekit.example.test', 'R1');
    const gameAudio = { kind: 'audio', attach: vi.fn(), detach: vi.fn() };

    roomEventHandlers.get('TrackSubscribed')?.(gameAudio, {}, companion);

    expect(gameAudio.attach).not.toHaveBeenCalled();
    expect(companion.setVolume).toHaveBeenCalledWith(0, 'microphone');
    expect(companion.setVolume).toHaveBeenCalledWith(0, 'screen_share_audio');
  });
});
