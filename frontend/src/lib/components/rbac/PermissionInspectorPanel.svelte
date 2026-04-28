<script lang="ts">
  import { useConnection } from '$lib/state/instance/connection.svelte';
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
    spaceId?: string | null;
    roomId?: string | null;
  };

  let { userId, spaceId = null, roomId = null }: Props = $props();

  const connection = useConnection();

  let explanations = $state<Explanation[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  $effect(() => {
    const currentUserId = userId;
    const currentSpaceId = spaceId ?? null;
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
          query PermissionInspector($userId: ID!, $spaceId: ID, $roomId: ID) {
            permissionExplanation(userId: $userId, spaceId: $spaceId, roomId: $roomId) {
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
        { userId: currentUserId, spaceId: currentSpaceId, roomId: currentRoomId }
      )
      .toPromise()
      .then((result) => {
        if (
          currentUserId !== userId ||
          currentSpaceId !== (spaceId ?? null) ||
          currentRoomId !== (roomId ?? null)
        ) {
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
  <Hint variant="danger">{error}</Hint>
{:else if explanations.length === 0}
  <div class="text-muted italic">No applicable permissions at this scope.</div>
{:else}
  <PermissionExplanationTable {explanations} />
{/if}
