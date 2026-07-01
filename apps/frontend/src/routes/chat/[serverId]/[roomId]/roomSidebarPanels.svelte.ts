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
  #desktopSessionState = $state<Record<string, RoomSidebarPanelState | undefined>>({});
  #mobilePanel = $state<RoomSidebarPanelState>(null);
  #mobileScope = $state<string | null>(null);
  #maximizedCallScope = $state<string | null>(null);
  #lastScope: string;

  constructor(getServerId: () => string, getRoomId: () => string) {
    this.#getServerId = getServerId;
    this.#getRoomId = getRoomId;
    this.#lastScope = this.#currentScope;
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

  get isDesktopCallMaximized(): boolean {
    return this.#desktopStateForRoom === 'call' && this.#maximizedCallScope === this.#currentScope;
  }

  toggleDesktopPanel(panel: RoomSidebarPanel): void {
    if (this.activeDesktopPanel === panel) {
      this.closeDesktop();
      return;
    }

    this.#setDesktopState(panel);
  }

  openDesktopPanel(panel: RoomSidebarPanel): void {
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

  openMobilePanel(panel: RoomSidebarPanel): void {
    this.#mobileScope = this.#currentScope;
    this.#mobilePanel = panel;
  }

  closeMobile(): void {
    this.#mobilePanel = null;
  }

  toggleDesktopCallMaximized(): void {
    this.syncCurrentScope();
    if (this.#desktopStateForRoom !== 'call') return;

    this.#maximizedCallScope = this.isDesktopCallMaximized ? null : this.#currentScope;
  }

  clearDesktopCallMaximized(): void {
    if (this.#maximizedCallScope !== null) {
      this.#maximizedCallScope = null;
    }
  }

  syncCurrentScope(): void {
    const scope = this.#currentScope;
    if (scope === this.#lastScope) return;

    this.#lastScope = scope;
    this.#maximizedCallScope = null;
  }

  get #currentScope(): string {
    return `${this.#getServerId()}:${this.#getRoomId()}`;
  }

  get #desktopStateForRoom(): RoomSidebarPanelState {
    if (this.#currentScope in this.#desktopSessionState) {
      return this.#desktopSessionState[this.#currentScope] ?? null;
    }

    return getRoomSidebarPanelState(this.#getServerId(), this.#getRoomId());
  }

  #setDesktopState(state: RoomSidebarPanelState): void {
    this.syncCurrentScope();
    const serverId = this.#getServerId();
    const roomId = this.#getRoomId();
    if (state !== 'call') {
      this.#maximizedCallScope = null;
    }
    if (state !== null) {
      setRoomSidebarPanelState(serverId, roomId, state);
    }
    this.#desktopSessionState = {
      ...this.#desktopSessionState,
      [`${serverId}:${roomId}`]: state
    };
  }
}
