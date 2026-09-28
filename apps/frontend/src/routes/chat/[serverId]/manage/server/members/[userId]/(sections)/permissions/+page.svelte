<!--
@component

Permissions section of a human member: the direct permission grants.
-->
<script lang="ts">
  import { UserPermissionsMatrix } from '$lib/components/rbac';
  import { m } from '$lib/i18n/messages';
  import { Hint } from '$lib/ui';
  import { useMemberDetail } from '../../memberDetailContext';

  const detail = useMemberDetail();
</script>

{#if !detail.isBot && detail.details.viewerCanManageUserPermissions}
  <Hint>{m('admin.permissions.resolution_hint')}</Hint>
  <UserPermissionsMatrix userId={detail.userId} />
{:else}
  <Hint tone="danger">{m('ui.access_denied.message')}</Hint>
{/if}
