<!--
@component

Per-user permission matrix loader. Owns the GraphQL query for the user's
matrix and the mutation dispatch for cell clicks; delegates rendering to
`SubjectPermissionsMatrix`.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import { Hint } from '$lib/ui';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { graphql } from '$lib/gql';
  import { toast } from '$lib/ui/toast';
  import * as m from '$lib/i18n/messages';
  import {
    setUserPermission,
    type UserMutationScope,
    type UserPermissionState
  } from './userPermissionMutations';
  import SubjectPermissionsMatrix, {
    type MatrixData,
    type MatrixScope,
    type CellState
  } from './SubjectPermissionsMatrix.svelte';

  type Matrix = MatrixData & { userId: string };

  let { userId }: { userId: string } = $props();

  const connection = useConnection();

  let data = $state<Matrix | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let updatingKey = $state<string | null>(null);

  $effect(() => {
    void load(userId);
  });

  async function load(uid: string) {
    // Only show the loading state on the initial load; refreshes after a
    // mutation keep the existing matrix visible so the page doesn't flash
    // a blank panel between request and response.
    //
    // Wrap the `data` read in `untrack` so the caller `$effect` doesn't
    // subscribe to it — otherwise every assignment below would re-fire
    // the effect and loop.
    const current = untrack(() => data);
    if (!current || current.userId !== uid) loading = true;
    error = null;

    const resp = await connection().client.query(
      graphql(`
        query UserPermissionsMatrixQuery($userId: ID!) {
          admin {
            rbac {
              userPermissionMatrix(userId: $userId) {
                userId
                applicablePermissions
                scopes {
                  id
                  label
                  kind
                  parentGroupId
                }
                cells {
                  permission
                  scopeId
                  override
                  effective
                }
              }
            }
          }
        }
      `),
      { userId: uid },
      { requestPolicy: 'network-only' }
    );

    if (uid !== userId) return;

    loading = false;
    if (resp.error) {
      error = resp.error.message;
      return;
    }
    const matrix = resp.data?.admin?.rbac.userPermissionMatrix;
    if (!matrix) {
      error = 'No data returned';
      return;
    }
    const m = matrix;
    data = {
      userId: m.userId,
      applicablePermissions: [...m.applicablePermissions],
      scopes: m.scopes.map((s) => ({ ...s })),
      cells: m.cells.map((c) => ({ ...c }))
    };
  }

  function mutationScopeFor(scope: MatrixScope): UserMutationScope {
    if (scope.kind === 'GROUP') {
      const groupId = scope.id.startsWith('group:') ? scope.id.slice('group:'.length) : '';
      return { tier: 'group', groupId };
    }
    if (scope.kind === 'ROOM') {
      const roomId = scope.id.startsWith('room:') ? scope.id.slice('room:'.length) : '';
      return { tier: 'room', roomId };
    }
    return { tier: 'server' };
  }

  async function handleCycle(scope: MatrixScope, permission: string, next: CellState) {
    if (!data) return;
    const cellKey = `${scope.id}::${permission}`;
    updatingKey = cellKey;
    error = null;

    const result = await setUserPermission(
      connection().client,
      data.userId,
      mutationScopeFor(scope),
      permission,
      next as UserPermissionState
    );
    if (result.error) {
      error = result.error;
      toast.error(result.error);
      updatingKey = null;
      return;
    }

    // Reload the matrix so both the override AND effective decisions stay
    // consistent — a server-scope grant flows into rooms via inheritance.
    await load(data.userId);
    updatingKey = null;
  }
</script>

{#if error}
  <Hint tone="danger">{error}</Hint>
{/if}

{#if loading}
  <div class="text-muted">{m['rbac.permissions.loading']()}</div>
{:else if !data}
  <Hint tone="info">{m['rbac.permissions.no_data']()}</Hint>
{:else}
  <SubjectPermissionsMatrix {data} {updatingKey} onCycle={handleCycle} subjectKind="user" />
{/if}
