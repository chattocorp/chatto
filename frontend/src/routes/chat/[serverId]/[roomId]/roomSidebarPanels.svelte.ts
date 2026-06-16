import {
  getRoomSidebarPanelState,
  ROOM_SIDEBAR_DEFAULT_PANEL,
  setRoomSidebarPanelState,
  type RoomSidebarPanel,
  type RoomSidebarPanelState
} from '$lib/storage/roomSidebarPanel';

export class RoomSidebarPanelsState {
  #getServerId: () => string;
  #getRoomId: () => string;
  #desktopState = $state<RoomSidebarPanelState>(ROOM_SIDEBAR_DEFAULT_PANEL);
  #desktopScope = $state<string | null>(null);
  #mobilePanel = $state<RoomSidebarPanelState>(null);
  #mobileScope = $state<string | null>(null);

  constructor(getServerId: () => string, getRoomId: () => string) {
    this.#getServerId = getServerId;
    this.#getRoomId = getRoomId;
  }

  get selectedPanelForRoom(): RoomSidebarPanel {
    return this.#desktopStateForRoom ?? ROOM_SIDEBAR_DEFAULT_PANEL;
  }

  get activeDesktopPanel(): RoomSidebarPanelState {
    return this.#desktopStateForRoom;
  }

  get mobilePanel(): RoomSidebarPanelState {
    if (this.#mobileScope !== this.#currentScope) return null;
    return this.#mobilePanel;
  }

  toggleDesktopPanel(panel: RoomSidebarPanel): void {
    if (this.activeDesktopPanel === panel) {
      this.closeDesktop();
      return;
    }

    this.#setDesktopState(panel);
  }

  closeDesktop(): void {
    this.#setDesktopState(null);
  }

  toggleMobilePanel(panel: RoomSidebarPanel): void {
    if (this.mobilePanel === panel) {
      this.closeMobile();
      return;
    }

    this.#mobileScope = this.#currentScope;
    this.#mobilePanel = panel;
  }

  closeMobile(): void {
    this.#mobilePanel = null;
  }

  get #currentScope(): string {
    return `${this.#getServerId()}:${this.#getRoomId()}`;
  }

  get #desktopStateForRoom(): RoomSidebarPanelState {
    if (this.#desktopScope === this.#currentScope) {
      return this.#desktopState;
    }

    return getRoomSidebarPanelState(this.#getServerId(), this.#getRoomId());
  }

  #setDesktopState(state: RoomSidebarPanelState): void {
    const serverId = this.#getServerId();
    const roomId = this.#getRoomId();
    setRoomSidebarPanelState(serverId, roomId, state);
    this.#desktopScope = `${serverId}:${roomId}`;
    this.#desktopState = state;
  }
}
