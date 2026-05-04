<script lang="ts">
  import Room from '../../../[instanceId]/(chrome)/[roomId]/Room.svelte';
  import { DM_SPACE_ID } from '$lib/constants';
  import { setLastRoom } from '$lib/storage/lastRoom';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';

  let { data } = $props();

  const getInstanceId = getActiveInstance();

  // Remember last visited DM conversation for redirect on return
  $effect(() => {
    if (data.conversationId) {
      setLastRoom(getInstanceId(), DM_SPACE_ID, data.conversationId);
    }
  });
</script>

{#if data.conversationId}
  <Room spaceId={DM_SPACE_ID} roomId={data.conversationId} />
{:else}
  <div class="flex h-full items-center justify-center text-muted">Conversation not found</div>
{/if}
