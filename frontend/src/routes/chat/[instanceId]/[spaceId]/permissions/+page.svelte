<script lang="ts">
  import { page } from '$app/state';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import { PermissionInspectorPanel } from '$lib/components/rbac';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const currentUser = getCurrentUser();
  const spaceId = $derived(page.params.spaceId);
  const userId = $derived(currentUser.user?.id ?? '');
</script>

<PageTitle title="Your permissions" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Your permissions"
    subtitle="What you're allowed to do in this space, and which role decided each call"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if !userId || !spaceId}
      <div class="text-muted">Loading...</div>
    {:else}
      <PermissionInspectorPanel {userId} {spaceId} />
    {/if}
  </div>
</div>
