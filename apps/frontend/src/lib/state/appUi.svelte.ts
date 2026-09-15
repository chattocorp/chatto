import { createContext } from 'svelte';
import {
  getRoomSidebarPanelState,
  setRoomSidebarPanelState,
  type RoomSidebarPreference,
  type RoomSidebarPanel,
  type RoomSidebarPanelState
} from '$lib/storage/roomSidebarPanel';

export type AppRoomScope = {
  serverId: string;
  roomId: string;
};

export type RoomSidebarPresentation = 'desktop' | 'mobile';

/** Return the room sidebar presentation used at the current Tailwind `lg` breakpoint. */
export function getRoomSidebarPresentation(): RoomSidebarPresentation {
  return window.matchMedia('(min-width: 1024px)').matches ? 'desktop' : 'mobile';
}

type RoomSidebarPanelRequest = AppRoomScope & {
  panel: RoomSidebarPanel;
  presentation: RoomSidebarPresentation;
};

export type AppFullscreenSurface = {
  id?: string;
  surface: string;
};

type RoomSidebarProfileState = AppRoomScope & {
  userId: string;
};

type RoomSidebarProfileRequest = RoomSidebarProfileState & {
  presentation: RoomSidebarPresentation;
};

/**
 * App-scoped UI state that should be shared across route components.
 *
 * The URL remains the source of truth for the active server/room; route
 * components report that scope here so sibling UI concerns such as the room
 * sidebar and call-wide mode can coordinate without custom event bridges.
 */
export class AppUiState {
  #activeServerId = $state<string | null>(null);
  #activeRoomId = $state<string | null>(null);
  #desktopRoomSidebarSessionState = $state<Record<string, RoomSidebarPreference>>({});
  #mobileRoomSidebarPanel = $state<RoomSidebarPanelState>(null);
  #mobileRoomSidebarScope = $state<string | null>(null);
  #roomSidebarProfile = $state<RoomSidebarProfileState | null>(null);
  #roomCallWideScope = $state<AppRoomScope | null>(null);
  #fullscreenSurface = $state<AppFullscreenSurface | null>(null);
  #roomSidebarPanelRequest: RoomSidebarPanelRequest | null = null;
  #roomSidebarProfileRequest: RoomSidebarProfileRequest | null = null;

  get activeServerId(): string | null {
    return this.#activeServerId;
  }

  get activeRoomId(): string | null {
    return this.#activeRoomId;
  }

  get activeRoomScope(): AppRoomScope | null {
    if (!this.#activeServerId || !this.#activeRoomId) return null;
    return { serverId: this.#activeServerId, roomId: this.#activeRoomId };
  }

  setActiveServer(serverId: string): void {
    const previousScope = this.#activeRoomScopeKey;

    this.#activeServerId = serverId;
    this.#activeRoomId = null;
    this.#clearRoomSidebarProfiles();
    if (previousScope !== null) this.disableRoomCallWide();
  }

  setActiveRoomScope(serverId: string, roomId: string): void {
    const previousScope = this.#activeRoomScopeKey;
    this.#activeServerId = serverId;
    this.#activeRoomId = roomId;

    const nextScope = this.#activeRoomScopeKey;
    if (previousScope !== null && previousScope !== nextScope) {
      this.#clearRoomSidebarProfiles();
      this.disableRoomCallWide();
    }

    if (nextScope && !(nextScope in this.#desktopRoomSidebarSessionState)) {
      this.#desktopRoomSidebarSessionState[nextScope] = getRoomSidebarPanelState(serverId, roomId);
    }
    this.#applyRoomSidebarPanelRequest();
    this.#applyRoomSidebarProfileRequest();
  }

  clearActiveRoomScope(serverId: string, roomId: string): void {
    this.disableRoomCallWideFor(serverId, roomId);
    if (this.#activeServerId !== serverId || this.#activeRoomId !== roomId) return;
    this.#activeRoomId = null;
  }

  get activeDesktopRoomSidebarPanel(): RoomSidebarPanelState {
    const scope = this.#activeRoomScopeKey;
    if (!scope) return null;
    const preference = this.#desktopRoomSidebarSessionState[scope];
    return typeof preference === 'object' && preference !== null
      ? preference.previousPanel
      : (preference ?? null);
  }

  /**
   * Resolve the desktop profile from the current DM participants. Missing choices
   * default to that profile. Saved closed/panel choices suppress the default.
   * This derived presentation must not create a mobile profile or write storage.
   */
  desktopRoomSidebarProfileUserId(defaultUserId: string | null): string | null {
    if (this.activeRoomSidebarProfileUserId) return this.activeRoomSidebarProfileUserId;
    const scope = this.#activeRoomScopeKey;
    if (!scope) return null;
    const preference = this.#desktopRoomSidebarSessionState[scope];
    return preference === undefined || (typeof preference === 'object' && preference !== null)
      ? defaultUserId
      : null;
  }

  get mobileRoomSidebarPanel(): RoomSidebarPanelState {
    if (this.#mobileRoomSidebarScope !== this.#activeRoomScopeKey) return null;
    return this.#mobileRoomSidebarPanel;
  }

  /** The profile currently shown for the active room, if any. */
  get activeRoomSidebarProfileUserId(): string | null {
    return this.#profileUserIdForActiveRoom(this.#roomSidebarProfile);
  }

  toggleDesktopRoomSidebarPanel(panel: RoomSidebarPanel): void {
    const preference = this.#activeRoomScopeKey
      ? this.#desktopRoomSidebarSessionState[this.#activeRoomScopeKey]
      : undefined;
    if (
      this.activeRoomSidebarProfileUserId ||
      (typeof preference === 'object' && preference !== null)
    ) {
      this.closeRoomSidebarProfile('desktop');
      this.openDesktopRoomSidebarPanel(panel);
      return;
    }
    if (this.activeDesktopRoomSidebarPanel === panel) {
      this.closeDesktopRoomSidebarPanel();
      return;
    }

    this.openDesktopRoomSidebarPanel(panel);
  }

  openDesktopRoomSidebarPanel(panel: RoomSidebarPanel): void {
    this.#roomSidebarProfile = null;
    this.#setDesktopRoomSidebarPanel(panel);
    if (panel !== 'call') this.disableRoomCallWideForActiveRoom();
  }

  closeDesktopRoomSidebarPanel(): void {
    this.#setDesktopRoomSidebarPanel(null);
    this.disableRoomCallWideForActiveRoom();
  }

  toggleMobileRoomSidebarPanel(panel: RoomSidebarPanel): void {
    if (this.activeRoomSidebarProfileUserId) {
      this.closeRoomSidebarProfile('mobile');
      this.openMobileRoomSidebarPanel(panel);
      return;
    }
    if (this.mobileRoomSidebarPanel === panel) {
      this.closeMobileRoomSidebarPanel();
      return;
    }

    this.openMobileRoomSidebarPanel(panel);
  }

  openMobileRoomSidebarPanel(panel: RoomSidebarPanel): void {
    const scope = this.#activeRoomScopeKey;
    if (!scope) return;

    this.#roomSidebarProfile = null;
    this.#mobileRoomSidebarScope = scope;
    this.#mobileRoomSidebarPanel = panel;
  }

  closeMobileRoomSidebarPanel(): void {
    this.#mobileRoomSidebarPanel = null;
  }

  /**
   * Show a user profile in the room sidebar.
   *
   * The selected room-extras panel is retained so closing the profile
   * returns the viewer to the prior panel. Desktop choices survive reloads.
   * The responsive layout selects desktop or mobile presentation at render
   * time, so the profile remains visible after a breakpoint change.
   */
  openRoomSidebarProfile(
    userId: string,
    presentation: RoomSidebarPresentation = getRoomSidebarPresentation()
  ): void {
    const scope = this.activeRoomScope;
    if (!scope) return;
    if (presentation === 'desktop') {
      this.#setDesktopRoomSidebarPanel({
        view: 'profile',
        previousPanel: this.activeDesktopRoomSidebarPanel
      });
    }
    this.#roomSidebarProfile = { ...scope, userId };
    this.disableRoomCallWideForActiveRoom();
  }

  /** Close a profile and remember its underlying panel on desktop only. */
  closeRoomSidebarProfile(
    presentation: RoomSidebarPresentation = getRoomSidebarPresentation()
  ): void {
    this.#roomSidebarProfile = null;
    if (presentation === 'desktop') {
      this.#setDesktopRoomSidebarPanel(this.activeDesktopRoomSidebarPanel);
    }
  }

  /**
   * Open a room sidebar panel now or when its target room becomes active.
   *
   * This keeps cross-room navigation requests inside the app-scoped UI owner
   * instead of relaying them through browser storage events.
   */
  requestRoomSidebarPanel(
    serverId: string,
    roomId: string,
    panel: RoomSidebarPanel,
    presentation: RoomSidebarPresentation
  ): void {
    this.#roomSidebarPanelRequest = { serverId, roomId, panel, presentation };
    this.#applyRoomSidebarPanelRequest();
  }

  /**
   * Show a user's profile in a room sidebar now or after its target room opens.
   *
   * A profile request can accompany direct-message navigation. It is consumed
   * only after the direct-message route reports the same active room scope.
   */
  requestRoomSidebarProfile(
    serverId: string,
    roomId: string,
    userId: string,
    presentation: RoomSidebarPresentation = getRoomSidebarPresentation()
  ): void {
    this.#roomSidebarProfileRequest = { serverId, roomId, userId, presentation };
    this.#applyRoomSidebarProfileRequest();
  }

  get roomCallWideScope(): AppRoomScope | null {
    return this.#roomCallWideScope;
  }

  get isRoomCallWide(): boolean {
    return this.#roomCallWideScope !== null;
  }

  isRoomCallWideFor(serverId: string, roomId: string): boolean {
    return (
      this.#roomCallWideScope?.serverId === serverId && this.#roomCallWideScope.roomId === roomId
    );
  }

  setRoomCallWide(serverId: string, roomId: string, wide: boolean): void {
    this.#roomCallWideScope = wide ? { serverId, roomId } : null;
  }

  toggleRoomCallWide(serverId: string, roomId: string): void {
    this.setRoomCallWide(serverId, roomId, !this.isRoomCallWideFor(serverId, roomId));
  }

  disableRoomCallWide(): void {
    this.#roomCallWideScope = null;
  }

  disableRoomCallWideFor(serverId: string, roomId: string): void {
    if (this.isRoomCallWideFor(serverId, roomId)) this.disableRoomCallWide();
  }

  disableRoomCallWideForActiveRoom(): void {
    const scope = this.activeRoomScope;
    if (scope) this.disableRoomCallWideFor(scope.serverId, scope.roomId);
  }

  get fullscreenSurface(): AppFullscreenSurface | null {
    return this.#fullscreenSurface;
  }

  get hasFullscreenSurface(): boolean {
    return this.#fullscreenSurface !== null;
  }

  setFullscreenSurface(surface: AppFullscreenSurface): void {
    this.#fullscreenSurface = surface;
  }

  clearFullscreenSurface(): void {
    this.#fullscreenSurface = null;
  }

  get #activeRoomScopeKey(): string | null {
    if (!this.#activeServerId || !this.#activeRoomId) return null;
    return roomScopeKey(this.#activeServerId, this.#activeRoomId);
  }

  #setDesktopRoomSidebarPanel(panel: Exclude<RoomSidebarPreference, undefined>): void {
    const scope = this.activeRoomScope;
    if (!scope) return;

    setRoomSidebarPanelState(scope.serverId, scope.roomId, panel);
    this.#desktopRoomSidebarSessionState = {
      ...this.#desktopRoomSidebarSessionState,
      [roomScopeKey(scope.serverId, scope.roomId)]: panel
    };
  }

  #profileUserIdForActiveRoom(profile: RoomSidebarProfileState | null): string | null {
    const scope = this.activeRoomScope;
    if (!scope || !profile) return null;
    return profile.serverId === scope.serverId && profile.roomId === scope.roomId
      ? profile.userId
      : null;
  }

  #clearRoomSidebarProfiles(): void {
    this.#roomSidebarProfile = null;
  }

  #applyRoomSidebarPanelRequest(): void {
    const request = this.#roomSidebarPanelRequest;
    if (
      !request ||
      request.serverId !== this.#activeServerId ||
      request.roomId !== this.#activeRoomId
    ) {
      return;
    }

    this.#roomSidebarPanelRequest = null;
    if (request.presentation === 'desktop') {
      this.openDesktopRoomSidebarPanel(request.panel);
      return;
    }

    this.openMobileRoomSidebarPanel(request.panel);
  }

  #applyRoomSidebarProfileRequest(): void {
    const request = this.#roomSidebarProfileRequest;
    if (
      !request ||
      request.serverId !== this.#activeServerId ||
      request.roomId !== this.#activeRoomId
    ) {
      return;
    }

    this.#roomSidebarProfileRequest = null;
    this.openRoomSidebarProfile(request.userId, request.presentation);
  }
}

function roomScopeKey(serverId: string, roomId: string): string {
  return `${serverId}:${roomId}`;
}

const [getAppUiContext, setAppUiContext] = createContext<AppUiState>();

export function provideAppUiState(state = new AppUiState()): AppUiState {
  setAppUiContext(state);
  return state;
}

export function getAppUiState(): AppUiState {
  return getAppUiContext();
}
