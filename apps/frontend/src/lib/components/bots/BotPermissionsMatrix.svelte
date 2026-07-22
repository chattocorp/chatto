<!-- SPDX-License-Identifier: Apache-2.0 -->
<!--
@component

Shared bot permission editor for owner settings and Server Admin. It exposes
the bot's direct decisions while preserving the owner's permission ceiling.
-->
<script lang="ts">
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { createBotAPI } from '$lib/api-client/bots';
  import ManagedSubjectPermissionsMatrix from '$lib/components/rbac/ManagedSubjectPermissionsMatrix.svelte';
  import type {
    CellState,
    MatrixData,
    MatrixScope
  } from '$lib/components/rbac/SubjectPermissionsMatrix.svelte';

  let { botId }: { botId: string } = $props();

  const connection = useConnection();

  function api() {
    const conn = connection();
    return createBotAPI({ baseUrl: conn.connectBaseUrl, bearerToken: conn.bearerToken });
  }

  async function load(targetBotId: string): Promise<MatrixData> {
    const matrix = await api().getPermissionMatrix(targetBotId);
    return {
      applicablePermissions: matrix.applicablePermissions,
      scopes: matrix.scopes,
      cells: matrix.cells.map((cell) => ({
        permission: cell.permission,
        scopeId: cell.scopeId,
        override: cell.directDecision,
        effective: cell.effectiveDecision,
        canAllow: cell.ownerAllowed
      }))
    };
  }

  async function mutate(targetBotId: string, scope: MatrixScope, permission: string, next: CellState) {
    await api().setPermission({
      botId: targetBotId,
      permission,
      scope,
      decision: next === 'allow' ? 'ALLOW' : next === 'deny' ? 'DENY' : 'NONE'
    });
  }
</script>

<ManagedSubjectPermissionsMatrix
  resourceKey={botId}
  {load}
  {mutate}
  subjectKind="bot"
  containedScroll={false}
/>
