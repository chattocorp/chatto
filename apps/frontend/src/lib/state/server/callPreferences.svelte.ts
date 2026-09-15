import { Codecs, serverSlot, type StorageSlot } from '$lib/storage/slot';

/** Saved device IDs are preferences, not permission grants or active track state. */
export interface CallPreferences {
  microphone: string;
  speaker: string;
  camera: string;
  /** Applied only when joining; toggling mute in a call does not change it. */
  joinMuted: boolean;
}

const defaults: CallPreferences = { microphone: '', speaker: '', camera: '', joinMuted: false };

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
      joinMuted: raw?.joinMuted === true
    });
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
