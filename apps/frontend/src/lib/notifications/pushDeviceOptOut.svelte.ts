/**
 * Reactive device-wide push opt-out.
 *
 * Push notifications are on for every registered server or for none of them.
 * The opt-out lives in local storage, owned by the registration coordinator,
 * so every tab honours it. This state makes reactive readers update when this
 * tab or another tab changes it.
 */

import { isPushDisabledOnDevice, pushDisabledOnDeviceKey } from './pushRegistrationCoordinator';

class PushDeviceOptOutState {
  /** Changes whenever the stored opt-out may have changed. */
  #version = $state(0);
  #watching = false;

  /** Whether the user turned push notifications off on this device. */
  get current(): boolean {
    this.#watch();
    void this.#version;
    return isPushDisabledOnDevice();
  }

  /** Makes reactive readers read the stored opt-out again. */
  changed(): void {
    this.#version++;
  }

  #watch(): void {
    if (
      this.#watching ||
      typeof window === 'undefined' ||
      typeof window.addEventListener !== 'function'
    ) {
      return;
    }
    this.#watching = true;
    window.addEventListener('storage', (event) => {
      if (event.key === null || event.key === pushDisabledOnDeviceKey) this.changed();
    });
  }
}

/** The single owner of this page's view of the device-wide push opt-out. */
export const pushDeviceOptOut = new PushDeviceOptOutState();
