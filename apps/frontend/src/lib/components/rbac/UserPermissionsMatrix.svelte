<!--
@component

Per-user adapter for the shared managed permission matrix.
-->
<script lang="ts">
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { createPermissionAPI } from '$lib/api-client/permissions';
  import {
    setUserPermission,
    type UserMutationScope,
    type UserPermissionState
  } from './userPermissionMutations';
  import ManagedSubjectPermissionsMatrix from './ManagedSubjectPermissionsMatrix.svelte';
  import type { CellState, MatrixData, MatrixScope } from './SubjectPermissionsMatrix.svelte';

  let { userId }: { userId: string } = $props();

  const connection = useConnection();

  function api() {
    const conn = connection();
    return createPermissionAPI({ baseUrl: conn.connectBaseUrl, bearerToken: conn.bearerToken });
  }

  function mutationScope(scope: MatrixScope): UserMutationScope {
    if (scope.kind === 'GROUP') {
      return { tier: 'group', groupId: scope.id.replace(/^group:/, '') };
    }
    if (scope.kind === 'ROOM') {
      return { tier: 'room', roomId: scope.id.replace(/^room:/, '') };
    }
    return { tier: 'server' };
  }

  async function load(targetUserId: string): Promise<MatrixData | null> {
    return api().getUserPermissionMatrix(targetUserId);
  }

  async function mutate(
    targetUserId: string,
    scope: MatrixScope,
    permission: string,
    next: CellState
  ) {
    const result = await setUserPermission(
      api(),
      targetUserId,
      mutationScope(scope),
      permission,
      next as UserPermissionState
    );
    if (result.error) throw new Error(result.error);
  }
</script>

<ManagedSubjectPermissionsMatrix
  resourceKey={userId}
  {load}
  {mutate}
  subjectKind="user"
  toastErrors
/>
