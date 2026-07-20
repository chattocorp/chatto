import { describe, expect, it } from 'vitest';
import {
  canBanMembersFromRoomSidebar,
  CHANNEL_ROOM_SIDEBAR_PANELS,
  DM_ROOM_SIDEBAR_PANELS,
  roomSidebarPanelForRoom,
  roomSidebarPanelsForRoom,
  visibleRoomSidebarPanel
} from './roomSidebarBehavior';

describe('room sidebar behavior', () => {
  it('allows channel member bans when the room capability is present', () => {
    expect(canBanMembersFromRoomSidebar(false, true)).toBe(true);
  });

  it('suppresses member bans for DM rooms even if stale capability data says otherwise', () => {
    expect(canBanMembersFromRoomSidebar(true, true)).toBe(false);
  });

  it('suppresses member bans when the room capability is absent', () => {
    expect(canBanMembersFromRoomSidebar(false, false)).toBe(false);
    expect(canBanMembersFromRoomSidebar(false, null)).toBe(false);
    expect(canBanMembersFromRoomSidebar(false, undefined)).toBe(false);
  });

  it('exposes files and calls as DM room sidebar panels', () => {
    expect(DM_ROOM_SIDEBAR_PANELS).toEqual(['files', 'call']);
  });

  it('keeps channel room sidebar panels unchanged', () => {
    expect(roomSidebarPanelForRoom(false, 'members')).toBe('members');
    expect(roomSidebarPanelForRoom(false, 'files')).toBe('files');
    expect(roomSidebarPanelForRoom(false, 'call')).toBe('call');
    expect(roomSidebarPanelForRoom(false, null)).toBeNull();
  });

  it('treats the members default as closed for DM rooms', () => {
    expect(roomSidebarPanelForRoom(true, 'members')).toBeNull();
    expect(roomSidebarPanelForRoom(true, null)).toBeNull();
  });

  it('allows the files panel to open for DM rooms', () => {
    expect(roomSidebarPanelForRoom(true, 'files')).toBe('files');
  });

  it('allows the call panel to open for DM rooms when LiveKit is configured', () => {
    expect(roomSidebarPanelForRoom(true, 'call', true)).toBe('call');
  });

  it('hides the call panel when LiveKit is not configured', () => {
    expect(roomSidebarPanelForRoom(false, 'call', false)).toBeNull();
    expect(roomSidebarPanelForRoom(true, 'call', false)).toBeNull();
    expect(roomSidebarPanelsForRoom(false, false)).toEqual(['members', 'files']);
    expect(roomSidebarPanelsForRoom(true, false)).toEqual(['files']);
  });

  it('returns all channel panels when LiveKit is configured', () => {
    expect(CHANNEL_ROOM_SIDEBAR_PANELS).toEqual(['members', 'files', 'call']);
    expect(roomSidebarPanelsForRoom(false, true)).toEqual(['members', 'files', 'call']);
  });

  it('uses only the desktop sidebar selection on desktop', () => {
    expect(visibleRoomSidebarPanel(true, 'files', null)).toBe('files');
    expect(visibleRoomSidebarPanel(true, null, 'files')).toBeNull();
  });

  it('uses only the mobile sidebar selection on mobile', () => {
    expect(visibleRoomSidebarPanel(false, null, 'files')).toBe('files');
    expect(visibleRoomSidebarPanel(false, 'files', null)).toBeNull();
  });
});
