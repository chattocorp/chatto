<!--
@component

Actions shown when a room row is right-clicked or long-pressed.
Room actions precede copy actions, with archive and leave actions at the bottom.
The parent owns all action behavior so this component stays presentation-only.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { MenuItem, MenuSection } from '$lib/ui';

  let {
    isRoomMember = true,
    canJoin = false,
    joinLabel = m('room.join.action'),
    canInvite = false,
    onInvite,
    canMarkRead,
    canConfigure = false,
    canLeave = true,
    onJoin = () => {},
    onMarkRead,
    onConfigure,
    onLeave,
    onCopyLink,
    onCopyId,
    onArchive
  }: {
    isRoomMember?: boolean;
    canJoin?: boolean;
    joinLabel?: string;
    canInvite?: boolean;
    onInvite?: () => void;
    canMarkRead: boolean;
    canConfigure?: boolean;
    canLeave?: boolean;
    onJoin?: () => void;
    onMarkRead: () => void;
    onConfigure?: () => void;
    onLeave: () => void;
    onCopyLink: () => void;
    onCopyId: () => void;
    onArchive: () => void;
  } = $props();
</script>

<MenuSection>
  {#if !isRoomMember}
    <MenuItem icon="icon-[uil--sign-in-alt]" mirrorIconInRtl onclick={onJoin} disabled={!canJoin}>
      {joinLabel}
    </MenuItem>
  {/if}
  {#if canInvite && onInvite}
    <MenuItem icon="icon-[uil--user-plus]" onclick={onInvite}>
      {m('room.invitation.invite')}
    </MenuItem>
  {/if}
  {#if isRoomMember}
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

<MenuSection>
  <MenuItem icon="icon-[uil--link]" onclick={onCopyLink} dataTestid="copy-room-link">
    {m('room_list.copy_room_link')}
  </MenuItem>
  <MenuItem icon="icon-[uil--copy]" onclick={onCopyId} dataTestid="copy-room-id">
    {m('room_list.copy_room_id')}
  </MenuItem>
</MenuSection>

{#if canConfigure || (isRoomMember && canLeave)}
  <MenuSection>
    {#if canConfigure}
      <MenuItem icon="icon-[uil--archive]" onclick={onArchive}>
        {m('room.management.archive_room')}
      </MenuItem>
    {/if}
    {#if isRoomMember && canLeave}
      <MenuItem icon="icon-[uil--sign-out-alt]" mirrorIconInRtl tone="danger" onclick={onLeave}>
        {m('room_list.leave_room')}
      </MenuItem>
    {/if}
  </MenuSection>
{/if}
