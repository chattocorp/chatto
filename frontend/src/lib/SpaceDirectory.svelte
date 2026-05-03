<script lang="ts">
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { toast } from '$lib/ui/toast';
  import { SpaceDirectoryStore } from '$lib/state/spaceDirectory.svelte';
  import type { SpaceCardSpaceFragment } from './gql/graphql';
  import SpaceCard from './components/SpaceCard.svelte';
  import SpaceCardSkeleton from './components/SpaceCardSkeleton.svelte';

  let { onspacejoined }: { onspacejoined?: (spaceId: string) => void } = $props();

  type SpaceWithInstance = {
    space: SpaceCardSpaceFragment;
    instanceId: string;
  };

  const store = new SpaceDirectoryStore();
  const currentUser = getCurrentUser();

  /** Reactively track which instances are authenticated. */
  const authenticatedInstances = $derived(
    instanceRegistry.instances.filter((i) => {
      const s = instanceRegistry.tryGetStore(i.id);
      if (s?.isAuthenticated) return true;
      return instanceRegistry.isOriginInstance(i.id) && !!currentUser.user;
    })
  );

  // Reload the per-instance result map whenever the authenticated set
  // changes (sign-in, instance add/remove). The store is responsible for
  // keeping previously-loaded entries stable and dropping departed ones.
  $effect(() => {
    void store.loadAll(authenticatedInstances);
  });

  // ---- Display derivations ----

  const instanceData = $derived([...store.instanceData.values()]);

  const allLoading = $derived(
    instanceData.length === 0 || instanceData.every((d) => d.loading)
  );

  const allSpaces = $derived.by(() => {
    const result: SpaceWithInstance[] = [];
    for (const inst of instanceData) {
      if (inst.loading || inst.error || inst.canBrowse === false) continue;
      for (const space of inst.spaces) {
        result.push({ space, instanceId: inst.instanceId });
      }
    }
    result.sort((a, b) => a.space.name.localeCompare(b.space.name));
    return result;
  });

  let searchQuery = $state('');

  const filteredSpaces = $derived.by(() => {
    if (!searchQuery.trim()) return allSpaces;
    const query = searchQuery.toLowerCase();
    return allSpaces.filter(
      (s) =>
        s.space.name.toLowerCase().includes(query) ||
        s.space.description?.toLowerCase().includes(query)
    );
  });

  /** Show errors/permission issues for instances that failed to load. */
  const instanceErrors = $derived(
    instanceData.filter((d) => !d.loading && (d.error || d.canBrowse === false))
  );

  /** Show the instance pill on each card only when more than one instance is connected. */
  const multiInstance = $derived(instanceRegistry.instances.length > 1);

  async function handleJoin(instanceId: string, spaceId: string) {
    const result = await store.joinSpace(instanceId, spaceId);
    if (result.ok) {
      // Navigation to the space URL handles instance switching via the layout context
      onspacejoined?.(spaceId);
    } else {
      toast.error('Failed to join space');
      console.error('Error joining space:', result.error);
    }
  }
</script>

{#if allLoading}
  <div class="grid gap-4 grid-cols-[repeat(auto-fill,minmax(220px,1fr))]">
    {#each { length: 6 } as _, i (i)}
      <SpaceCardSkeleton />
    {/each}
  </div>
{:else}
  <div class="mb-6">
    <input
      type="text"
      placeholder="Filter spaces..."
      bind:value={searchQuery}
      class="w-full rounded-md border border-border bg-surface px-3 py-2 text-text placeholder:text-muted focus:border-primary focus:outline-none"
    />
  </div>

  {#each instanceErrors as inst (inst.instanceId)}
    {#if inst.error}
      <p class="mb-4 text-sm text-muted">Could not connect to {inst.instanceName}.</p>
    {:else if inst.canBrowse === false}
      <p class="mb-4 text-sm text-muted">No permission to browse spaces on {inst.instanceName}.</p>
    {/if}
  {/each}

  {#if allSpaces.length === 0 && instanceErrors.length === 0}
    <p class="mb-4 text-muted">No spaces available.</p>
  {:else if filteredSpaces.length === 0 && searchQuery.trim()}
    <p class="mb-4 text-muted">No spaces match your filter.</p>
  {:else}
    <div class="mb-6 grid gap-4 grid-cols-[repeat(auto-fill,minmax(220px,1fr))]">
      {#each filteredSpaces as { space, instanceId } (`${instanceId}:${space.id}`)}
        <SpaceCard
          {space}
          instanceId={multiInstance ? instanceId : undefined}
          joined={space.viewerIsMember}
          href={space.viewerIsMember
            ? resolve('/chat/[instanceId]/[spaceId]', { instanceId: instanceIdToSegment(instanceId), spaceId: space.id })
            : undefined}
          joining={store.joiningKey === `${instanceId}:${space.id}`}
          onjoin={() => handleJoin(instanceId, space.id)}
        />
      {/each}
    </div>
  {/if}
{/if}
