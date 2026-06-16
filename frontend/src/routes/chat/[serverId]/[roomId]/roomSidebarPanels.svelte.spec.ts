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
});
