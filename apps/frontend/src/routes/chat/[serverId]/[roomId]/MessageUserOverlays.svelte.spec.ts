import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import type { RoomMember } from '$lib/state/room';
import { MessageUserInteractionState } from './messageUserInteractions.svelte';
import UserContextMenuStub from './MessageUserOverlaysUserContextMenuStub.svelte';

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'server-1',
    store: {},
    connection: {
      getAPI: vi.fn()
    },
    isCurrent: () => true
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
    let resolveUserMenu!: (
      module: typeof import('$lib/components/menus/UserContextMenu.svelte')
    ) => void;
    const userMenuModule = new Promise<
      typeof import('$lib/components/menus/UserContextMenu.svelte')
    >((resolve) => {
      resolveUserMenu = resolve;
    });
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
        userContextMenuLoader: () => userMenuModule
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

    resolveUserMenu({
      default: UserContextMenuStub
    } as unknown as typeof import('$lib/components/menus/UserContextMenu.svelte'));
  });

  it('shows a dismissible dialog while the deferred ban modal is loading', async () => {
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
        userContextMenuLoader: async () =>
          ({
            default: UserContextMenuStub
          }) as unknown as typeof import('$lib/components/menus/UserContextMenu.svelte'),
        banRoomMemberModalLoader: () => new Promise(() => {})
      }
    });

    let banButton: HTMLButtonElement | null = null;
    await vi.waitFor(() => {
      banButton =
        Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find(
          (button) => button.textContent?.trim() === 'Ban from room'
        ) ?? null;
      expect(banButton).not.toBeNull();
    });
    banButton!.click();

    let dialog: HTMLDialogElement | null = null;
    await vi.waitFor(() => {
      dialog = document.querySelector<HTMLDialogElement>('dialog[open]');
      expect(dialog?.querySelector('[aria-busy="true"]')?.textContent).toContain('Loading');
    });

    dialog!.dispatchEvent(new Event('cancel', { cancelable: true }));

    await vi.waitFor(() => {
      expect(dialog?.open).toBe(false);
    });
  });
});
