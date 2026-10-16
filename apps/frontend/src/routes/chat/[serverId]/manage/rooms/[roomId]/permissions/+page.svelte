<!--
@component

Permissions section of a room: the role permissions that apply in the room.
-->
<script lang="ts">
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import { m } from '$lib/i18n/messages';
  import { Hint } from '$lib/ui';
  import { useRoomManagement } from '../roomManagementContext';

  const detail = useRoomManagement();
</script>

<div class="flex min-h-0 flex-1 flex-col gap-4">
  <Hint>{m('admin.rooms_admin.room_permissions_hint')}</Hint>
  <PermissionMatrix
    roomId={detail.roomId}
    subtitle={m('admin.rooms_admin.room_permissions_subtitle')}
    fillHeight
  />
</div>
