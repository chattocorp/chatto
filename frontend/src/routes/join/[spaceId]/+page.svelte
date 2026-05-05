<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';

  // join page is outside the [[instanceId=hostname]] route tree,
  // so it cannot use getActiveInstance(). Use the home instance ID.
  const homeInstanceId = $derived(instanceRegistry.originInstance?.id ?? '');
  import { graphql } from '$lib/gql';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  let { data } = $props();

  const spaceId = $derived(data.spaceId!);

  type SpaceInfo = {
    id: string;
    name: string;
    description?: string | null;
    memberCount: number;
  };

  let space = $state<SpaceInfo | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let isLoggedIn = $state(false);
  let joining = $state(false);

  async function loadSpaceAndStatus() {
    loading = true;
    error = null;

    try {
      const result = await graphqlClientManager.originClient.client
        .query(
          graphql(`
            query SpaceJoinPage($spaceId: ID!) {
              space(id: $spaceId) {
                id
                name
                description
                memberCount
                viewerIsMember
              }
              me {
                id
              }
            }
          `),
          { spaceId }
        )
        .toPromise();

      if (result.error || !result.data?.space) {
        error = 'Space not found';
        return;
      }

      space = result.data.space;
      isLoggedIn = !!result.data.me;
    } catch (_e) {
      error = 'Failed to load space information';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    loadSpaceAndStatus();
  });

  async function joinSpace() {
    joining = true;
    try {
      const result = await graphqlClientManager.originClient.client
        .mutation(
          graphql(`
            mutation JoinSpaceFromInvite($input: JoinSpaceInput!) {
              joinSpace(input: $input)
            }
          `),
          { input: { spaceId } }
        )
        .toPromise();

      if (result.error) {
        error = 'Failed to join space';
        return;
      }

      // eslint-disable-next-line svelte/no-navigation-without-resolve -- uses resolve() with query string appended
      goto(resolve('/chat/[instanceId]', { instanceId: instanceIdToSegment(homeInstanceId) }) + '?welcome=true');
    } catch (_e) {
      error = 'Failed to join space';
    } finally {
      joining = false;
    }
  }
</script>

<PageTitle title={space ? `Join ${space.name}` : 'Join Space'} />

<div class="flex min-h-0 flex-1 items-center justify-center overflow-y-auto p-8">
  <div class="w-full max-w-md rounded-lg bg-surface p-8 shadow-xl">
    {#if loading}
      <div class="flex items-center justify-center py-8">
        <div
          class="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"
        ></div>
      </div>
    {:else if error}
      <div class="text-center">
        <div class="mb-4 text-4xl">:(</div>
        <h1 class="mb-4 text-xl font-bold">{error}</h1>
        <a href={resolve('/')} class="btn-secondary inline-block">Go to Home</a>
      </div>
    {:else if space}
      <div class="text-center">
        <h1 class="mb-2 text-2xl font-bold">{space.name}</h1>

        {#if space.description}
          <p class="mb-4 text-muted">{space.description}</p>
        {/if}

        <p class="mb-6 text-sm text-muted">
          {`${space.memberCount} ${space.memberCount === 1 ? 'member' : 'members'}`}
        </p>

        {#if isLoggedIn}
          <!-- Logged in, not a member -->
          <button type="button" class="btn-primary w-full" onclick={joinSpace} disabled={joining}>
            {joining ? 'Joining...' : 'Join Space'}
          </button>
        {:else}
          <!-- Not logged in -->
          <p class="mb-4 text-muted">Sign in to join this space</p>

          <div class="flex flex-col gap-3">
            <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- uses resolve() with query string -->
            <a href={resolve('/login') + `?redirect=/join/${spaceId}`} class="btn-primary text-center">
              Sign in with Email
            </a>
            <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- uses resolve() with query string -->
            <a href={resolve('/register') + `?join=${spaceId}`} class="btn-secondary text-center"
              >Create Account</a
            >

            <div class="relative my-2">
              <div class="absolute inset-0 flex items-center">
                <div class="w-full border-t border-border"></div>
              </div>
              <div class="relative flex justify-center text-sm">
                <span class="bg-surface px-2 text-muted">or</span>
              </div>
            </div>

            <!-- eslint-disable svelte/no-navigation-without-resolve -->
            <a href={`/auth/google?redirect=/join/${spaceId}`} class="btn-ghost text-center">
              Continue with Google
            </a>
            <!-- eslint-enable svelte/no-navigation-without-resolve -->
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
