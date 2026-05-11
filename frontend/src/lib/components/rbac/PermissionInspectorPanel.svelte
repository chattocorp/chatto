<script lang="ts">
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { graphql } from '$lib/gql';
  import { Hint } from '$lib/ui';
  import PermissionExplanationTable from './PermissionExplanationTable.svelte';

  type DecisionKind = 'ALLOW' | 'DENY' | 'NONE';
  type Level = 'INSTANCE' | 'SPACE' | 'ROOM';

  type Explanation = {
    permission: string;
    state: DecisionKind;
    decidedAt?: Level | null;
    decidedByRole?: string | null;
    trace: { level: Level; roleName: string; decision: DecisionKind; applied: boolean }[];
  };

  type Props = {
    userId: string;
    roomId?: string | null;
  };

  let { userId, roomId = null }: Props = $props();

  const connection = useConnection();

  let explanations = $state<Explanation[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  $effect(() => {
    const currentUserId = userId;
    const currentRoomId = roomId ?? null;

    if (!currentUserId) {
      explanations = [];
      loading = false;
      error = null;
      return;
    }

    loading = true;
    error = null;

    connection()
      .client.query(
        graphql(`
          query PermissionInspector($userId: ID!, $roomId: ID) {
            permissionExplanation(userId: $userId, roomId: $roomId) {
              permission
              state
              decidedAt
              decidedByRole
              trace {
                level
                roleName
                decision
                applied
              }
            }
          }
        `),
        { userId: currentUserId, roomId: currentRoomId ?? undefined }
      )
      .toPromise()
      .then((result) => {
        if (currentUserId !== userId || currentRoomId !== (roomId ?? null)) {
          return;
        }

        if (result.error) {
          error = result.error.message;
          explanations = [];
        } else if (result.data?.permissionExplanation) {
          explanations = result.data.permissionExplanation as Explanation[];
        }
        loading = false;
      });
  });
</script>

{#if loading}
  <div class="text-muted">Loading permissions...</div>
{:else if error}
  <Hint tone="danger">{error}</Hint>
{:else if explanations.length === 0}
  <div class="text-muted italic">No applicable permissions at this scope.</div>
{:else}
  <PermissionExplanationTable {explanations} />
{/if}
