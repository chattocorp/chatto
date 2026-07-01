import { beforeEach, describe, expect, it } from 'vitest';
import { getRoomSidebarPanelState, setRoomSidebarPanelState } from '$lib/storage/roomSidebarPanel';
import { RoomSidebarPanelsState } from './roomSidebarPanels.svelte';

describe('RoomSidebarPanelsState', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('persists desktop panel changes but keeps closes session-local', () => {
    const sidebar = new RoomSidebarPanelsState(
      () => 'server-a',
      () => 'room-1'
    );

    sidebar.toggleDesktopPanel('files');

    expect(sidebar.activeDesktopPanel).toBe('files');
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('files');

    sidebar.toggleDesktopPanel('files');

    expect(sidebar.activeDesktopPanel).toBeNull();
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('files');
  });

  it('keeps desktop closes local to the current app session', () => {
    setRoomSidebarPanelState('server-a', 'room-1', null);
    const sidebar = new RoomSidebarPanelsState(
      () => 'server-a',
      () => 'room-1'
    );

    sidebar.closeDesktop();

    expect(sidebar.activeDesktopPanel).toBeNull();
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('members');

    const freshSession = new RoomSidebarPanelsState(
      () => 'server-a',
      () => 'room-1'
    );
    expect(freshSession.activeDesktopPanel).toBe('members');
  });

  it('does not let mobile overlay selection overwrite a desktop close in the current session', () => {
    const sidebar = new RoomSidebarPanelsState(
      () => 'server-a',
      () => 'room-1'
    );

    sidebar.closeDesktop();
    sidebar.toggleMobilePanel('files');

    expect(sidebar.activeDesktopPanel).toBeNull();
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('members');
  });

  it('does not let mobile overlay selection overwrite a persisted desktop panel', () => {
    setRoomSidebarPanelState('server-a', 'room-1', 'files');
    const sidebar = new RoomSidebarPanelsState(
      () => 'server-a',
      () => 'room-1'
    );

    sidebar.toggleMobilePanel('members');

    expect(sidebar.mobilePanel).toBe('members');
    expect(sidebar.activeDesktopPanel).toBe('files');
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('files');
  });

  it('persists the call panel for desktop rooms', () => {
    const sidebar = new RoomSidebarPanelsState(
      () => 'server-a',
      () => 'room-1'
    );

    sidebar.toggleDesktopPanel('call');

    expect(sidebar.activeDesktopPanel).toBe('call');
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('call');
  });

  it('opens the requested desktop panel even after the panel was session-closed', () => {
    const sidebar = new RoomSidebarPanelsState(
      () => 'server-a',
      () => 'room-1'
    );

    sidebar.closeDesktop();
    sidebar.openDesktopPanel('call');

    expect(sidebar.activeDesktopPanel).toBe('call');
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('call');
  });

  it('opens the requested mobile panel without toggling it closed', () => {
    const sidebar = new RoomSidebarPanelsState(
      () => 'server-a',
      () => 'room-1'
    );

    sidebar.openMobilePanel('call');
    sidebar.openMobilePanel('call');

    expect(sidebar.mobilePanel).toBe('call');
  });

  it('treats mobile overlay state as closed after the room changes', () => {
    let roomId = 'room-1';
    const sidebar = new RoomSidebarPanelsState(
      () => 'server-a',
      () => roomId
    );

    sidebar.toggleMobilePanel('files');
    expect(sidebar.mobilePanel).toBe('files');

    roomId = 'room-2';
    expect(sidebar.mobilePanel).toBeNull();
  });

  it('toggles maximized call state only while the desktop call panel is active', () => {
    const sidebar = new RoomSidebarPanelsState(
      () => 'server-a',
      () => 'room-1'
    );

    sidebar.toggleDesktopCallMaximized();
    expect(sidebar.isDesktopCallMaximized).toBe(false);

    sidebar.openDesktopPanel('call');
    sidebar.toggleDesktopCallMaximized();
    expect(sidebar.isDesktopCallMaximized).toBe(true);

    sidebar.toggleDesktopCallMaximized();
    expect(sidebar.isDesktopCallMaximized).toBe(false);
  });

  it('clears maximized call state when the desktop panel closes or switches away', () => {
    const sidebar = new RoomSidebarPanelsState(
      () => 'server-a',
      () => 'room-1'
    );

    sidebar.openDesktopPanel('call');
    sidebar.toggleDesktopCallMaximized();
    sidebar.openDesktopPanel('files');
    expect(sidebar.isDesktopCallMaximized).toBe(false);

    sidebar.openDesktopPanel('call');
    sidebar.toggleDesktopCallMaximized();
    sidebar.closeDesktop();
    expect(sidebar.isDesktopCallMaximized).toBe(false);
  });

  it('clears maximized call state when the active call ends', () => {
    const sidebar = new RoomSidebarPanelsState(
      () => 'server-a',
      () => 'room-1'
    );

    sidebar.openDesktopPanel('call');
    sidebar.toggleDesktopCallMaximized();
    expect(sidebar.isDesktopCallMaximized).toBe(true);

    sidebar.clearDesktopCallMaximized();

    expect(sidebar.isDesktopCallMaximized).toBe(false);
  });

  it('does not leak maximized call state after the room changes', () => {
    let roomId = 'room-1';
    const sidebar = new RoomSidebarPanelsState(
      () => 'server-a',
      () => roomId
    );

    sidebar.openDesktopPanel('call');
    sidebar.toggleDesktopCallMaximized();
    expect(sidebar.isDesktopCallMaximized).toBe(true);

    roomId = 'room-2';
    sidebar.syncCurrentScope();
    sidebar.openDesktopPanel('call');
    expect(sidebar.isDesktopCallMaximized).toBe(false);

    roomId = 'room-1';
    sidebar.syncCurrentScope();
    expect(sidebar.isDesktopCallMaximized).toBe(false);
  });

  it('keeps maximized call state out of persisted storage', () => {
    const sidebar = new RoomSidebarPanelsState(
      () => 'server-a',
      () => 'room-1'
    );

    sidebar.openDesktopPanel('call');
    sidebar.toggleDesktopCallMaximized();

    expect(sidebar.isDesktopCallMaximized).toBe(true);
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('call');

    const freshSession = new RoomSidebarPanelsState(
      () => 'server-a',
      () => 'room-1'
    );
    expect(freshSession.activeDesktopPanel).toBe('call');
    expect(freshSession.isDesktopCallMaximized).toBe(false);
  });
});
