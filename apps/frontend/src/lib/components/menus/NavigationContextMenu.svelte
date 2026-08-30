<!--
@component

Shared actions shown when a server icon or room row is right-clicked or long-pressed.
The parent owns membership, read, configuration, and leave behavior so this component stays
presentation-only.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import MenuItem from '$lib/ui/MenuItem.svelte';
  import MenuSection from '$lib/ui/MenuSection.svelte';

  let {
    kind,
    isRoomMember = true,
    canJoin = false,
    showMarkRead = true,
    canMarkRead,
    canConfigure = false,
    canLeave = true,
    onJoin = () => {},
    onMarkRead,
    onConfigure,
    onLeave
  }: {
    kind: 'server' | 'room';
    isRoomMember?: boolean;
    canJoin?: boolean;
    showMarkRead?: boolean;
    canMarkRead: boolean;
    canConfigure?: boolean;
    canLeave?: boolean;
    onJoin?: () => void;
    onMarkRead: () => void;
    onConfigure?: () => void;
    onLeave: () => void;
  } = $props();
</script>

{#if (kind === 'room' && !isRoomMember) || showMarkRead || (canConfigure && onConfigure)}
  <MenuSection>
    {#if kind === 'room' && !isRoomMember}
      <MenuItem icon="icon-[uil--sign-in-alt]" onclick={onJoin} disabled={!canJoin}>
        {m('room.join.action')}
      </MenuItem>
    {:else if showMarkRead}
      <MenuItem icon="icon-[uil--check-circle]" onclick={onMarkRead} disabled={!canMarkRead}>
        {m('room_list.mark_as_read')}
      </MenuItem>
    {/if}

    {#if canConfigure && onConfigure}
      <MenuItem icon="icon-[uil--setting]" onclick={onConfigure}>
        {m('room_list.room_settings')}
      </MenuItem>
    {/if}
  </MenuSection>
{/if}

{#if isRoomMember && canLeave}
  <MenuSection>
    <MenuItem
      icon={kind === 'server' ? 'icon-[uil--minus-circle]' : 'icon-[uil--sign-out-alt]'}
      tone="danger"
      onclick={onLeave}
    >
      {kind === 'server' ? m('room_list.remove_server') : m('room_list.leave_room')}
    </MenuItem>
  </MenuSection>
{/if}
