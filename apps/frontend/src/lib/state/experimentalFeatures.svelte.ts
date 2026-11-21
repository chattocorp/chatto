import { Codecs, globalSlot } from '@chatto/client/storage/slot';

/** One browser-wide opt-in. Separate keys let tabs change different experiments independently. */
class ClientExperiment {
  readonly #slot;
  #enabled = $state(false);

  constructor(name: string) {
    this.#slot = globalSlot(`experiments:${name}`, false, Codecs.boolean);
    this.#enabled = this.#slot.get();
  }

  get enabled(): boolean {
    return this.#enabled;
  }

  set enabled(value: boolean) {
    this.#enabled = value;
    this.#slot.set(value);
  }

  /** Read the latest value on remote writes, removals, or a storage clear. */
  refresh(event: StorageEvent): void {
    if (event.key === this.#slot.key || event.key === null) this.#enabled = this.#slot.get();
  }
}

export const experimentalFeatures = {
  newMessageHeader: new ClientExperiment('new-message-header'),
  /** Render tab and `glsl rendered` previews for GLSL code blocks in messages. */
  glslShaders: new ClientExperiment('glsl-shaders')
};

/** Apply same-origin local storage changes to the mounted app. */
export function refreshExperimentalFeatures(event: StorageEvent): void {
  if (event.storageArea !== window.localStorage) return;
  for (const feature of Object.values(experimentalFeatures)) feature.refresh(event);
}
