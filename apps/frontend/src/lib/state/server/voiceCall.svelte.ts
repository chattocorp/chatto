import type { MicrophoneProcessor } from '$lib/audio/microphoneProcessor';
import { microphoneMeter } from '$lib/audio/noiseGate';
/**
 * Voice call state — manages LiveKit connection for voice/video calls.
 *
 * Per-instance class that wraps livekit-client's Room instance.
 * Handles joining/leaving calls, mute toggle, camera toggle,
 * screen share toggle, and audio/video device selection.
 */

import {
  CallPreferencesState,
  availableCallDevice,
  DEFAULT_PARTICIPANT_AUDIO,
  normalizeParticipantVolume,
  type ParticipantAudioPreferences,
  type ParticipantVolumeControl
} from './callPreferences.svelte';
import type {
  Participant,
  RemoteTrack,
  RemoteTrackPublication,
  RemoteParticipant,
  Room,
  Track
} from 'livekit-client';
import { toast } from '$lib/ui/toast';
import { playCallSound } from '$lib/audio/callSounds';
import { m } from '$lib/i18n/messages';
import type { VoiceCallAPI } from '$lib/api-client/voiceCalls';
import type { NativeScreenSharePublisherSession } from '$lib/desktop/nativeScreenSharePublisher';

/** Resolved room actions. Missing permission data always denies access. */
export type CallPermissions = {
  start: boolean;
  join: boolean;
  voice: boolean;
  camera: boolean;
  screenshare: boolean;
};

export const NO_CALL_PERMISSIONS: CallPermissions = {
  start: false,
  join: false,
  voice: false,
  camera: false,
  screenshare: false
};

export type CallParticipantInfo = {
  identity: string;
  name: string;
  login: string;
  avatarUrl: string | null;
  isBot?: boolean;
  isMuted: boolean;
  isLocal: boolean;
  connectionQuality: 'excellent' | 'good' | 'poor' | 'lost' | 'unknown';
  isCameraEnabled: boolean;
  videoTrack: Track | null;
  isScreenShareEnabled: boolean;
  screenShareTrack: Track | null;
  isLocallyMuted: boolean;
};

/** Non-reactive audio level snapshot, read imperatively by the UI at ~60ms. */
export type AudioLevelInfo = {
  isSpeaking: boolean;
  audioLevel: number;
};

export type CallTransitionSoundDecision = 'play' | 'defer' | 'skip';

/** Metadata embedded in the LiveKit token by the backend. */
type ParticipantMetadata = {
  login?: string;
  avatarUrl?: string;
  isBot?: boolean;
  publisherKind?: string;
  ownerIdentity?: string;
};

type LiveKitModule = typeof import('livekit-client');

const RECENTLY_DISCONNECTED_CALL_SOUND_MS = 5_000;
const MEDIA_DEVICE_TOAST_DEDUPLICATION_MS = 1_500;
let liveKitModule: LiveKitModule | null = null;
let liveKitModulePromise: Promise<LiveKitModule> | null = null;

async function loadLiveKit(): Promise<LiveKitModule> {
  liveKitModulePromise ??= import('livekit-client')
    .then((module) => {
      liveKitModule = module;
      return module;
    })
    .catch((error: unknown) => {
      liveKitModulePromise = null;
      throw error;
    });
  return liveKitModulePromise;
}

function getLoadedLiveKit(): LiveKitModule {
  if (!liveKitModule) {
    throw new Error('LiveKit must be loaded before using an active call');
  }
  return liveKitModule;
}

type VoiceCallMediaDeviceTarget = 'microphone' | 'camera' | 'screen' | 'speaker' | 'device';
type VoiceCallMediaDeviceContext = 'join' | 'enable' | 'switch' | 'event';
type MediaDeviceFailureKind =
  'permission-denied' | 'not-found' | 'in-use' | 'constraint' | 'aborted' | 'unknown';

export class VoiceCallJoinError extends Error {
  readonly userMessage: string;
  readonly cause?: unknown;

  constructor(message: string, userMessage: string, cause?: unknown) {
    super(message);
    this.name = 'VoiceCallJoinError';
    this.userMessage = userMessage;
    this.cause = cause;
  }
}

export function getVoiceCallJoinErrorMessage(err: unknown): string {
  if (err instanceof VoiceCallJoinError) return err.userMessage;

  const message = errorMessage(err);
  if (/signal connection|serverunreachable|websocket|web socket|abort handler/i.test(message)) {
    return m('voice.signaling_failed');
  }
  if (/e2ee|cryptor|encoded transform|insertable stream/i.test(message)) {
    return m('voice.encrypted_unsupported');
  }

  return m('voice.join_failed');
}

export function getVoiceCallMediaDeviceErrorMessage(
  target: VoiceCallMediaDeviceTarget,
  err: unknown,
  context: VoiceCallMediaDeviceContext = 'event'
): string {
  const failure = classifyMediaDeviceFailure(err);

  if (target === 'microphone' && context === 'join') {
    switch (failure) {
      case 'permission-denied':
        return m('voice.microphone_join_denied');
      case 'not-found':
        return m('voice.microphone_join_not_found');
      case 'in-use':
        return m('voice.microphone_join_in_use');
      default:
        return m('voice.microphone_join_failed');
    }
  }

  if (target === 'microphone') {
    switch (failure) {
      case 'permission-denied':
        return m('voice.microphone_denied');
      case 'not-found':
        return m('voice.microphone_not_found');
      case 'in-use':
        return m('voice.microphone_in_use');
      default:
        return m('voice.microphone_failed');
    }
  }

  if (target === 'camera') {
    switch (failure) {
      case 'permission-denied':
        return m('voice.camera_denied');
      case 'not-found':
        return m('voice.camera_not_found');
      case 'in-use':
        return m('voice.camera_in_use');
      default:
        return m('voice.camera_failed');
    }
  }

  if (target === 'screen') {
    if (failure === 'permission-denied' || failure === 'aborted') {
      return m('voice.screen_share_blocked');
    }
    return m('voice.screen_share_failed');
  }

  if (target === 'speaker') {
    return m('voice.speaker_switch_failed');
  }

  if (context === 'switch') {
    return m('voice.device_switch_failed');
  }

  return m('voice.media_device_failed');
}

export class VoiceCallState {
  #api: VoiceCallAPI;

  // Current call context
  roomId = $state<string | null>(null);

  // Connection state
  connecting = $state(false);
  connected = $state(false);

  // Audio state
  isMuted = $state(false);
  // True while LiveKit is applying local device enable/disable changes.
  isMicrophonePending = $state(false);

  // Video state — camera is always disabled by default
  isCameraEnabled = $state(false);
  // True while LiveKit is applying local camera enable/disable changes.
  isCameraPending = $state(false);
  isScreenShareEnabled = $state(false);
  // True while LiveKit is applying local screen-share enable/disable changes.
  isScreenSharePending = $state(false);
  isNativeScreenShareEnabled = $state(false);
  isNativeScreenSharePending = $state(false);
  nativeScreenShareSourceName = $state<string | null>(null);

  // Participants (including local)
  participants = $state<CallParticipantInfo[]>([]);

  // Remote participants locally muted by this browser session only.
  locallyMutedParticipantIds = $state<Record<string, boolean>>({});

  // Audio input devices
  audioDevices = $state<MediaDeviceInfo[]>([]);
  selectedDeviceId = $state<string | null>(null);

  // Audio output devices
  audioOutputDevices = $state<MediaDeviceInfo[]>([]);
  selectedOutputDeviceId = $state<string | null>(null);

  // Video input devices
  videoDevices = $state<MediaDeviceInfo[]>([]);
  selectedVideoDeviceId = $state<string | null>(null);

  // Internal LiveKit room instance
  private room: Room | null = null;
  private liveKitURL: string | null = null;
  private activeCallId: string | null = null;
  private pendingOwnJoinSound: {
    roomId: string;
    callId: string;
  } | null = null;
  private recentlyDisconnectedCall: {
    roomId: string;
    callId: string;
    disconnectedAt: number;
  } | null = null;
  private joinInFlight: Promise<void> | null = null;
  private joinInFlightRoomId: string | null = null;
  private leaveInFlight: Promise<void> | null = null;
  private microphoneToggleInFlight: Promise<void> | null = null;
  private cameraToggleInFlight: Promise<void> | null = null;
  private screenShareToggleInFlight: Promise<void> | null = null;
  private nativeScreenShareToggleInFlight: Promise<void> | null = null;
  private nativeScreenShareSession: NativeScreenSharePublisherSession | null = null;
  private e2eeWorker: Worker | null = null;
  private audioLevelInterval: ReturnType<typeof setInterval> | null = null;
  private suppressDisconnectToast = false;
  private explicitMediaDeviceOperationDepth = 0;
  private lastMediaDeviceToast: {
    message: string;
    shownAt: number;
  } | null = null;

  // Non-reactive audio level cache — updated at 60ms by the polling interval.
  // Deliberately NOT $state to avoid triggering Svelte reactivity at 60Hz.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity -- deliberately non-reactive, polled imperatively at 60Hz
  private audioLevelCache = new Map<string, AudioLevelInfo>();

  // Local microphone audio analysis (Web Audio API) for instant level feedback.
  private microphoneProcessor: MicrophoneProcessor | null = null;
  /** Pre-gate level for the settings meter; zero while muted. */
  microphoneLevel = $state(0);
  microphoneGateUnavailable = $state(false);

  /** The call owns this context; LiveKit uses its gain nodes for received audio. */
  private playbackContext: (AudioContext & { setSinkId?: (id: string) => Promise<void> }) | null =
    null;
  private unsavedParticipantAudio = $state<Record<string, ParticipantAudioPreferences>>({});
  /** True when the call mixer can amplify above the media-element limit. */
  audioBoostAvailable = $state(false);
  /** A user gesture must resume blocked call playback. */
  audioPlaybackBlocked = $state(false);
  /** Whether the active playback path supports a non-default speaker. */
  outputSelectionAvailable = $state(true);

  readonly preferences?: CallPreferencesState;

  readonly permissionsFor: (roomId: string) => CallPermissions;

  constructor(
    api: VoiceCallAPI,
    permissionsFor: (roomId: string) => CallPermissions = () => NO_CALL_PERMISSIONS,
    preferences?: CallPreferencesState
  ) {
    this.#api = api;
    this.permissionsFor = permissionsFor;
    this.preferences = preferences;
  }

  get canUseVoice(): boolean {
    return !!this.roomId && this.permissionsFor(this.roomId).voice;
  }
  get canUseCamera(): boolean {
    return !!this.roomId && this.permissionsFor(this.roomId).camera;
  }
  get canScreenShare(): boolean {
    return !!this.roomId && this.permissionsFor(this.roomId).screenshare;
  }

  /** Stop revoked media after pending capture operations settle. Recheck the
   * room after each await so old work cannot affect a replacement call. */
  async reconcilePermissions(): Promise<void> {
    const room = this.room;
    if (!room || !this.roomId) return;
    if (!this.permissionsFor(this.roomId).join) {
      await this.leave();
      return;
    }
    try {
      await Promise.all([
        this.microphoneToggleInFlight,
        this.cameraToggleInFlight,
        this.screenShareToggleInFlight,
        this.nativeScreenShareToggleInFlight
      ]);
      if (this.room !== room) return;
      if (!this.canUseVoice) {
        await room.localParticipant.setMicrophoneEnabled(false);
        if (this.room !== room) return;
        this.isMuted = true;
      }
      if (!this.canUseCamera) {
        await room.localParticipant.setCameraEnabled(false);
        if (this.room !== room) return;
        this.isCameraEnabled = false;
      }
      if (!this.canScreenShare) {
        await this.stopNativeScreenShare();
        if (this.room !== room) return;
        await room.localParticipant.setScreenShareEnabled(false);
        if (this.room !== room) return;
        this.isScreenShareEnabled = false;
      }
      this.updateParticipants();
    } catch {
      // If capture cannot be stopped reliably, close this media session.
      if (this.room === room) await this.leave();
    }
  }

  /**
   * Whether the user is currently in a call in the given room.
   */
  isInCall(roomId: string): boolean {
    return this.connected && this.roomId === roomId;
  }

  matchesActiveCall(roomId: string, callId: string | null): boolean {
    return (
      this.connected && this.roomId === roomId && callId !== null && this.activeCallId === callId
    );
  }

  /**
   * Whether a durable call transition event should be audible to this client.
   *
   * Remote transitions only play while the viewer is actively connected to
   * the same call. The viewer's own join can arrive before LiveKit finishes
   * connecting, so it is deferred until connect succeeds. The viewer's own
   * leave can arrive just after local cleanup, so a short recently-left
   * window keeps that event audible without leaking sounds to bystanders.
   */
  callTransitionSoundDecision(
    kind: 'join' | 'leave',
    roomId: string,
    callId: string | null,
    actorIsCurrentUser: boolean
  ): CallTransitionSoundDecision {
    if (!callId) return 'skip';

    if (this.matchesActiveCall(roomId, callId)) return 'play';

    if (!actorIsCurrentUser) return 'skip';

    if (kind === 'join' && this.roomId === roomId && this.connecting) {
      this.pendingOwnJoinSound = { roomId, callId };
      return 'defer';
    }

    if (kind === 'leave' && this.matchesRecentlyDisconnectedCall(roomId, callId)) {
      return 'play';
    }

    return 'skip';
  }

  /**
   * Whether the user is currently in any call.
   */
  get isInAnyCall(): boolean {
    return this.connected;
  }

  /**
   * Read the current audio level for a participant. Non-reactive — intended
   * to be called from a manual polling loop (setInterval), not from Svelte
   * templates or $derived expressions.
   */
  getAudioLevel(identity: string): AudioLevelInfo {
    return this.audioLevelCache.get(identity) ?? { isSpeaking: false, audioLevel: 0 };
  }

  /** Saved listener-local levels for a logical Chatto participant. */
  getParticipantAudio(identity: string): Readonly<ParticipantAudioPreferences> {
    return (
      this.preferences?.getParticipantAudio(identity) ??
      this.unsavedParticipantAudio[identity] ??
      DEFAULT_PARTICIPANT_AUDIO
    );
  }

  /** Save and apply a listener-local source level; self playback stays muted. */
  setParticipantVolume(identity: string, control: ParticipantVolumeControl, value: number): void {
    if (!this.room || identity === this.room.localParticipant.identity) return;
    if (this.preferences) this.preferences.setParticipantVolume(identity, control, value);
    else
      this.unsavedParticipantAudio = {
        ...this.unsavedParticipantAudio,
        [identity]: {
          ...this.getParticipantAudio(identity),
          [control]: normalizeParticipantVolume(value)
        }
      };
    this.applyParticipantAudioVolume(identity);
  }

  /** Restore unity gain without changing the independent local mute state. */
  resetParticipantAudio(identity: string): void {
    this.preferences?.resetParticipantAudio(identity);
    const { [identity]: _removed, ...remaining } = this.unsavedParticipantAudio;
    void _removed;
    this.unsavedParticipantAudio = remaining;
    this.applyParticipantAudioVolume(identity);
  }

  /** Resume browser-blocked audio from an explicit user gesture. */
  async resumeAudio(): Promise<void> {
    try {
      await this.room?.startAudio();
      this.audioPlaybackBlocked = this.playbackContext?.state === 'suspended';
    } catch {
      this.audioPlaybackBlocked = true;
    }
  }

  isParticipantLocallyMuted(identity: string): boolean {
    return !!this.locallyMutedParticipantIds[identity];
  }

  toggleParticipantLocalMute(identity: string): void {
    if (!this.room || identity === this.room.localParticipant.identity) return;

    const muted = !this.isParticipantLocallyMuted(identity);
    this.locallyMutedParticipantIds = {
      ...this.locallyMutedParticipantIds,
      [identity]: muted
    };
    if (!muted) {
      const { [identity]: _removed, ...remaining } = this.locallyMutedParticipantIds;
      void _removed;
      this.locallyMutedParticipantIds = remaining;
    }
    this.applyParticipantAudioVolume(identity);
    this.updateParticipants();
  }

  /**
   * Join a voice call in a room.
   */
  async join(livekitUrl: string, roomId: string): Promise<void> {
    // Already in this call
    if (this.isInCall(roomId)) return;

    if (this.joinInFlight) {
      if (this.joinInFlightRoomId === roomId) {
        return this.joinInFlight;
      }
      await this.joinInFlight;
      if (this.isInCall(roomId)) return;
    }

    const joinPromise = this.performJoin(livekitUrl, roomId);
    this.joinInFlight = joinPromise;
    this.joinInFlightRoomId = roomId;
    try {
      await joinPromise;
    } finally {
      if (this.joinInFlight === joinPromise) {
        this.joinInFlight = null;
        this.joinInFlightRoomId = null;
      }
    }
  }

  private async performJoin(livekitUrl: string, roomId: string): Promise<void> {
    if (!this.permissionsFor(roomId).join)
      throw new VoiceCallJoinError('Call join denied', m('voice.permission_denied'));
    assertLiveKitE2EESupported();

    // Leave existing call first
    if (this.connected) {
      await this.leave();
    }

    this.connecting = true;
    this.roomId = roomId;
    let joinIntentRecorded = false;
    let ownedRoom: Room | null = null;

    try {
      const { AudioPresets, ExternalE2EEKeyProvider, Room, VideoPresets } = await loadLiveKit();

      await this.#api.joinCall(roomId);
      joinIntentRecorded = true;

      // Request a credential for the active call.
      const tokenResponse = await this.#api.createCallToken(roomId);
      if (!tokenResponse) {
        throw new Error('Failed to get voice call token');
      }
      const { token, e2eeKey, callId } = tokenResponse;
      this.activeCallId = callId;

      const keyProvider = new ExternalE2EEKeyProvider();
      const { default: E2EEWorker } = await import('livekit-client/e2ee-worker?worker');
      this.e2eeWorker = new E2EEWorker();

      // Enumeration never requests capture merely to restore a saved device.
      const outputDevices = this.preferences?.speaker
        ? await Room.getLocalDevices('audiooutput', false).catch(() => [])
        : [];
      let outputDevice = availableCallDevice(this.preferences?.speaker ?? '', outputDevices);

      try {
        const { MicrophoneProcessor } = await import('$lib/audio/microphoneProcessor');
        this.microphoneProcessor = new MicrophoneProcessor(this.preferences?.microphoneThreshold);
        if (this.preferences) this.microphoneProcessor.setEffects(this.preferences.effects);
      } catch {
        this.microphoneGateUnavailable = true;
      }

      // LiveKit's Web Audio gain supports amplification; media element volume stops at 1.
      try {
        this.playbackContext = new AudioContext();
        this.audioBoostAvailable = true;
      } catch {
        this.playbackContext = null;
        this.audioBoostAvailable = false;
      }
      const playbackContext = this.playbackContext;
      this.outputSelectionAvailable = playbackContext
        ? typeof playbackContext.setSinkId === 'function'
        : typeof HTMLMediaElement !== 'undefined' && 'setSinkId' in HTMLMediaElement.prototype;
      if (playbackContext?.setSinkId && outputDevice) {
        try {
          await playbackContext.setSinkId(outputDevice === 'default' ? '' : outputDevice);
          this.selectedOutputDeviceId = outputDevice;
        } catch (err) {
          outputDevice = '';
          this.selectedOutputDeviceId = 'default';
          this.notifyMediaDeviceError(
            getVoiceCallMediaDeviceErrorMessage('speaker', err, 'switch')
          );
        }
      }
      if (!this.outputSelectionAvailable) outputDevice = '';

      // Create and connect LiveKit room
      this.room = new Room({
        webAudioMix: playbackContext ? { audioContext: playbackContext } : false,
        encryption: {
          keyProvider,
          worker: this.e2eeWorker
        },
        ...(outputDevice &&
        typeof HTMLMediaElement !== 'undefined' &&
        'setSinkId' in HTMLMediaElement.prototype
          ? { audioOutput: { deviceId: outputDevice } }
          : {}),
        audioCaptureDefaults: {
          ...(this.preferences?.microphone
            ? { deviceId: { ideal: this.preferences.microphone } }
            : {}),
          channelCount: { ideal: 1 },
          autoGainControl: false,
          echoCancellation: true,
          noiseSuppression: true
        },
        videoCaptureDefaults: {
          ...(this.preferences?.camera ? { deviceId: { ideal: this.preferences.camera } } : {}),
          resolution: VideoPresets.h720.resolution
        },
        publishDefaults: {
          audioPreset: AudioPresets.speech,
          forceStereo: false,
          dtx: true,
          red: true,
          simulcast: true
        },
        adaptiveStream: true,
        dynacast: true,
        disconnectOnPageLeave: true
      });

      this.setupRoomEventListeners();
      const room = this.room;
      ownedRoom = room;
      await keyProvider.setKey(e2eeKey);
      if (this.room !== room) return;
      await room.setE2EEEnabled(true);
      if (this.room !== room) return;
      await room.connect(livekitUrl, token);
      if (this.room !== room) {
        room.disconnect();
        return;
      }
      this.liveKitURL = livekitUrl;

      // Listen-only participants never request microphone access.
      this.isMuted = true;
      if (this.canUseVoice && !this.preferences?.joinMuted)
        try {
          await this.runExplicitMediaDeviceOperation(() =>
            room.localParticipant.setMicrophoneEnabled(true)
          );
          if (this.room === room) {
            this.isMuted = false;
            await this.setupMicrophoneProcessor();
          }
        } catch (err) {
          if (this.room === room) {
            this.isMuted = true;
            this.notifyMediaDeviceError(
              getVoiceCallMediaDeviceErrorMessage('microphone', err, 'join')
            );
          }
        }

      // Initial capture can finish after revocation or an explicit leave.
      if (this.room !== room) {
        await room.localParticipant.setMicrophoneEnabled(false);
        room.disconnect();
        return;
      }
      this.connected = true;
      this.audioPlaybackBlocked = this.playbackContext?.state === 'suspended';
      this.applyAllParticipantAudioVolumes();
      await this.reconcilePermissions();
      if (this.room !== room) return;
      this.updateParticipants();
      await this.refreshDevices();
      if (this.room !== room) return;
      if (this.consumePendingOwnJoinSound()) {
        void playCallSound('join');
      }
    } catch (err) {
      // A departed join must not clean up or mutate a replacement call.
      if (ownedRoom && this.room !== ownedRoom) {
        ownedRoom.disconnect();
        return;
      }
      console.error('Failed to join voice call:', summarizeJoinError(err));
      if (joinIntentRecorded) {
        await this.recordLeaveIntent(roomId);
      }
      this.cleanup();
      throw err;
    } finally {
      if (!ownedRoom || this.room === ownedRoom) this.connecting = false;
    }
  }

  /**
   * Leave the current voice call.
   */
  async leave(): Promise<void> {
    if (this.leaveInFlight) return this.leaveInFlight;
    if (!this.room) return;

    const leavePromise = this.performLeave();
    this.leaveInFlight = leavePromise;
    try {
      await leavePromise;
    } finally {
      if (this.leaveInFlight === leavePromise) {
        this.leaveInFlight = null;
      }
    }
  }

  private async performLeave(): Promise<void> {
    const roomId = this.roomId;
    if (roomId) {
      await this.recordLeaveIntent(roomId);
    }

    this.room?.disconnect();
    this.cleanup();
  }

  /**
   * Apply a backend-authored participant leave. Used for reconciliation and
   * moderation paths where the server has already committed the leave fact.
   */
  handleParticipantLeftEvent(
    roomId: string,
    callId: string | null,
    actorId: string | null,
    currentUserId: string | null
  ): void {
    if (!actorId || !currentUserId || actorId !== currentUserId) return;
    this.disconnectFromServerEvent(roomId, callId);
  }

  /**
   * Apply a backend-authored call end. Does not record another leave intent.
   */
  handleCallEndedEvent(roomId: string, callId: string | null): void {
    this.disconnectFromServerEvent(roomId, callId);
  }

  /** Disconnect local media immediately when this viewer loses room access. */
  handleRoomAccessRevoked(roomId: string): void {
    if (this.roomId !== roomId) return;
    const room = this.room;
    if (room) {
      this.suppressDisconnectToast = true;
      room.disconnect();
    }
    this.cleanup();
    this.suppressDisconnectToast = false;
  }

  private disconnectFromServerEvent(roomId: string, callId: string | null): void {
    if (this.roomId !== roomId) return;
    if (!callId || this.activeCallId !== callId) return;

    const room = this.room;
    if (room) {
      this.suppressDisconnectToast = true;
      room.disconnect();
    }
    this.cleanup();
    this.suppressDisconnectToast = false;
  }

  private async recordLeaveIntent(roomId: string): Promise<void> {
    try {
      await this.#api.leaveCall(roomId);
    } catch {
      // LiveKit disconnect/cleanup should still proceed if the intent write fails.
    }
  }

  /**
   * Toggle microphone mute.
   */
  async toggleMute(): Promise<void> {
    if (this.isMuted && !this.canUseVoice) return;
    if (this.microphoneToggleInFlight) return this.microphoneToggleInFlight;

    const room = this.room;
    if (!room) return;

    const togglePromise = this.performToggleMute(room);
    this.microphoneToggleInFlight = togglePromise;
    this.isMicrophonePending = true;
    try {
      await togglePromise;
    } finally {
      if (this.microphoneToggleInFlight === togglePromise) {
        this.microphoneToggleInFlight = null;
        this.isMicrophonePending = false;
      }
    }
  }

  private async performToggleMute(room: Room): Promise<void> {
    const newMuted = !this.isMuted;
    try {
      await this.runExplicitMediaDeviceOperation(() =>
        room.localParticipant.setMicrophoneEnabled(!newMuted)
      );
      if (this.room !== room) return;
    } catch (err) {
      if (this.room === room && !newMuted) {
        this.notifyMediaDeviceError(
          getVoiceCallMediaDeviceErrorMessage('microphone', err, 'enable')
        );
      }
      return;
    }

    this.isMuted = newMuted;

    if (!newMuted) {
      await this.setupMicrophoneProcessor();
    }

    this.updateParticipants();
  }

  /**
   * Toggle camera on/off. Camera is always off by default.
   */
  async toggleCamera(): Promise<void> {
    if (!this.isCameraEnabled && !this.canUseCamera) return;
    if (this.cameraToggleInFlight) return this.cameraToggleInFlight;

    const room = this.room;
    if (!room) return;

    const togglePromise = this.performToggleCamera(room);
    this.cameraToggleInFlight = togglePromise;
    this.isCameraPending = true;
    try {
      await togglePromise;
    } finally {
      if (this.cameraToggleInFlight === togglePromise) {
        this.cameraToggleInFlight = null;
        this.isCameraPending = false;
      }
    }
  }

  private async performToggleCamera(room: Room): Promise<void> {
    const newEnabled = !this.isCameraEnabled;
    try {
      await this.runExplicitMediaDeviceOperation(() =>
        room.localParticipant.setCameraEnabled(newEnabled)
      );
      if (this.room !== room) return;

      this.isCameraEnabled = newEnabled;
      if (newEnabled) {
        await this.refreshDevices({ requestVideoPermissions: true });
      }
    } catch (err) {
      // Permission denied or no camera available — keep current state
      if (this.room !== room) return;
      if (newEnabled) {
        this.notifyMediaDeviceError(getVoiceCallMediaDeviceErrorMessage('camera', err, 'enable'));
      }
      this.isCameraEnabled = false;
    }
    this.updateParticipants();
  }

  /**
   * Toggle video-only screen/window/tab sharing.
   */
  async toggleScreenShare(): Promise<void> {
    if (!this.isScreenShareEnabled && !this.canScreenShare) return;
    if (this.nativeScreenShareToggleInFlight) {
      await this.nativeScreenShareToggleInFlight;
      return;
    }
    if (this.nativeScreenShareSession) {
      await this.stopNativeScreenShare();
      return;
    }
    if (this.screenShareToggleInFlight) return this.screenShareToggleInFlight;

    const room = this.room;
    if (!room) return;

    const togglePromise = this.performToggleScreenShare(room);
    this.screenShareToggleInFlight = togglePromise;
    this.isScreenSharePending = true;
    try {
      await togglePromise;
    } finally {
      if (this.screenShareToggleInFlight === togglePromise) {
        this.screenShareToggleInFlight = null;
        this.isScreenSharePending = false;
      }
    }
  }

  /** Publish a host-provided native capture as this participant's screen share. */
  async startNativeScreenShare(sourceId: string, sourceName: string): Promise<void> {
    const room = this.room;
    if (!room || !this.canScreenShare) return;
    if (this.nativeScreenShareToggleInFlight) return this.nativeScreenShareToggleInFlight;
    if (this.screenShareToggleInFlight) await this.screenShareToggleInFlight;
    if (this.room !== room || !this.canScreenShare) return;

    const startPromise = this.performStartNativeScreenShare(room, sourceId, sourceName);
    this.nativeScreenShareToggleInFlight = startPromise;
    this.isNativeScreenSharePending = true;
    try {
      await startPromise;
    } finally {
      if (this.nativeScreenShareToggleInFlight === startPromise) {
        this.nativeScreenShareToggleInFlight = null;
        this.isNativeScreenSharePending = false;
      }
    }
  }

  private async performStartNativeScreenShare(
    room: Room,
    sourceId: string,
    sourceName: string
  ): Promise<void> {
    if (this.nativeScreenShareSession) await this.performStopNativeScreenShare(room);
    const replacingBrowserScreenShare = this.isScreenShareEnabled;

    const livekitUrl = this.liveKitURL;
    const roomId = this.roomId;
    if (!livekitUrl || !roomId) return;
    const { NativeScreenSharePublisherSession } =
      await import('$lib/desktop/nativeScreenSharePublisher');
    if (this.room !== room || !this.canScreenShare) return;
    const credential = await this.#api.createGameSharePublisherToken(roomId);
    if (!credential || credential.callId !== this.activeCallId) {
      throw new Error('The server could not create a native screen-share publisher credential.');
    }
    const session = await NativeScreenSharePublisherSession.start({
      sourceId,
      livekitUrl,
      token: credential.token,
      e2eeKey: credential.e2eeKey
    });
    let provisionalSessionEnded = false;
    let provisionalSessionError: Error | undefined;
    session.onEnded = (error) => {
      provisionalSessionEnded = true;
      provisionalSessionError = error;
    };
    if (this.room !== room) {
      await session.stop().catch(() => undefined);
      return;
    }

    if (replacingBrowserScreenShare) {
      let replacementError: unknown;
      try {
        await room.localParticipant.setScreenShareEnabled(false);
      } catch (error) {
        replacementError = error;
      }
      if (this.room !== room) {
        await session.stop().catch(() => undefined);
        return;
      }

      const browserScreenShareStillPublished = hasParticipantScreenSharePublication(
        room.localParticipant
      );
      this.isScreenShareEnabled = browserScreenShareStillPublished;
      if (browserScreenShareStillPublished) {
        await session.stop().catch(() => undefined);
        throw (
          replacementError ?? new Error('The existing browser screen share could not be replaced.')
        );
      }

      // LiveKit removes local tracks before its final negotiation completes, so
      // a late rejection can still mean that the browser share was replaced.
      // Keep the working native publisher in that case instead of losing both.
      if (provisionalSessionEnded) {
        this.isNativeScreenShareEnabled = false;
        this.nativeScreenShareSourceName = null;
        this.updateParticipants();
        throw (
          provisionalSessionError ??
          new Error('The native screen-share publisher stopped during handoff.')
        );
      }
    }

    if (provisionalSessionEnded) {
      throw (
        provisionalSessionError ??
        new Error('The native screen-share publisher stopped before it became active.')
      );
    }
    this.nativeScreenShareSession = session;
    session.onEnded = (error) => void this.handleNativeScreenShareEnded(session, error);
    if (this.room !== room || this.nativeScreenShareSession !== session) {
      session.stop();
      return;
    }

    this.isNativeScreenShareEnabled = true;
    this.nativeScreenShareSourceName = sourceName;
    this.isScreenShareEnabled = true;
    this.updateParticipants();
  }

  /** Stop the active native screen-share companion publisher. */
  async stopNativeScreenShare(): Promise<void> {
    if (this.nativeScreenShareToggleInFlight) return this.nativeScreenShareToggleInFlight;
    const room = this.room;
    if (!room || !this.nativeScreenShareSession) return;

    const stopPromise = this.performStopNativeScreenShare(room);
    this.nativeScreenShareToggleInFlight = stopPromise;
    this.isNativeScreenSharePending = true;
    try {
      await stopPromise;
    } finally {
      if (this.nativeScreenShareToggleInFlight === stopPromise) {
        this.nativeScreenShareToggleInFlight = null;
        this.isNativeScreenSharePending = false;
      }
    }
  }

  private async performStopNativeScreenShare(room: Room): Promise<void> {
    const session = this.nativeScreenShareSession;
    if (!session) return;
    session.onEnded = null;
    try {
      await session.stop();
    } finally {
      if (this.nativeScreenShareSession === session) this.nativeScreenShareSession = null;
      if (this.room === room) {
        this.isNativeScreenShareEnabled = false;
        this.nativeScreenShareSourceName = null;
        this.isScreenShareEnabled = false;
        this.updateParticipants();
      }
    }
  }

  private async handleNativeScreenShareEnded(
    session: NativeScreenSharePublisherSession,
    error?: Error
  ): Promise<void> {
    if (this.nativeScreenShareSession !== session) return;
    this.nativeScreenShareSession = null;
    this.isNativeScreenShareEnabled = false;
    this.nativeScreenShareSourceName = null;
    this.isScreenShareEnabled = false;
    this.updateParticipants();
    if (error) toast.error(m('voice.screen_share_failed'));
  }

  private async performToggleScreenShare(room: Room): Promise<void> {
    const newEnabled = !this.isScreenShareEnabled;
    const { AudioPresets } = getLoadedLiveKit();
    try {
      await this.runExplicitMediaDeviceOperation(() =>
        room.localParticipant.setScreenShareEnabled(
          newEnabled,
          newEnabled
            ? {
                audio: true,
                // Tab audio is useful shared media; whole-system audio can feed
                // remote call playback back into the room.
                systemAudio: 'exclude'
              }
            : undefined,
          newEnabled
            ? {
                audioPreset: AudioPresets.musicStereo,
                forceStereo: true,
                dtx: false,
                red: false
              }
            : undefined
        )
      );
      if (this.room !== room) return;

      this.isScreenShareEnabled = newEnabled;
    } catch (err) {
      if (this.room !== room) return;
      // The browser uses the same error for dismissing its picker and denying
      // capture permission. Neither needs a toast; other capture failures do.
      const pickerDismissed = ['NotAllowedError', 'PermissionDeniedError'].includes(errorName(err));
      if (newEnabled && !pickerDismissed) {
        this.notifyMediaDeviceError(getVoiceCallMediaDeviceErrorMessage('screen', err, 'enable'));
      }
      this.isScreenShareEnabled = newEnabled ? false : this.isScreenShareEnabled;
    }
    this.updateParticipants();
  }

  /**
   * Refresh available audio and video devices.
   */
  async refreshDevices(options: { requestVideoPermissions?: boolean } = {}): Promise<void> {
    const room = this.room;
    try {
      const { Room } = await loadLiveKit();
      const requestVideoPermissions =
        this.canUseCamera && (options.requestVideoPermissions ?? this.isCameraEnabled);
      const [inputDevices, outputDevices, videoInputDevices] = await Promise.all([
        Room.getLocalDevices('audioinput', this.canUseVoice && !this.isMuted),
        Room.getLocalDevices('audiooutput', this.canUseVoice && !this.isMuted),
        Room.getLocalDevices('videoinput', requestVideoPermissions)
      ]);

      if (this.room !== room) return;
      this.audioDevices = inputDevices;
      this.audioOutputDevices = outputDevices;
      this.videoDevices = videoInputDevices;

      // Set default selections if not already set
      if (!this.selectedDeviceId && inputDevices.length > 0) {
        this.selectedDeviceId =
          availableCallDevice(this.preferences?.microphone ?? '', inputDevices) ||
          inputDevices[0].deviceId;
      }
      if (!this.selectedOutputDeviceId && outputDevices.length > 0) {
        this.selectedOutputDeviceId =
          availableCallDevice(this.preferences?.speaker ?? '', outputDevices) ||
          outputDevices[0].deviceId;
      }
      if (!this.selectedVideoDeviceId && videoInputDevices.length > 0) {
        this.selectedVideoDeviceId =
          availableCallDevice(this.preferences?.camera ?? '', videoInputDevices) ||
          videoInputDevices[0].deviceId;
      }
    } catch {
      this.audioDevices = [];
      this.audioOutputDevices = [];
      this.videoDevices = [];
    }
  }

  /**
   * Switch to a different audio input device.
   */
  async setAudioDevice(deviceId: string): Promise<void> {
    const room = this.room;
    if (!room) return;

    try {
      const changed = await this.runExplicitMediaDeviceOperation(() =>
        room.switchActiveDevice('audioinput', deviceId)
      );
      if (changed === false) throw new Error('Device switch failed');
      if (this.room !== room) return;
      this.selectedDeviceId = deviceId;
      this.preferences?.setDevice('audioinput', deviceId);
    } catch (err) {
      this.notifyMediaDeviceError(getVoiceCallMediaDeviceErrorMessage('microphone', err, 'switch'));
      return;
    }

    // Attach processing if the device change created a new local track.
    if (!this.isMuted) {
      await this.setupMicrophoneProcessor();
    }
  }

  /**
   * Switch to a different audio output device.
   */
  async setAudioOutputDevice(deviceId: string): Promise<void> {
    const room = this.room;
    if (!room) return;

    const context = this.playbackContext;
    const previousOutput = this.selectedOutputDeviceId;
    try {
      // Await the context operation ourselves: the SDK does not await setSinkId.
      const changed = await this.runExplicitMediaDeviceOperation(async () => {
        if (this.playbackContext) {
          if (!this.playbackContext.setSinkId) throw new Error('Output selection unavailable');
          await this.playbackContext.setSinkId(deviceId === 'default' ? '' : deviceId);
        }
        return room.switchActiveDevice('audiooutput', deviceId);
      });
      if (changed === false) throw new Error('Device switch failed');
      if (this.room !== room) return;
      this.selectedOutputDeviceId = deviceId;
      this.preferences?.setDevice('audiooutput', deviceId);
    } catch (err) {
      if (this.room === room && context?.setSinkId) {
        await context
          .setSinkId(previousOutput === 'default' ? '' : (previousOutput ?? ''))
          .catch(() => undefined);
      }
      this.notifyMediaDeviceError(getVoiceCallMediaDeviceErrorMessage('speaker', err, 'switch'));
    }
  }

  /**
   * Switch to a different video input device.
   */
  async setVideoDevice(deviceId: string): Promise<void> {
    const room = this.room;
    if (!room) return;

    try {
      const changed = await this.runExplicitMediaDeviceOperation(() =>
        room.switchActiveDevice('videoinput', deviceId)
      );
      if (changed === false) throw new Error('Device switch failed');
      if (this.room !== room) return;
      this.selectedVideoDeviceId = deviceId;
      this.preferences?.setDevice('videoinput', deviceId);
    } catch (err) {
      this.notifyMediaDeviceError(getVoiceCallMediaDeviceErrorMessage('camera', err, 'switch'));
    }
  }

  private setupRoomEventListeners(): void {
    if (!this.room) return;
    const { RoomEvent, Track } = getLoadedLiveKit();

    this.room.on(RoomEvent.AudioPlaybackStatusChanged, (playing: boolean) => {
      this.audioPlaybackBlocked = !playing;
    });
    this.room.on(RoomEvent.Reconnected, () => {
      this.applyAllParticipantAudioVolumes();
    });
    this.room.on(RoomEvent.ParticipantConnected, () => {
      this.applyAllParticipantAudioVolumes();
      this.updateParticipants();
    });

    this.room.on(RoomEvent.ParticipantDisconnected, () => {
      this.updateParticipants();
    });

    this.room.on(RoomEvent.TrackMuted, () => {
      this.updateParticipants();
    });

    this.room.on(RoomEvent.TrackUnmuted, () => {
      this.updateParticipants();
    });

    this.room.on(RoomEvent.Disconnected, () => {
      // Only show toast if we were in an active call (not a failed join attempt)
      if (this.connected && !this.suppressDisconnectToast) {
        toast.error(m('voice.disconnected'));
      }
      this.cleanup();
    });

    this.room.on(RoomEvent.MediaDevicesChanged, () => {
      this.refreshDevices();
    });

    this.room.on(RoomEvent.MediaDevicesError, (err: Error) => {
      if (this.explicitMediaDeviceOperationDepth > 0) return;
      this.notifyMediaDeviceError(getVoiceCallMediaDeviceErrorMessage('device', err, 'event'));
    });

    this.room.on(RoomEvent.ConnectionQualityChanged, () => {
      this.updateParticipants();
    });

    // Attach remote audio tracks so we actually hear other participants.
    // LiveKit delivers audio data over WebRTC, but the browser won't play it
    // until the track is attached to an <audio> element.
    // Video tracks are NOT attached here — VideoThumbnail manages its own lifecycle.
    this.room.on(
      RoomEvent.TrackSubscribed,
      (
        track: RemoteTrack,
        _publication: RemoteTrackPublication,
        participant: RemoteParticipant
      ) => {
        if (track.kind === Track.Kind.Audio) {
          if (!this.isLocalCompanionPublisher(participant)) track.attach();
          this.applyAllParticipantAudioVolumes();
        }
        this.updateParticipants();
      }
    );

    this.room.on(
      RoomEvent.TrackUnsubscribed,
      (track: RemoteTrack, _publication: RemoteTrackPublication) => {
        track.detach();
        this.updateParticipants();
      }
    );

    // Track published/unpublished — catches camera enable/disable by remote participants
    this.room.on(RoomEvent.TrackPublished, () => {
      this.updateParticipants();
    });

    this.room.on(RoomEvent.TrackUnpublished, () => {
      this.updateParticipants();
    });

    this.room.on(RoomEvent.LocalTrackPublished, () => {
      this.updateParticipants();
    });

    this.room.on(RoomEvent.LocalTrackUnpublished, () => {
      this.updateParticipants();
    });

    // Keep audio level snapshots fresh for call UI consumers without pushing
    // 60Hz updates through Svelte's reactive graph.
    this.audioLevelInterval = setInterval(() => {
      this.updateAudioLevels();
    }, 60);
  }

  private updateParticipants(): void {
    if (!this.room) {
      this.participants = [];
      return;
    }

    const companionPublishers = Array.from(this.room.remoteParticipants.values()).filter(
      isCompanionPublisher
    );
    const allParticipants: Participant[] = [
      this.room.localParticipant,
      ...Array.from(this.room.remoteParticipants.values()).filter(
        (participant) => !isCompanionPublisher(participant)
      )
    ];
    this.isCameraEnabled = isParticipantCameraEnabled(this.room.localParticipant);
    this.isScreenShareEnabled =
      isParticipantScreenShareEnabled(this.room.localParticipant) ||
      this.isNativeScreenShareEnabled;
    this.applyAllParticipantAudioVolumes();

    this.participants = allParticipants.map((p) => {
      const md = parseParticipantMetadata(p.metadata);
      const isLocal = p === this.room!.localParticipant;
      const companion = companionPublishers.find(
        (candidate) => parseParticipantMetadata(candidate.metadata).ownerIdentity === p.identity
      );
      const screenShareTrack =
        getParticipantScreenShareTrack(p) ??
        (companion ? getParticipantScreenShareTrack(companion) : null);
      return {
        identity: p.identity,
        name: p.name ?? p.identity,
        login: md.login ?? p.identity,
        avatarUrl: md.avatarUrl ?? null,
        isBot: md.isBot ?? false,
        isMuted: isParticipantMuted(p),
        isLocal,
        connectionQuality: p.connectionQuality as CallParticipantInfo['connectionQuality'],
        isCameraEnabled: isParticipantCameraEnabled(p),
        videoTrack: getParticipantCameraTrack(p),
        isScreenShareEnabled:
          screenShareTrack !== null || (isLocal && this.isNativeScreenShareEnabled),
        screenShareTrack,
        isLocallyMuted: !isLocal && this.isParticipantLocallyMuted(p.identity)
      };
    });
  }

  private applyAllParticipantAudioVolumes(): void {
    if (!this.room) return;
    for (const participant of this.room.remoteParticipants.values()) {
      this.applyRemoteParticipantAudioVolume(participant);
    }
  }

  private applyParticipantAudioVolume(identity: string): void {
    if (!this.room) return;
    for (const participant of this.room.remoteParticipants.values()) {
      const ownerIdentity = parseParticipantMetadata(participant.metadata).ownerIdentity;
      if (participant.identity === identity || ownerIdentity === identity) {
        this.applyRemoteParticipantAudioVolume(participant);
      }
    }
  }

  private applyRemoteParticipantAudioVolume(participant: RemoteParticipant): void {
    const { Track } = getLoadedLiveKit();
    const ownerIdentity = parseParticipantMetadata(participant.metadata).ownerIdentity;
    const logicalIdentity = ownerIdentity || participant.identity;
    const muted =
      logicalIdentity === this.room?.localParticipant.identity ||
      this.isParticipantLocallyMuted(logicalIdentity);
    const settings = this.getParticipantAudio(logicalIdentity);
    const maximum = this.audioBoostAvailable ? 2 : 1;
    participant.setVolume(
      muted ? 0 : Math.min(maximum, settings.voiceVolume / 100),
      Track.Source.Microphone
    );
    participant.setVolume(
      muted ? 0 : Math.min(maximum, settings.streamVolume / 100),
      Track.Source.ScreenShareAudio
    );
  }

  private isLocalCompanionPublisher(participant: RemoteParticipant): boolean {
    const metadata = parseParticipantMetadata(participant.metadata);
    return (
      isCompanionPublisher(participant) &&
      metadata.ownerIdentity === this.room?.localParticipant.identity
    );
  }

  /**
   * Update the non-reactive audio level cache. Called at ~60ms.
   * Participant levels stay in a plain Map; only the settings meter and
   * optional processor availability enter Svelte's reactive graph.
   */
  private updateAudioLevels(): void {
    if (!this.room) return;

    this.microphoneProcessor?.setThreshold(this.preferences?.microphoneThreshold ?? -60);
    if (this.preferences) this.microphoneProcessor?.setEffects(this.preferences.effects);
    if (this.microphoneProcessor)
      this.microphoneGateUnavailable = this.microphoneProcessor.unavailable;
    const inputLevel = this.isMuted ? 0 : (this.microphoneProcessor?.level ?? 0);
    const localAudioLevel = Math.min(inputLevel * 2, 1);
    this.microphoneLevel = microphoneMeter(inputLevel);

    const allParticipants: Participant[] = [
      this.room.localParticipant,
      ...Array.from(this.room.remoteParticipants.values()).filter(
        (participant) => !isCompanionPublisher(participant)
      )
    ];

    for (const p of allParticipants) {
      const isLocal = p === this.room!.localParticipant;
      this.audioLevelCache.set(p.identity, {
        isSpeaking: p.isSpeaking,
        audioLevel: isLocal ? localAudioLevel : p.audioLevel
      });
    }
  }

  /**
   * Attach optional processing after LiveKit assigns the audio context.
   * The processor also owns the input meter and its analyser fallback.
   */
  private async setupMicrophoneProcessor(): Promise<void> {
    const room = this.room;
    const processor = this.microphoneProcessor;
    if (!room) return;

    const { Track } = getLoadedLiveKit();
    const micPub = room.localParticipant.getTrackPublication(Track.Source.Microphone);
    const audioTrack = micPub?.audioTrack;
    if (audioTrack && processor) {
      try {
        if (audioTrack.getProcessor() !== processor) await audioTrack.setProcessor(processor);
      } catch {
        // Optional processing must not fail microphone enable or a device switch.
        processor.unavailable = true;
      }
      if (this.room !== room || this.isMuted) return;
      this.microphoneGateUnavailable = processor.unavailable;
    }
  }

  private cleanup(): void {
    this.microphoneProcessor?.dispose();
    this.microphoneProcessor = null;
    this.microphoneLevel = 0;
    this.microphoneGateUnavailable = false;
    const disconnectedRoomId = this.roomId;
    const disconnectedCallId = this.activeCallId;
    const wasConnected = this.connected;

    if (this.audioLevelInterval) {
      clearInterval(this.audioLevelInterval);
      this.audioLevelInterval = null;
    }

    if (this.room) {
      // Detach all remote audio tracks to clean up <audio> elements
      for (const p of this.room.remoteParticipants.values()) {
        for (const pub of p.trackPublications.values()) {
          pub.track?.detach();
        }
      }
      this.room.removeAllListeners();
      this.room = null;
    }
    if (this.playbackContext) void this.playbackContext.close().catch(() => undefined);
    this.playbackContext = null;
    this.audioBoostAvailable = false;
    this.audioPlaybackBlocked = false;
    this.outputSelectionAvailable = true;
    this.e2eeWorker?.terminate();
    this.e2eeWorker = null;
    if (wasConnected && disconnectedRoomId && disconnectedCallId) {
      this.recentlyDisconnectedCall = {
        roomId: disconnectedRoomId,
        callId: disconnectedCallId,
        disconnectedAt: Date.now()
      };
    }
    this.activeCallId = null;
    this.liveKitURL = null;
    this.pendingOwnJoinSound = null;
    this.joinInFlight = null;
    this.joinInFlightRoomId = null;
    this.microphoneToggleInFlight = null;
    this.cameraToggleInFlight = null;
    this.screenShareToggleInFlight = null;
    this.nativeScreenShareToggleInFlight = null;
    if (this.nativeScreenShareSession)
      void this.nativeScreenShareSession.stop().catch(() => undefined);
    this.nativeScreenShareSession = null;
    this.suppressDisconnectToast = false;
    this.connected = false;
    this.connecting = false;
    this.roomId = null;
    this.isMuted = false;
    this.isMicrophonePending = false;
    this.isCameraEnabled = false;
    this.isCameraPending = false;
    this.isScreenShareEnabled = false;
    this.isScreenSharePending = false;
    this.isNativeScreenShareEnabled = false;
    this.isNativeScreenSharePending = false;
    this.nativeScreenShareSourceName = null;
    this.participants = [];
    this.locallyMutedParticipantIds = {};
    this.audioDevices = [];
    this.selectedDeviceId = null;
    this.audioOutputDevices = [];
    this.selectedOutputDeviceId = null;
    this.videoDevices = [];
    this.selectedVideoDeviceId = null;
    this.audioLevelCache.clear();
    this.explicitMediaDeviceOperationDepth = 0;
    this.lastMediaDeviceToast = null;
  }

  private async runExplicitMediaDeviceOperation<T>(operation: () => Promise<T>): Promise<T> {
    this.explicitMediaDeviceOperationDepth += 1;
    try {
      return await operation();
    } finally {
      this.explicitMediaDeviceOperationDepth = Math.max(
        0,
        this.explicitMediaDeviceOperationDepth - 1
      );
    }
  }

  private notifyMediaDeviceError(message: string): void {
    const now = Date.now();
    if (
      this.lastMediaDeviceToast &&
      this.lastMediaDeviceToast.message === message &&
      now - this.lastMediaDeviceToast.shownAt < MEDIA_DEVICE_TOAST_DEDUPLICATION_MS
    ) {
      return;
    }

    this.lastMediaDeviceToast = { message, shownAt: now };
    toast.error(message);
  }

  private consumePendingOwnJoinSound(): boolean {
    const pending = this.pendingOwnJoinSound;
    if (!pending) return false;
    this.pendingOwnJoinSound = null;
    return this.matchesActiveCall(pending.roomId, pending.callId);
  }

  private matchesRecentlyDisconnectedCall(roomId: string, callId: string): boolean {
    const recentlyDisconnectedCall = this.recentlyDisconnectedCall;
    if (!recentlyDisconnectedCall) return false;
    if (
      Date.now() - recentlyDisconnectedCall.disconnectedAt >
      RECENTLY_DISCONNECTED_CALL_SOUND_MS
    ) {
      this.recentlyDisconnectedCall = null;
      return false;
    }
    return recentlyDisconnectedCall.roomId === roomId && recentlyDisconnectedCall.callId === callId;
  }
}

/** Parse the JSON metadata string from a LiveKit participant. */
function parseParticipantMetadata(metadata: string | undefined): ParticipantMetadata {
  if (!metadata) return {};
  try {
    return JSON.parse(metadata) as ParticipantMetadata;
  } catch {
    return {};
  }
}

function isCompanionPublisher(participant: Participant): boolean {
  const metadata = parseParticipantMetadata(participant.metadata);
  return metadata.publisherKind === 'game_share' && !!metadata.ownerIdentity;
}

function isParticipantMuted(participant: Participant): boolean {
  const { Track } = getLoadedLiveKit();
  for (const pub of participant.getTrackPublications()) {
    if (pub.track?.source === Track.Source.Microphone) {
      return pub.isMuted;
    }
  }
  // No audio track = effectively muted
  return true;
}

function isParticipantCameraEnabled(participant: Participant): boolean {
  const { Track } = getLoadedLiveKit();
  for (const pub of participant.getTrackPublications()) {
    if (pub.track?.source === Track.Source.Camera) {
      return !pub.isMuted;
    }
  }
  return false;
}

function getParticipantCameraTrack(participant: Participant): Track | null {
  const { Track } = getLoadedLiveKit();
  for (const pub of participant.getTrackPublications()) {
    if (pub.track?.source === Track.Source.Camera && !pub.isMuted) {
      return pub.track;
    }
  }
  return null;
}

function isParticipantScreenShareEnabled(participant: Participant): boolean {
  const { Track } = getLoadedLiveKit();
  for (const pub of participant.getTrackPublications()) {
    if (pub.track?.source === Track.Source.ScreenShare) {
      return !pub.isMuted;
    }
  }
  return false;
}

function hasParticipantScreenSharePublication(participant: Participant): boolean {
  const { Track } = getLoadedLiveKit();
  return participant
    .getTrackPublications()
    .some((publication) => publication.track?.source === Track.Source.ScreenShare);
}

function getParticipantScreenShareTrack(participant: Participant): Track | null {
  const { Track } = getLoadedLiveKit();
  for (const pub of participant.getTrackPublications()) {
    if (pub.track?.source === Track.Source.ScreenShare && !pub.isMuted) {
      return pub.track;
    }
  }
  return null;
}

function assertLiveKitE2EESupported(): void {
  const globals = globalThis as typeof globalThis & Record<string, unknown>;
  const senderCtor = globals.RTCRtpSender as { prototype?: object } | undefined;
  const senderProto = senderCtor?.prototype as Record<string, unknown> | undefined;
  const hasEncodedTransform =
    typeof globals.RTCRtpScriptTransform === 'function' ||
    typeof senderProto?.createEncodedStreams === 'function';

  if (
    typeof globals.Worker !== 'function' ||
    typeof globals.TransformStream !== 'function' ||
    typeof globals.ReadableStream !== 'function' ||
    typeof globals.WritableStream !== 'function' ||
    !globals.crypto ||
    typeof globals.crypto !== 'object' ||
    !('subtle' in globals.crypto) ||
    !hasEncodedTransform
  ) {
    throw new VoiceCallJoinError(
      'LiveKit E2EE is not supported by this browser',
      m('voice.encrypted_unsupported')
    );
  }
}

function summarizeJoinError(err: unknown): string {
  return redactSensitiveUrlParts(errorMessage(err));
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  return String(err);
}

function errorName(err: unknown): string {
  if (typeof DOMException !== 'undefined' && err instanceof DOMException) return err.name;
  if (err instanceof Error) return err.name;
  return '';
}

function classifyMediaDeviceFailure(err: unknown): MediaDeviceFailureKind {
  const name = errorName(err).toLowerCase();
  const message = errorMessage(err).toLowerCase();
  const signal = `${name} ${message}`;

  if (
    signal.includes('notallowed') ||
    signal.includes('permissiondenied') ||
    signal.includes('permission denied') ||
    signal.includes('securityerror')
  ) {
    return 'permission-denied';
  }

  if (
    signal.includes('notfound') ||
    signal.includes('devicesnotfound') ||
    signal.includes('device not found') ||
    signal.includes('no device')
  ) {
    return 'not-found';
  }

  if (
    signal.includes('notreadable') ||
    signal.includes('trackstarterror') ||
    signal.includes('deviceinuse') ||
    signal.includes('device in use') ||
    signal.includes('already in use')
  ) {
    return 'in-use';
  }

  if (signal.includes('overconstrained') || signal.includes('constraint')) {
    return 'constraint';
  }

  if (signal.includes('abort')) {
    return 'aborted';
  }

  return 'unknown';
}

function redactSensitiveUrlParts(message: string): string {
  return message
    .replace(/access_token=([^&\s]+)/gi, 'access_token=<redacted>')
    .replace(/join_request=([^&\s]+)/gi, 'join_request=<redacted>')
    .replace(/\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b/g, '<jwt-redacted>');
}
