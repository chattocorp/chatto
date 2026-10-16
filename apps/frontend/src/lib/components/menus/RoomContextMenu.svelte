<!--
@component

Actions shown when a room row is right-clicked or long-pressed.
The parent owns membership, read, configuration, and leave behavior so this component stays
presentation-only.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { MenuItem, MenuSection } from '$lib/ui';

  let {
    isRoomMember = true,
    canJoin = false,
    canMarkRead,
    canConfigure = false,
    canLeave = true,
    onJoin = () => {},
    onMarkRead,
    onConfigure,
    onLeave
  }: {
    isRoomMember?: boolean;
    canJoin?: boolean;
    canMarkRead: boolean;
    canConfigure?: boolean;
    canLeave?: boolean;
    onJoin?: () => void;
    onMarkRead: () => void;
    onConfigure?: () => void;
    onLeave: () => void;
  } = $props();
</script>

<MenuSection>
  {#if !isRoomMember}
    <MenuItem icon="icon-[uil--sign-in-alt]" mirrorIconInRtl onclick={onJoin} disabled={!canJoin}>
      {m('room.join.action')}
    </MenuItem>
  {:else}
    <MenuItem icon="icon-[uil--check-circle]" onclick={onMarkRead} disabled={!canMarkRead}>
      {m('room_list.mark_as_read')}
    </MenuItem>
  {/if}

  {#if canConfigure && onConfigure}
    <MenuItem icon="icon-[uil--setting]" onclick={onConfigure}>
      {m('room_list.open_room_settings')}
    </MenuItem>
  {/if}
</MenuSection>

{#if isRoomMember && canLeave}
  <MenuSection>
    <MenuItem icon="icon-[uil--sign-out-alt]" mirrorIconInRtl tone="danger" onclick={onLeave}>
      {m('room_list.leave_room')}
    </MenuItem>
  </MenuSection>
{/if}
