/**
 * Voice call state — manages LiveKit connection for voice/video calls.
 *
 * Per-instance class that wraps livekit-client's Room instance.
 * Handles joining/leaving calls, mute toggle, camera toggle,
 * and audio/video device selection.
 */

import {
  Room,
  RoomEvent,
  Track,
  AudioPresets,
  VideoPresets,
  ExternalE2EEKeyProvider,
  type Participant,
  type RemoteTrack,
  type RemoteTrackPublication
} from 'livekit-client';
import { graphql } from '$lib/gql';
import type { Client } from '@urql/svelte';
import { toast } from '$lib/ui/toast';

export type CallParticipantInfo = {
  identity: string;
  name: string;
  login: string;
  avatarUrl: string | null;
  isMuted: boolean;
  isLocal: boolean;
  connectionQuality: 'excellent' | 'good' | 'poor' | 'lost' | 'unknown';
  isCameraEnabled: boolean;
  videoTrack: Track | null;
};

/** Non-reactive audio level snapshot, read imperatively by the UI at ~60ms. */
export type AudioLevelInfo = {
  isSpeaking: boolean;
  audioLevel: number;
};

/** Metadata embedded in the LiveKit token by the backend. */
type ParticipantMetadata = {
  login?: string;
  avatarUrl?: string;
};

const VoiceCallTokenQuery = graphql(`
  query GetVoiceCallToken($roomId: ID!) {
    room(roomId: $roomId) {
      voiceCallToken {
        token
        e2eeKey
      }
    }
  }
`);

const JoinVoiceCallMutation = graphql(`
  mutation JoinVoiceCall($roomId: ID!) {
    joinVoiceCall(input: { roomId: $roomId })
  }
`);

const LeaveVoiceCallMutation = graphql(`
  mutation LeaveVoiceCall($roomId: ID!) {
    leaveVoiceCall(input: { roomId: $roomId })
  }
`);

export class VoiceCallState {
  #client: Client;

  // Current call context
  roomId = $state<string | null>(null);

  // Connection state
  connecting = $state(false);
  connected = $state(false);

  // Audio state
  isMuted = $state(false);

  // Video state — camera is always disabled by default
  isCameraEnabled = $state(false);

  // Participants (including local)
  participants = $state<CallParticipantInfo[]>([]);

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
  private e2eeWorker: Worker | null = null;
  private audioLevelInterval: ReturnType<typeof setInterval> | null = null;

  // Non-reactive audio level cache — updated at 60ms by the polling interval,
  // read imperatively by VoiceCallPanel to drive the speaking ring animation.
  // Deliberately NOT $state to avoid triggering Svelte reactivity at 60Hz.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity -- deliberately non-reactive, polled imperatively at 60Hz
  private audioLevelCache = new Map<string, AudioLevelInfo>();

  // Local microphone audio analysis (Web Audio API) for instant level feedback.
  // LiveKit's audioLevel for the local participant comes from the server
  // (round-trip latency), so we read the mic input directly instead.
  private audioContext: AudioContext | null = null;
  private analyser: AnalyserNode | null = null;
  private analyserSource: MediaStreamAudioSourceNode | null = null;
  private analyserData: Float32Array<ArrayBuffer> | null = null;

  constructor(client: Client) {
    this.#client = client;
  }

  /**
   * Whether the user is currently in a call in the given room.
   */
  isInCall(roomId: string): boolean {
    return this.connected && this.roomId === roomId;
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

  /**
   * Join a voice call in a room.
   */
  async join(livekitUrl: string, roomId: string): Promise<void> {
    // Already in this call
    if (this.isInCall(roomId)) return;

    // Leave existing call first
    if (this.connected) {
      await this.leave();
    }

    this.connecting = true;
    this.roomId = roomId;
    let joinIntentRecorded = false;

    try {
      const intentResult = await this.#client
        .mutation(JoinVoiceCallMutation, { roomId })
        .toPromise();
      if (intentResult.error) {
        throw intentResult.error;
      }
      joinIntentRecorded = true;

      // Get token from server (pure query, no side effects)
      const result = await this.#client.query(VoiceCallTokenQuery, { roomId }).toPromise();
      if (result.error) {
        throw result.error;
      }

      const token = result.data?.room?.voiceCallToken?.token;
      const e2eeKey = result.data?.room?.voiceCallToken?.e2eeKey;
      if (!token || !e2eeKey) {
        throw new Error('Failed to get voice call token');
      }

      const keyProvider = new ExternalE2EEKeyProvider();
      const { default: E2EEWorker } = await import('livekit-client/e2ee-worker?worker');
      this.e2eeWorker = new E2EEWorker();

      // Create and connect LiveKit room
      this.room = new Room({
        encryption: {
          keyProvider,
          worker: this.e2eeWorker
        },
        audioCaptureDefaults: {
          autoGainControl: true,
          echoCancellation: true,
          noiseSuppression: true
        },
        videoCaptureDefaults: {
          resolution: VideoPresets.h720.resolution
        },
        publishDefaults: {
          audioPreset: AudioPresets.speech,
          dtx: true,
          red: true,
          simulcast: true
        },
        adaptiveStream: true,
        dynacast: true,
        disconnectOnPageLeave: true
      });

      this.setupRoomEventListeners();

      await keyProvider.setKey(e2eeKey);
      await this.room.setE2EEEnabled(true);
      await this.room.connect(livekitUrl, token);

      // Try to enable microphone, but join muted if no device is available
      try {
        await this.room.localParticipant.setMicrophoneEnabled(true);
        this.isMuted = false;
        this.setupLocalAudioAnalyser();
      } catch {
        this.isMuted = true;
      }

      this.connected = true;
      this.updateParticipants();
      await this.refreshDevices();
    } catch (err) {
      console.error('Failed to join voice call:', err);
      if (joinIntentRecorded) {
        await this.recordLeaveIntent(roomId);
      }
      this.cleanup();
      throw err;
    } finally {
      this.connecting = false;
    }
  }

  /**
   * Leave the current voice call.
   */
  async leave(): Promise<void> {
    if (!this.room) return;

    const roomId = this.roomId;
    if (roomId) {
      await this.recordLeaveIntent(roomId);
    }

    this.room.disconnect();
    this.cleanup();
  }

  private async recordLeaveIntent(roomId: string): Promise<void> {
    try {
      await this.#client.mutation(LeaveVoiceCallMutation, { roomId }).toPromise();
    } catch {
      // LiveKit disconnect/cleanup should still proceed if the intent write fails.
    }
  }

  /**
   * Toggle microphone mute.
   */
  async toggleMute(): Promise<void> {
    if (!this.room) return;

    const newMuted = !this.isMuted;
    await this.room.localParticipant.setMicrophoneEnabled(!newMuted);
    this.isMuted = newMuted;

    if (!newMuted) {
      this.setupLocalAudioAnalyser();
    } else {
      this.teardownLocalAudioAnalyser();
    }

    this.updateParticipants();
  }

  /**
   * Toggle camera on/off. Camera is always off by default.
   */
  async toggleCamera(): Promise<void> {
    if (!this.room) return;

    const newEnabled = !this.isCameraEnabled;
    try {
      await this.room.localParticipant.setCameraEnabled(newEnabled);
      this.isCameraEnabled = newEnabled;
    } catch {
      // Permission denied or no camera available — keep current state
      this.isCameraEnabled = false;
    }
    this.updateParticipants();
  }

  /**
   * Refresh available audio and video devices.
   */
  async refreshDevices(): Promise<void> {
    try {
      const [inputDevices, outputDevices, videoInputDevices] = await Promise.all([
        Room.getLocalDevices('audioinput'),
        Room.getLocalDevices('audiooutput'),
        Room.getLocalDevices('videoinput')
      ]);

      this.audioDevices = inputDevices;
      this.audioOutputDevices = outputDevices;
      this.videoDevices = videoInputDevices;

      // Set default selections if not already set
      if (!this.selectedDeviceId && inputDevices.length > 0) {
        this.selectedDeviceId = inputDevices[0].deviceId;
      }
      if (!this.selectedOutputDeviceId && outputDevices.length > 0) {
        this.selectedOutputDeviceId = outputDevices[0].deviceId;
      }
      if (!this.selectedVideoDeviceId && videoInputDevices.length > 0) {
        this.selectedVideoDeviceId = videoInputDevices[0].deviceId;
      }
    } catch {
      this.audioDevices = [];
      this.audioOutputDevices = [];
      this.videoDevices = [];
    }
  }

  /** @deprecated Use refreshDevices() instead */
  async refreshAudioDevices(): Promise<void> {
    return this.refreshDevices();
  }

  /**
   * Switch to a different audio input device.
   */
  async setAudioDevice(deviceId: string): Promise<void> {
    if (!this.room) return;

    await this.room.switchActiveDevice('audioinput', deviceId);
    this.selectedDeviceId = deviceId;

    // Reconnect analyser to the new mic track
    if (!this.isMuted) {
      this.setupLocalAudioAnalyser();
    }
  }

  /**
   * Switch to a different audio output device.
   */
  async setAudioOutputDevice(deviceId: string): Promise<void> {
    if (!this.room) return;

    await this.room.switchActiveDevice('audiooutput', deviceId);
    this.selectedOutputDeviceId = deviceId;
  }

  /**
   * Switch to a different video input device.
   */
  async setVideoDevice(deviceId: string): Promise<void> {
    if (!this.room) return;

    await this.room.switchActiveDevice('videoinput', deviceId);
    this.selectedVideoDeviceId = deviceId;
  }

  private setupRoomEventListeners(): void {
    if (!this.room) return;

    this.room.on(RoomEvent.ParticipantConnected, () => {
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
      if (this.connected) {
        toast.error('Voice call disconnected');
      }
      this.cleanup();
    });

    this.room.on(RoomEvent.MediaDevicesChanged, () => {
      this.refreshDevices();
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
      (track: RemoteTrack, _publication: RemoteTrackPublication) => {
        if (track.kind === Track.Kind.Audio) {
          track.attach();
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

    // Poll audio levels at ~60ms for smooth visual feedback.
    // ActiveSpeakersChanged fires at ~100ms which is slightly too coarse
    // for buttery equalizer animation.
    this.audioLevelInterval = setInterval(() => {
      this.updateAudioLevels();
    }, 60);
  }

  private updateParticipants(): void {
    if (!this.room) {
      this.participants = [];
      return;
    }

    const allParticipants: Participant[] = [
      this.room.localParticipant,
      ...Array.from(this.room.remoteParticipants.values())
    ];

    this.participants = allParticipants.map((p) => {
      const md = parseParticipantMetadata(p.metadata);
      return {
        identity: p.identity,
        name: p.name ?? p.identity,
        login: md.login ?? p.identity,
        avatarUrl: md.avatarUrl ?? null,
        isMuted: isParticipantMuted(p),
        isLocal: p === this.room!.localParticipant,
        connectionQuality: p.connectionQuality as CallParticipantInfo['connectionQuality'],
        isCameraEnabled: isParticipantCameraEnabled(p),
        videoTrack: getParticipantVideoTrack(p)
      };
    });
  }

  /**
   * Update the non-reactive audio level cache. Called at ~60ms.
   * Writes to a plain Map (not $state) so Svelte's reactive graph is
   * completely untouched — the UI reads this imperatively via getAudioLevel().
   */
  private updateAudioLevels(): void {
    if (!this.room) return;

    const localAudioLevel = this.getLocalAudioLevel();

    const allParticipants: Participant[] = [
      this.room.localParticipant,
      ...Array.from(this.room.remoteParticipants.values())
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
   * Set up a Web Audio API analyser connected to the local microphone track.
   * This gives us instant audio level readings without server round-trip.
   */
  private setupLocalAudioAnalyser(): void {
    this.teardownLocalAudioAnalyser();
    if (!this.room) return;

    const micPub = this.room.localParticipant.getTrackPublication(Track.Source.Microphone);
    const mediaStreamTrack = micPub?.track?.mediaStreamTrack;
    if (!mediaStreamTrack) return;

    try {
      this.audioContext = new AudioContext();
      this.analyser = this.audioContext.createAnalyser();
      this.analyser.fftSize = 256;
      this.analyserData = new Float32Array(this.analyser.fftSize) as Float32Array<ArrayBuffer>;

      const stream = new MediaStream([mediaStreamTrack]);
      this.analyserSource = this.audioContext.createMediaStreamSource(stream);
      this.analyserSource.connect(this.analyser);
      // Don't connect analyser to destination — we don't want to hear ourselves
    } catch {
      this.teardownLocalAudioAnalyser();
    }
  }

  private teardownLocalAudioAnalyser(): void {
    this.analyserSource?.disconnect();
    this.analyserSource = null;
    this.analyser?.disconnect();
    this.analyser = null;
    if (this.audioContext && this.audioContext.state !== 'closed') {
      this.audioContext.close().catch(() => {});
    }
    this.audioContext = null;
    this.analyserData = null;
  }

  /**
   * Read the current local microphone audio level (0–1) from the Web Audio
   * API analyser. Returns 0 if the analyser is not set up.
   */
  private getLocalAudioLevel(): number {
    if (!this.analyser || !this.analyserData) return 0;

    this.analyser.getFloatTimeDomainData(this.analyserData);

    // Compute RMS of the waveform samples
    let sumSq = 0;
    for (let i = 0; i < this.analyserData.length; i++) {
      sumSq += this.analyserData[i] * this.analyserData[i];
    }
    const rms = Math.sqrt(sumSq / this.analyserData.length);

    // Normalize: RMS of ~0.5 is very loud speech, scale so it maps to ~1.0
    return Math.min(rms * 2, 1);
  }

  private cleanup(): void {
    if (this.audioLevelInterval) {
      clearInterval(this.audioLevelInterval);
      this.audioLevelInterval = null;
    }
    this.teardownLocalAudioAnalyser();
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
    this.e2eeWorker?.terminate();
    this.e2eeWorker = null;
    this.connected = false;
    this.connecting = false;
    this.roomId = null;
    this.isMuted = false;
    this.isCameraEnabled = false;
    this.participants = [];
    this.audioDevices = [];
    this.selectedDeviceId = null;
    this.audioOutputDevices = [];
    this.selectedOutputDeviceId = null;
    this.videoDevices = [];
    this.selectedVideoDeviceId = null;
    this.audioLevelCache.clear();
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

function isParticipantMuted(participant: Participant): boolean {
  for (const pub of participant.getTrackPublications()) {
    if (pub.track?.source === Track.Source.Microphone) {
      return pub.isMuted;
    }
  }
  // No audio track = effectively muted
  return true;
}

function isParticipantCameraEnabled(participant: Participant): boolean {
  for (const pub of participant.getTrackPublications()) {
    if (pub.track?.source === Track.Source.Camera) {
      return !pub.isMuted;
    }
  }
  return false;
}

function getParticipantVideoTrack(participant: Participant): Track | null {
  for (const pub of participant.getTrackPublications()) {
    if (pub.track?.source === Track.Source.Camera && !pub.isMuted) {
      return pub.track;
    }
  }
  return null;
}
