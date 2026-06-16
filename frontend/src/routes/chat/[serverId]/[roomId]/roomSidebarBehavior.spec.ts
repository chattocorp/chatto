import { describe, expect, it } from 'vitest';
import {
  canBanMembersFromRoomSidebar,
  DM_ROOM_SIDEBAR_PANELS,
  roomSidebarPanelForRoom
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

  it('only exposes files as a DM room sidebar panel', () => {
    expect(DM_ROOM_SIDEBAR_PANELS).toEqual(['files']);
  });

  it('keeps channel room sidebar panels unchanged', () => {
    expect(roomSidebarPanelForRoom(false, 'members')).toBe('members');
    expect(roomSidebarPanelForRoom(false, 'files')).toBe('files');
    expect(roomSidebarPanelForRoom(false, null)).toBeNull();
  });

  it('treats the members default as closed for DM rooms', () => {
    expect(roomSidebarPanelForRoom(true, 'members')).toBeNull();
    expect(roomSidebarPanelForRoom(true, null)).toBeNull();
  });

  it('allows the files panel to open for DM rooms', () => {
    expect(roomSidebarPanelForRoom(true, 'files')).toBe('files');
  });
});
