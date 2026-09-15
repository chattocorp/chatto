import {
  defaultMicrophoneEffects,
  normalizeMicrophoneEffects,
  type MicrophoneEffects
} from '$lib/audio/microphoneEffects';
import { GATE_OFF, normalizeGateThreshold } from '$lib/audio/noiseGate';
import { Codecs, serverSlot, type StorageSlot } from '$lib/storage/slot';

/** Saved device IDs are preferences, not permission grants or active track state. */
export interface CallPreferences {
  microphone: string;
  speaker: string;
  camera: string;
  /** Applied only when joining; toggling mute in a call does not change it. */
  joinMuted: boolean;
  /** dBFS threshold; -60 disables the optional gate. */
  microphoneThreshold: number;
  effects: MicrophoneEffects;
}

const defaults: CallPreferences = {
  microphone: '',
  speaker: '',
  camera: '',
  joinMuted: false,
  effects: { ...defaultMicrophoneEffects },
  microphoneThreshold: GATE_OFF
};

/** Browser-local choices for one server. Empty device IDs follow the OS default. */
export class CallPreferencesState {
  readonly #slot: StorageSlot<CallPreferences>;
  #value: CallPreferences;

  constructor(serverId: string) {
    this.#slot = serverSlot(serverId, 'callPreferences', defaults, Codecs.json());
    const raw = this.#slot.get();
    this.#value = $state({
      microphone: typeof raw?.microphone === 'string' ? raw.microphone : '',
      speaker: typeof raw?.speaker === 'string' ? raw.speaker : '',
      camera: typeof raw?.camera === 'string' ? raw.camera : '',
      microphoneThreshold: normalizeGateThreshold(raw?.microphoneThreshold),
      effects: normalizeMicrophoneEffects(raw?.effects),
      joinMuted: raw?.joinMuted === true
    });
  }

  get effects(): Readonly<MicrophoneEffects> {
    return this.#value.effects;
  }

  setEffects(patch: Partial<MicrophoneEffects>): void {
    this.#value.effects = normalizeMicrophoneEffects({ ...this.#value.effects, ...patch });
    this.#slot.set(this.#value);
  }

  resetProcessing(): void {
    this.#value.effects = { ...defaultMicrophoneEffects };
    this.#value.microphoneThreshold = GATE_OFF;
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
