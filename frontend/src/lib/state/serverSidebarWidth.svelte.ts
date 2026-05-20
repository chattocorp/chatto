import {
  getServerSidebarWidth,
  setServerSidebarWidth,
  SERVER_SIDEBAR_DEFAULT_WIDTH,
  SERVER_SIDEBAR_MAX_WIDTH,
  SERVER_SIDEBAR_MIN_WIDTH
} from '$lib/storage/serverSidebarWidth';

class ServerSidebarWidthState {
  #width = $state(getServerSidebarWidth());

  get value(): number {
    return this.#width;
  }

  set(width: number): void {
    const clamped = Math.min(
      SERVER_SIDEBAR_MAX_WIDTH,
      Math.max(SERVER_SIDEBAR_MIN_WIDTH, width)
    );
    this.#width = clamped;
    setServerSidebarWidth(clamped);
  }

  reset(): void {
    this.set(SERVER_SIDEBAR_DEFAULT_WIDTH);
  }
}

export const serverSidebarWidth = new ServerSidebarWidthState();
