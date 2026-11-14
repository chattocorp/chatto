<!--
@component

The server menu: the server's name, host, and problem, and the actions on that
server. The Server Gutter icon and the Server Sidebar header open it. Pass
gutter-only actions, such as reordering, in `ordering`.
-->
<script lang="ts">
  import { pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { serverRegistry } from '$lib/client';
  import { copyServerLinkToClipboard } from '$lib/copyChatLink';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { markNavigationServerAsRead } from '$lib/navigation/readActions';
  import { beginOriginReauthentication } from '$lib/auth/reauth';
  import { isRemoteSignInPending, startRemoteSignIn } from '$lib/auth/remoteSignIn.svelte';
  import { isSigningOut, signOutOfServer } from '$lib/auth/serverSignOut.svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import type { ServerStatus } from '$lib/state/server/serverStatus.svelte';
  import { ContextMenu, MenuItem, MenuSection } from '$lib/ui';
  import { copyToClipboard } from '$lib/utils/copyToClipboard';

  let {
    status,
    position,
    presentation = 'auto',
    focusFirstItem = false,
    ordering,
    onclose
  }: {
    status: ServerStatus;
    /** Viewport point where the menu opens. */
    position?: { x: number; y: number; alignRight?: boolean };
    presentation?: 'auto' | 'floating' | 'sheet';
    /** Move focus to the first item after opening, for keyboard users. */
    focusFirstItem?: boolean;
    /** Optional section after the server actions, such as gutter reordering. */
    ordering?: Snippet;
    onclose: () => void;
  } = $props();

  const serverId = $derived(status.serverId);
  const serverSegment = $derived(serverIdToSegment(serverId));
  const stores = $derived(status.stores);
  const host = $derived(status.host);
  const problem = $derived(status.problem);

  /** Move keyboard focus into the menu after its popover reaches the top layer. */
  const focusMenu: Attachment<HTMLElement> = (node) => {
    if (!focusFirstItem) return;
    const frame = requestAnimationFrame(() =>
      node
        .closest('[role="menu"]')
        ?.querySelector<HTMLElement>('a[href], button:not(:disabled)')
        ?.focus()
    );
    return () => cancelAnimationFrame(frame);
  };

  function handleMarkServerRead(): void {
    onclose();
    void markNavigationServerAsRead(serverId);
  }

  async function handleCopyServerHostname(): Promise<void> {
    const hostname = host;
    onclose();
    if (hostname) await copyToClipboard(hostname);
  }

  function handleRemoveServer(): void {
    if (status.isOrigin) return;
    onclose();
    pushState('', { modal: { type: 'removeServer', serverId, spaceName: status.name } });
  }

  function handleSignIn(): void {
    const server = status.registeredServer;
    if (!server) return;
    onclose();
    if (status.isOrigin) {
      beginOriginReauthentication(resolve('/chat/[serverId]', { serverId: serverSegment }));
      return;
    }
    void startRemoteSignIn(server);
  }

  function handleSignOut(): void {
    if (!stores.isAuthenticated) return;
    onclose();
    void signOutOfServer(serverId);
  }
</script>

<ContextMenu
  {position}
  {presentation}
  ariaLabel={m('room_list.server_actions', { server: status.name })}
  class="w-80 max-w-[calc(100vw-1rem)]"
  {onclose}
>
  <div
    class="menu-section px-3 py-2 text-sm"
    role="presentation"
    data-testid="server-compatibility-section"
    {@attach focusMenu}
  >
    <div class="truncate font-medium text-text" data-testid="server-name">
      {status.name}
    </div>
    {#if host}
      <div
        class="mt-0.5 truncate text-muted"
        title={status.registeredServer?.url}
        data-testid="server-hostname"
      >
        {host}
      </div>
    {/if}
    {#if problem}
      <div
        class="mt-1 flex items-start gap-1.5 whitespace-normal text-warning"
        data-testid="server-problem-message"
      >
        <span class="iconify mt-0.5 icon-[uil--exclamation-circle] shrink-0" aria-hidden="true"
        ></span>
        <span>{problem}</span>
      </div>
    {/if}
  </div>

  <MenuSection>
    <MenuItem
      icon="icon-[uil--info-circle]"
      href={resolve('/chat/[serverId]/info', { serverId: serverSegment })}
      onclick={onclose}
    >
      {m('chat.server_info.title')}
    </MenuItem>
    {#if host}
      <MenuItem
        icon="icon-[uil--link]"
        dataTestid="copy-server-link"
        onclick={() => {
          void copyServerLinkToClipboard(serverId);
          onclose();
        }}
      >
        {m('room_list.copy_server_link')}
      </MenuItem>
      <MenuItem
        icon="icon-[uil--copy]"
        onclick={() => void handleCopyServerHostname()}
        dataTestid="copy-server-hostname"
      >
        {m('room_list.copy_server_hostname')}
      </MenuItem>
    {/if}
    {#if status.serverActionsAvailable}
      <MenuItem
        icon="icon-[uil--check-circle]"
        onclick={handleMarkServerRead}
        disabled={!serverUi(stores).roomUnread.hasAnyUnread &&
          stores.notifications.unreadNotificationCount === 0}
      >
        {m('room_list.mark_as_read')}
      </MenuItem>
    {/if}
  </MenuSection>
  {@render ordering?.()}
  {#if status.recoveryNeeded || status.signInRequired || stores.isAuthenticated || !status.isOrigin}
    <MenuSection>
      {#if status.recoveryNeeded}
        <MenuItem
          icon="icon-[uil--redo]"
          mirrorIconInRtl
          onclick={() => {
            onclose();
            void serverRegistry.recoverServer(serverId);
          }}>{m('common.retry')}</MenuItem
        >
      {/if}
      {#if status.signInRequired}
        <MenuItem
          icon="icon-[uil--sign-in-alt]"
          mirrorIconInRtl
          onclick={handleSignIn}
          disabled={isRemoteSignInPending(serverId)}
          dataTestid="server-log-in"
        >
          {m('chat.server_gutter.log_in')}
        </MenuItem>
      {:else if stores.isAuthenticated}
        <MenuItem
          icon="icon-[uil--sign-out-alt]"
          mirrorIconInRtl
          onclick={handleSignOut}
          disabled={isSigningOut(serverId)}
          dataTestid="server-sign-out"
        >
          {m('chat.server_gutter.sign_out')}
        </MenuItem>
      {/if}
      {#if !status.isOrigin}
        <MenuItem icon="icon-[uil--minus-circle]" tone="danger" onclick={handleRemoveServer}>
          {m('room_list.remove_server')}
        </MenuItem>
      {/if}
    </MenuSection>
  {/if}
</ContextMenu>
