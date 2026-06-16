import { beforeEach, describe, expect, it } from 'vitest';
import {
  getRoomSidebarPanelState,
  setRoomSidebarPanelState
} from '$lib/storage/roomSidebarPanel';
import { RoomSidebarPanelsState } from './roomSidebarPanels.svelte';

describe('RoomSidebarPanelsState', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('persists desktop panel changes and closes per room', () => {
    const sidebar = new RoomSidebarPanelsState(() => 'server-a', () => 'room-1');

    sidebar.toggleDesktopPanel('files');

    expect(sidebar.activeDesktopPanel).toBe('files');
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('files');

    sidebar.toggleDesktopPanel('files');

    expect(sidebar.activeDesktopPanel).toBeNull();
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBeNull();
  });

  it('does not let mobile overlay selection overwrite a persisted desktop close', () => {
    setRoomSidebarPanelState('server-a', 'room-1', null);
    const sidebar = new RoomSidebarPanelsState(() => 'server-a', () => 'room-1');

    expect(sidebar.activeDesktopPanel).toBeNull();

    sidebar.toggleMobilePanel('files');

    expect(sidebar.mobilePanel).toBe('files');
    expect(sidebar.activeDesktopPanel).toBeNull();
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBeNull();
  });

  it('does not let mobile overlay selection overwrite a persisted desktop panel', () => {
    setRoomSidebarPanelState('server-a', 'room-1', 'files');
    const sidebar = new RoomSidebarPanelsState(() => 'server-a', () => 'room-1');

    sidebar.toggleMobilePanel('members');

    expect(sidebar.mobilePanel).toBe('members');
    expect(sidebar.activeDesktopPanel).toBe('files');
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('files');
  });

  it('treats mobile overlay state as closed after the room changes', () => {
    let roomId = 'room-1';
    const sidebar = new RoomSidebarPanelsState(() => 'server-a', () => roomId);

    sidebar.toggleMobilePanel('files');
    expect(sidebar.mobilePanel).toBe('files');

    roomId = 'room-2';
    expect(sidebar.mobilePanel).toBeNull();
  });
});
