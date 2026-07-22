<!-- SPDX-License-Identifier: Apache-2.0 -->
<!--
@component

Shared bot permission editor for owner settings and Server Admin. It exposes
the bot's direct decisions while preserving the owner's permission ceiling.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { createBotAPI, type BotPermissionMatrix } from '$lib/api-client/bots';
  import SubjectPermissionsMatrix, {
    type CellState,
    type MatrixData,
    type MatrixScope
  } from '$lib/components/rbac/SubjectPermissionsMatrix.svelte';
  import { Hint } from '$lib/ui';
  import * as m from '$lib/i18n/messages';

  let { botId }: { botId: string } = $props();

  const connection = useConnection();
  let matrix = $state<BotPermissionMatrix | null>(null);
  let error = $state<string | null>(null);
  let updatingKey = $state<string | null>(null);

  const data = $derived<MatrixData | null>(
    matrix
      ? {
          applicablePermissions: matrix.applicablePermissions,
          scopes: matrix.scopes,
          cells: matrix.cells.map((cell) => ({
            permission: cell.permission,
            scopeId: cell.scopeId,
            override: cell.directDecision,
            effective: cell.effectiveDecision,
            canAllow: cell.ownerAllowed
          }))
        }
      : null
  );

  function api() {
    const conn = connection();
    return createBotAPI({ baseUrl: conn.connectBaseUrl, bearerToken: conn.bearerToken });
  }

  async function load() {
    error = null;
    try {
      matrix = await api().getPermissionMatrix(botId);
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause);
    }
  }

  async function cycle(scope: MatrixScope, permission: string, next: CellState) {
    const key = `${scope.id}::${permission}`;
    updatingKey = key;
    error = null;
    try {
      await api().setPermission({
        botId,
        permission,
        scope,
        decision: next === 'allow' ? 'ALLOW' : next === 'deny' ? 'DENY' : 'NONE'
      });
      matrix = await api().getPermissionMatrix(botId);
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause);
    } finally {
      updatingKey = null;
    }
  }

  onMount(() => void load());
</script>

{#if error}
  <Hint tone="danger">{error}</Hint>
{/if}

{#if data}
  <SubjectPermissionsMatrix {data} {updatingKey} subjectKind="bot" onCycle={cycle} />
{:else if !error}
  <div class="text-muted">{m['rbac.permissions.loading']()}</div>
{/if}
