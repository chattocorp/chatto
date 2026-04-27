<script lang="ts">
  import { SvelteMap } from 'svelte/reactivity';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
  import { toast } from '$lib/ui/toast';
  import { graphql, useFragment } from './gql';
  import { SpaceCardSpaceFragmentDoc, type SpaceCardSpaceFragment } from './gql/graphql';
  import SpaceCard from './components/SpaceCard.svelte';
  import SpaceCardSkeleton from './components/SpaceCardSkeleton.svelte';

  let { onspacejoined }: { onspacejoined?: (spaceId: string) => void } = $props();

  type InstanceSpaceData = {
    instanceId: string;
    instanceName: string;
    loading: boolean;
    canBrowse: boolean | null;
    spaces: SpaceCardSpaceFragment[];
    error: string | null;
  };

  type SpaceWithInstance = {
    space: SpaceCardSpaceFragment;
    instanceId: string;
    instanceName: string;
  };

  /** Per-instance space data, keyed by instance ID. Survives other instances being added/removed.
   *  Uses SvelteMap so that .set()/.delete() trigger reactive updates in $derived consumers. */
  const instanceDataMap = new SvelteMap<string, InstanceSpaceData>();

  let searchQuery = $state('');
  let joiningKey = $state<string | null>(null); // "instanceId:spaceId"

  const instanceData = $derived([...instanceDataMap.values()]);

  const allLoading = $derived(
    instanceData.length === 0 || instanceData.every((d) => d.loading)
  );

  /** Flatten all instance spaces into a single list, sorted by name. */
  const allSpaces = $derived.by(() => {
    const result: SpaceWithInstance[] = [];
    for (const inst of instanceData) {
      if (inst.loading || inst.error || inst.canBrowse === false) continue;
      const store = instanceRegistry.tryGetStore(inst.instanceId);
      const instanceName = store?.instance.name ?? inst.instanceName;
      for (const space of inst.spaces) {
        result.push({ space, instanceId: inst.instanceId, instanceName });
      }
    }
    result.sort((a, b) => a.space.name.localeCompare(b.space.name));
    return result;
  });

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

  // Read currentUser from Svelte context. AuthenticatedChatProvider re-sets
  // this context to the origin's registry CurrentUserState when it mounts, so
  // it stays in sync with what the chat layout's auth guard reads.
  const currentUser = getCurrentUser();

  /** Reactively track which instances are authenticated. */
  const authenticatedInstances = $derived(
    instanceRegistry.instances.filter((i) =>
      instanceRegistry.isOriginInstance(i.id)
        ? !!currentUser.user
        : !!i.token
    )
  );

  const LoadInstanceSpacesQuery = graphql(`
    query LoadInstanceSpaces {
      spaces {
        ...SpaceCardSpace
      }
      viewer {
        canListSpaces
      }
    }
  `);

  /** Load spaces for all authenticated instances. Called reactively when
   *  the set of authenticated instances changes (login, instance add/remove). */
  async function loadAllInstances(instances: typeof authenticatedInstances) {
    // Remove stale entries for instances that disappeared
    const currentIds = new Set(instances.map((i) => i.id));
    for (const id of instanceDataMap.keys()) {
      if (!currentIds.has(id)) {
        instanceDataMap.delete(id);
      }
    }

    // Set loading state for all instances
    for (const inst of instances) {
      instanceDataMap.set(inst.id, {
        instanceId: inst.id,
        instanceName: inst.name,
        loading: true,
        canBrowse: null,
        spaces: [],
        error: null
      });
    }

    // Query each instance in parallel
    await Promise.all(instances.map(async (inst) => {
      try {
        const client = graphqlClientManager.getClient(inst.id).client;
        const result = await client.query(LoadInstanceSpacesQuery, {}).toPromise();

        if (result.error) {
          instanceDataMap.set(inst.id, {
            ...instanceDataMap.get(inst.id)!,
            error: result.error.message,
            loading: false
          });
          return;
        }

        if (result.data) {
          const canList = result.data.viewer?.canListSpaces ?? false;
          const spaces = canList
            ? (result.data.spaces?.map((s) => useFragment(SpaceCardSpaceFragmentDoc, s)) ?? [])
            : [];

          instanceDataMap.set(inst.id, {
            ...instanceDataMap.get(inst.id)!,
            canBrowse: canList,
            spaces,
            loading: false
          });
          return;
        }
      } catch (err) {
        instanceDataMap.set(inst.id, {
          ...instanceDataMap.get(inst.id)!,
          error: err instanceof Error ? err.message : 'Failed to connect',
          loading: false
        });
        return;
      }

      instanceDataMap.set(inst.id, { ...instanceDataMap.get(inst.id)!, loading: false });
    }));
  }

  /** React to instances appearing/disappearing.
   *  Reads `authenticatedInstances` synchronously (tracked), then passes
   *  the snapshot to the async loader to avoid the effect depending on
   *  instanceDataMap (which the loader mutates). */
  $effect(() => {
    const instances = authenticatedInstances;
    loadAllInstances(instances);
  });

  async function joinSpace(instanceId: string, spaceId: string) {
    const key = `${instanceId}:${spaceId}`;
    joiningKey = key;

    try {
      const client = graphqlClientManager.getClient(instanceId).client;
      const result = await client
        .mutation(
          graphql(`
            mutation JoinSpace($input: JoinSpaceInput!) {
              joinSpace(input: $input)
            }
          `),
          { input: { spaceId } }
        )
        .toPromise();

      if (result.error) {
        toast.error('Failed to join space');
        console.error('Error joining space:', result.error);
        return;
      }

      // Navigation to the space URL handles instance switching via the layout context
      onspacejoined?.(spaceId);
    } finally {
      joiningKey = null;
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
      {#each filteredSpaces as { space, instanceId, instanceName } (`${instanceId}:${space.id}`)}
        <SpaceCard
          {space}
          {instanceName}
          joined={space.viewerIsMember}
          href={space.viewerIsMember
            ? resolve('/chat/[instanceId]/[spaceId]', { instanceId: instanceIdToSegment(instanceId), spaceId: space.id })
            : undefined}
          joining={joiningKey === `${instanceId}:${space.id}`}
          onjoin={() => joinSpace(instanceId, space.id)}
        />
      {/each}
    </div>
  {/if}
{/if}
