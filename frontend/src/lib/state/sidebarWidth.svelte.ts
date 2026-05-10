import {
  getSecondarySidebarWidth,
  setSecondarySidebarWidth,
  SECONDARY_SIDEBAR_DEFAULT_WIDTH,
  SECONDARY_SIDEBAR_MAX_WIDTH,
  SECONDARY_SIDEBAR_MIN_WIDTH
} from '$lib/storage/secondarySidebarWidth';

class SecondarySidebarWidthState {
  #width = $state(getSecondarySidebarWidth());

  get value(): number {
    return this.#width;
  }

  set(width: number): void {
    const clamped = Math.min(
      SECONDARY_SIDEBAR_MAX_WIDTH,
      Math.max(SECONDARY_SIDEBAR_MIN_WIDTH, width)
    );
    this.#width = clamped;
    setSecondarySidebarWidth(clamped);
  }

  reset(): void {
    this.set(SECONDARY_SIDEBAR_DEFAULT_WIDTH);
  }
}

export const secondarySidebarWidth = new SecondarySidebarWidthState();
