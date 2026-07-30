import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import type { RoomMember } from '$lib/state/room';
import { MessageUserInteractionState } from './messageUserInteractions.svelte';

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    getAPI: vi.fn()
  })
}));

import MessageUserOverlays from './MessageUserOverlays.svelte';

const member: RoomMember = {
  id: 'user-1',
  login: 'alice',
  displayName: 'Alice',
  deleted: false,
  avatarUrl: null,
  customStatus: null,
  presenceStatus: PresenceStatus.ONLINE
};

describe('MessageUserOverlays', () => {
  it('can dismiss the user menu while its deferred module is still loading', async () => {
    const interactions = new MessageUserInteractionState(() => [member]);
    interactions.showUser(member, new DOMRect(20, 20, 40, 40));

    render(MessageUserOverlays, {
      props: {
        interactions,
        serverId: 'server-1',
        roomId: 'room-1',
        currentUserId: 'current-user',
        canStartDMs: true,
        canBanRoomMembers: true,
        userContextMenuLoader: () => new Promise(() => {})
      }
    });

    await vi.waitFor(() => {
      expect(document.querySelector('[aria-busy="true"]')).not.toBeNull();
    });

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));

    await vi.waitFor(() => {
      expect(interactions.user).toBeNull();
      expect(interactions.anchorRect).toBeNull();
    });
  });
});
