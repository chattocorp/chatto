<script lang="ts">
  import DMConversationList from '$lib/dm/DMConversationList.svelte';
  import SecondarySidebar from '$lib/components/SecondarySidebar.svelte';
  import { PaneHeader } from '$lib/ui';
  import { getInstancePermissions } from '$lib/state/instance/permissions.svelte';
  import AccessDenied from '$lib/ui/AccessDenied.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { DMConversationsStore, setDMConversationsStore } from '$lib/state/dm/conversations.svelte';

  let { data, children } = $props();

  // Check origin instance permissions (gate the entire DM inbox).
  // When permissions haven't loaded (origin unauthenticated, or still loading),
  // default to allowing access — per-instance DM routes handle their own auth.
  const instancePerms = getInstancePermissions();
  let canViewDMs = $derived(
    !instancePerms.current.loaded ? true : instancePerms.current.canViewDMs
  );

  // Single store for the cross-instance DM list, available to children via
  // context. The $effect reads `instanceRegistry.instances` (passed directly
  // to wireSubscriptions, and read internally by loadAll), so adding or
  // disconnecting an instance triggers a refetch + subscription rewire.
  const dmStore = new DMConversationsStore();
  setDMConversationsStore(dmStore);
  $effect(() => {
    void dmStore.loadAll();
    return dmStore.wireSubscriptions(instanceRegistry.instances, () => data.conversationId);
  });
</script>

{#if !canViewDMs}
  <AccessDenied message="You do not have permission to access Direct Messages." />
{:else}
  <SecondarySidebar width="md:w-80" mobileWidth="max-md:w-80">
    <PaneHeader title="Direct Messages" />
    <DMConversationList activeConversationId={data.conversationId} />
  </SecondarySidebar>

  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    {@render children?.()}
  </div>
{/if}
