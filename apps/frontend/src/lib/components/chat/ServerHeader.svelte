<!--
@component

The Server Sidebar header. Its action button opens the server menu, so server
actions such as Sign out stay available when the Server Gutter is hidden.
-->
<script lang="ts">
  import { tick } from 'svelte';
  import ServerActionsMenu from '$lib/components/ServerActionsMenu.svelte';
  import { m } from '$lib/i18n/messages';
  import { serverStatus } from '$lib/state/server/serverStatus.svelte';
  import { HeaderIconButton, PaneHeader } from '$lib/ui';

  let { serverId, serverName }: { serverId: string; serverName: string } = $props();

  const status = $derived(serverStatus(serverId));
  let menuPosition = $state<{ x: number; y: number; alignRight: boolean } | null>(null);
  let keyboardOpened = $state(false);
  let trigger: HTMLElement | null = null;

  function openMenu(event: MouseEvent): void {
    trigger = event.currentTarget as HTMLElement;
    const rect = trigger.getBoundingClientRect();
    // A keyboard activation reports no pointer position.
    keyboardOpened = event.detail === 0;
    // Align the menu's end edge with the button, like the room list menus.
    menuPosition = { x: rect.right, y: rect.bottom + 4, alignRight: true };
  }

  function closeMenu(): void {
    const returnFocus = keyboardOpened;
    menuPosition = null;
    keyboardOpened = false;
    if (returnFocus) void tick().then(() => trigger?.focus({ preventScroll: true }));
  }
</script>

<PaneHeader title={serverName}>
  {#snippet actions()}
    <HeaderIconButton
      icon="icon-[uil--angle-down]"
      iconSize="lg"
      label={m('room_list.server_actions', { server: serverName })}
      menuExpanded={menuPosition !== null}
      onclick={openMenu}
    />
  {/snippet}
</PaneHeader>

{#if menuPosition}
  <ServerActionsMenu
    {status}
    position={menuPosition}
    focusFirstItem={keyboardOpened}
    onclose={closeMenu}
  />
{/if}
