import { microphoneEffectsForAmount, normalizeVoiceAmount } from '$lib/audio/microphoneEffects';
import { GATE_OFF, normalizeGateThreshold } from '$lib/audio/noiseGate';
import { Codecs, serverSlot, type StorageSlot } from '$lib/storage/slot';

/** Listener-local playback levels. Percentages above 100 boost the received signal. */
export interface ParticipantAudioPreferences {
  voiceVolume: number;
  streamVolume: number;
}
export type ParticipantVolumeControl = keyof ParticipantAudioPreferences;
export const DEFAULT_PARTICIPANT_AUDIO: Readonly<ParticipantAudioPreferences> = {
  voiceVolume: 100,
  streamVolume: 100
};

/** Reject corrupt values and bound playback gain to 0–200%. */
export function normalizeParticipantVolume(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.round(Math.max(0, Math.min(200, value)))
    : 100;
}

function normalizeParticipantAudio(value: unknown): Record<string, ParticipantAudioPreferences> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
  return Object.fromEntries(
    Object.entries(value).flatMap(([id, settings]) => {
      if (!settings || typeof settings !== 'object' || Array.isArray(settings)) return [];
      return [
        [
          id,
          {
            voiceVolume: normalizeParticipantVolume(settings.voiceVolume),
            streamVolume: normalizeParticipantVolume(settings.streamVolume)
          }
        ]
      ];
    })
  );
}

/** Saved device IDs are preferences, not permission grants or active track state. */
export interface CallPreferences {
  /** Keyed by stable Chatto user ID, never by a transient LiveKit publisher ID. */
  participantAudio: Record<string, ParticipantAudioPreferences>;
  microphone: string;
  speaker: string;
  camera: string;
  /** Applied only when joining; toggling mute in a call does not change it. */
  joinMuted: boolean;
  /** dBFS threshold; -60 disables the optional gate. */
  microphoneThreshold: number;
  /** Voice processing amount, 0–100; the noise gate remains independent. */
  voiceAmount: number;
}

const defaults: CallPreferences = {
  participantAudio: {},
  microphone: '',
  speaker: '',
  camera: '',
  joinMuted: false,
  voiceAmount: 0,
  microphoneThreshold: GATE_OFF
};

/** Browser-local choices for one server. Empty device IDs follow the OS default. */
export class CallPreferencesState {
  readonly #slot: StorageSlot<CallPreferences>;
  #value: CallPreferences;

  constructor(serverId: string) {
    this.#slot = serverSlot(serverId, 'callPreferences', defaults, Codecs.json());
    const raw = this.#slot.get();
    // Preserve the previous preset positions; experimental enabled effects use the midpoint.
    const legacy = raw as unknown as {
      processingPreset?: string;
      effects?: { lowCut?: unknown; equalizer?: unknown; compressor?: unknown };
    } | null;
    const oldAmount =
      legacy?.processingPreset === 'strong'
        ? 100
        : legacy?.processingPreset === 'subtle'
          ? 50
          : legacy?.processingPreset !== undefined
            ? 0
            : legacy?.effects?.lowCut === true ||
                legacy?.effects?.equalizer === true ||
                legacy?.effects?.compressor === true
              ? 50
              : 0;
    this.#value = $state({
      participantAudio: normalizeParticipantAudio(raw?.participantAudio),
      microphone: typeof raw?.microphone === 'string' ? raw.microphone : '',
      speaker: typeof raw?.speaker === 'string' ? raw.speaker : '',
      camera: typeof raw?.camera === 'string' ? raw.camera : '',
      microphoneThreshold: normalizeGateThreshold(raw?.microphoneThreshold),
      voiceAmount: normalizeVoiceAmount(
        raw?.voiceAmount === undefined ? oldAmount : raw.voiceAmount
      ),
      joinMuted: raw?.joinMuted === true
    });
  }

  /** Return saved levels, or unity gain for an unconfigured participant. */
  getParticipantAudio(userId: string): Readonly<ParticipantAudioPreferences> {
    return Object.hasOwn(this.#value.participantAudio, userId)
      ? this.#value.participantAudio[userId]
      : DEFAULT_PARTICIPANT_AUDIO;
  }

  /** Persist one source level while preserving other participant controls. */
  setParticipantVolume(userId: string, control: ParticipantVolumeControl, value: number): void {
    this.#value.participantAudio = {
      ...this.#value.participantAudio,
      [userId]: {
        ...this.getParticipantAudio(userId),
        [control]: normalizeParticipantVolume(value)
      }
    };
    this.#slot.set(this.#value);
  }

  /** Remove the override so both sources return to their defaults. */
  resetParticipantAudio(userId: string): void {
    const { [userId]: _removed, ...remaining } = this.#value.participantAudio;
    void _removed;
    this.#value.participantAudio = remaining;
    this.#slot.set(this.#value);
  }

  get effects() {
    return microphoneEffectsForAmount(this.#value.voiceAmount);
  }

  get voiceAmount(): number {
    return this.#value.voiceAmount;
  }

  /** Change voice processing without changing the gate or capture choices. */
  setVoiceAmount(amount: number): void {
    this.#value.voiceAmount = normalizeVoiceAmount(amount);
    this.#slot.set(this.#value);
  }

  get microphoneThreshold() {
    return this.#value.microphoneThreshold;
  }

  setMicrophoneThreshold(value: number): void {
    this.#value.microphoneThreshold = normalizeGateThreshold(value);
    this.#slot.set(this.#value);
  }

  get microphone() {
    return this.#value.microphone;
  }
  get speaker() {
    return this.#value.speaker;
  }
  get camera() {
    return this.#value.camera;
  }
  get joinMuted() {
    return this.#value.joinMuted;
  }

  setDevice(kind: MediaDeviceKind, deviceId: string): void {
    const key =
      kind === 'audioinput' ? 'microphone' : kind === 'audiooutput' ? 'speaker' : 'camera';
    this.#value[key] = deviceId;
    this.#slot.set(this.#value);
  }

  setJoinMuted(value: boolean): void {
    this.#value.joinMuted = value;
    this.#slot.set(this.#value);
  }
}

/** Resolve a saved choice without erasing it when hardware is unavailable. */
export function availableCallDevice(saved: string, devices: MediaDeviceInfo[]): string {
  return devices.some((device) => device.deviceId === saved) ? saved : '';
}
