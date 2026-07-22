<!--
@component

Per-role adapter for the shared managed permission matrix.
-->
<script lang="ts">
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { createPermissionAPI } from '$lib/api-client/permissions';
  import {
    setRolePermission,
    type MutationScope,
    type PermissionState
  } from './permissionMutations';
  import ManagedSubjectPermissionsMatrix from './ManagedSubjectPermissionsMatrix.svelte';
  import type { CellState, MatrixData, MatrixScope } from './SubjectPermissionsMatrix.svelte';
  import * as m from '$lib/i18n/messages';

  let { roleName }: { roleName: string } = $props();

  const connection = useConnection();
  const isOwnerRole = $derived(roleName === 'owner');

  function api() {
    const conn = connection();
    return createPermissionAPI({ baseUrl: conn.connectBaseUrl, bearerToken: conn.bearerToken });
  }

  function mutationScope(scope: MatrixScope, targetRoleName: string): MutationScope {
    if (scope.kind === 'GROUP') {
      return {
        tier: 'group',
        roleName: targetRoleName,
        groupId: scope.id.replace(/^group:/, '')
      };
    }
    if (scope.kind === 'ROOM') {
      return {
        tier: 'room',
        roleName: targetRoleName,
        roomId: scope.id.replace(/^room:/, '')
      };
    }
    return { tier: 'server', roleName: targetRoleName };
  }

  async function load(targetRoleName: string): Promise<MatrixData | null> {
    return api().getRolePermissionMatrix(targetRoleName);
  }

  async function mutate(
    targetRoleName: string,
    scope: MatrixScope,
    permission: string,
    next: CellState
  ) {
    const result = await setRolePermission(
      api(),
      mutationScope(scope, targetRoleName),
      permission,
      next as PermissionState
    );
    if (result.error) throw new Error(result.error);
  }
</script>

<ManagedSubjectPermissionsMatrix
  resourceKey={roleName}
  {load}
  {mutate}
  subjectKind="role"
  forceAllow={isOwnerRole}
  readOnly={isOwnerRole}
  emptyMessage={m['admin.permissions.role_not_found']()}
  toastErrors
/>
