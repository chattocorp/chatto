<!-- @component Role permissions and inheritance help for the current room group. -->
<script lang="ts">
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import { Hint } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { useRoomGroupManagement } from '../roomGroupManagementContext';
  const detail = useRoomGroupManagement();
</script>

<div class="flex min-h-0 flex-1 flex-col gap-4">
  <Hint>{m('admin.rooms_admin.group_permissions_hint')}</Hint>
  <PermissionMatrix
    groupId={detail.groupId}
    subtitle={m('admin.rooms_admin.group_permissions_subtitle')}
    fillHeight
  />
</div>
