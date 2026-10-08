<!--
@component

Permissions section of a role: its allows and denies at the server, in room
groups, in rooms, and in direct messages. A role that the viewer cannot change
shows read-only.
-->
<script lang="ts">
  import { RolePermissionsMatrix } from '$lib/components/rbac';
  import { m } from '$lib/i18n/messages';
  import { Hint } from '$lib/ui';
  import { useRoleDetail } from '../roleDetailContext';

  const detail = useRoleDetail();
</script>

<Hint>
  {#if detail.roleName === 'owner'}
    {m('admin.permissions.owner_permissions_hint')}
  {:else}
    {m('admin.permissions.role_permissions_hint')}
  {/if}
</Hint>
<RolePermissionsMatrix roleName={detail.roleName} readOnly={!detail.canEditRole} />
