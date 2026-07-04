import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AppUiState } from './appUi.svelte';

const mocks = vi.hoisted(() => ({
  roomSidebarPanel: new Map<string, string>(),
  getRoomSidebarPanelState: vi.fn((serverId: string, roomId: string) => {
    return mocks.roomSidebarPanel.get(`${serverId}:${roomId}`) ?? 'members';
  }),
  setRoomSidebarPanelState: vi.fn((serverId: string, roomId: string, panel: string | null) => {
    if (panel) mocks.roomSidebarPanel.set(`${serverId}:${roomId}`, panel);
  })
}));

vi.mock('$lib/storage/roomSidebarPanel', () => ({
  ROOM_SIDEBAR_DEFAULT_PANEL: 'members',
  getRoomSidebarPanelState: mocks.getRoomSidebarPanelState,
  setRoomSidebarPanelState: mocks.setRoomSidebarPanelState
}));

describe('AppUiState', () => {
  beforeEach(() => {
    mocks.roomSidebarPanel.clear();
    mocks.getRoomSidebarPanelState.mockClear();
    mocks.setRoomSidebarPanelState.mockClear();
  });

  it('tracks the active chat route scope', () => {
    const appUi = new AppUiState();

    expect(appUi.activeRoomScope).toBe(null);

    appUi.setActiveRoomScope('server-a', 'room-1');

    expect(appUi.activeServerId).toBe('server-a');
    expect(appUi.activeRoomId).toBe('room-1');
    expect(appUi.activeRoomScope).toEqual({ serverId: 'server-a', roomId: 'room-1' });
  });

  it('clears room scope when the active route moves to a server page', () => {
    const appUi = new AppUiState();

    appUi.setActiveRoomScope('server-a', 'room-1');
    appUi.setRoomCallWide('server-a', 'room-1', true);
    appUi.setActiveServer('server-a');

    expect(appUi.activeServerId).toBe('server-a');
    expect(appUi.activeRoomId).toBe(null);
    expect(appUi.activeRoomScope).toBe(null);
    expect(appUi.isRoomCallWide).toBe(false);
  });

  it('tracks the active room sidebar panel inside the active room scope', () => {
    const appUi = new AppUiState();

    appUi.setActiveRoomScope('server-a', 'room-1');

    expect(appUi.selectedDesktopRoomSidebarPanel).toBe('members');
    expect(appUi.activeDesktopRoomSidebarPanel).toBe('members');

    appUi.openDesktopRoomSidebarPanel('files');

    expect(appUi.activeDesktopRoomSidebarPanel).toBe('files');
    expect(mocks.setRoomSidebarPanelState).toHaveBeenCalledWith('server-a', 'room-1', 'files');

    appUi.closeDesktopRoomSidebarPanel();
    expect(appUi.activeDesktopRoomSidebarPanel).toBe(null);

    appUi.setActiveRoomScope('server-a', 'room-2');
    expect(appUi.activeDesktopRoomSidebarPanel).toBe('members');
  });

  it('scopes mobile room sidebar state to the active room', () => {
    const appUi = new AppUiState();

    appUi.setActiveRoomScope('server-a', 'room-1');
    appUi.openMobileRoomSidebarPanel('files');

    expect(appUi.mobileRoomSidebarPanel).toBe('files');

    appUi.setActiveRoomScope('server-a', 'room-2');
    expect(appUi.mobileRoomSidebarPanel).toBe(null);
  });

  it('tracks the scoped wide call room', () => {
    const appUi = new AppUiState();

    expect(appUi.isRoomCallWide).toBe(false);
    expect(appUi.isRoomCallWideFor('server-a', 'room-1')).toBe(false);

    appUi.setRoomCallWide('server-a', 'room-1', true);

    expect(appUi.isRoomCallWide).toBe(true);
    expect(appUi.roomCallWideScope).toEqual({ serverId: 'server-a', roomId: 'room-1' });
    expect(appUi.isRoomCallWideFor('server-a', 'room-1')).toBe(true);
    expect(appUi.isRoomCallWideFor('server-a', 'room-2')).toBe(false);
  });

  it('toggles and disables the active wide call scope', () => {
    const appUi = new AppUiState();

    appUi.toggleRoomCallWide('server-a', 'room-1');
    expect(appUi.isRoomCallWideFor('server-a', 'room-1')).toBe(true);

    appUi.disableRoomCallWideFor('server-a', 'room-2');
    expect(appUi.isRoomCallWideFor('server-a', 'room-1')).toBe(true);

    appUi.disableRoomCallWideFor('server-a', 'room-1');
    expect(appUi.isRoomCallWide).toBe(false);
  });

  it('clears wide mode when the viewed room scope changes', () => {
    const appUi = new AppUiState();

    appUi.setActiveRoomScope('server-a', 'room-1');
    appUi.setRoomCallWide('server-a', 'room-1', true);
    appUi.setActiveRoomScope('server-a', 'room-1');
    expect(appUi.isRoomCallWideFor('server-a', 'room-1')).toBe(true);

    appUi.setActiveRoomScope('server-a', 'room-2');
    expect(appUi.isRoomCallWide).toBe(false);
  });

  it('exposes generic fullscreen UI state for top-level consumers', () => {
    const appUi = new AppUiState();

    expect(appUi.hasFullscreenSurface).toBe(false);

    appUi.setFullscreenSurface({ surface: 'media-viewer', id: 'asset-1' });
    expect(appUi.hasFullscreenSurface).toBe(true);
    expect(appUi.fullscreenSurface).toEqual({ surface: 'media-viewer', id: 'asset-1' });

    appUi.clearFullscreenSurface();
    expect(appUi.fullscreenSurface).toBe(null);
  });
});
