<script lang="ts">
  import DMConversationList from '$lib/dm/DMConversationList.svelte';
  import SecondarySidebar from '$lib/components/SecondarySidebar.svelte';
  import { PaneHeader } from '$lib/ui';
  import { getInstancePermissions } from '$lib/state/instance/permissions.svelte';
  import AccessDenied from '$lib/ui/AccessDenied.svelte';
  import { DMConversationsStore, setDMConversationsStore } from '$lib/state/dm/conversations.svelte';

  let { data, children } = $props();

  // Check origin instance permissions (gate the entire DM inbox).
  // When permissions haven't loaded (origin unauthenticated, or still loading),
  // default to allowing access — per-instance DM routes handle their own auth.
  const instancePerms = getInstancePermissions();
  let canViewDMs = $derived(
    !instancePerms.current.loaded ? true : instancePerms.current.canViewDMs
  );

  // Single store for the cross-instance DM list. Synchronously available to
  // children via context. Subscriptions wire up in $effect; the store reads
  // the active conversation id reactively via the getter.
  const dmStore = new DMConversationsStore();
  setDMConversationsStore(dmStore);
  $effect(() => dmStore.start(() => data.conversationId));
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
