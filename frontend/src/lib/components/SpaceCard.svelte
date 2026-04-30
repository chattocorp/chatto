<script lang="ts" module>
  /* eslint-disable svelte/no-navigation-without-resolve -- href is a prop; callers pass already-resolved paths */
  import { graphql } from '$lib/gql';

  // Fragment for card data - banner at 384x288 (2x retina for ~192x144 display, 4:3 aspect)
  export const SpaceCardFragment = graphql(`
    fragment SpaceCardSpace on Space {
      id
      name
      description
      logoUrl(width: 96, height: 96)
      bannerUrl(width: 384, height: 288)
      memberCount
      viewerCanJoinSpace
      viewerIsMember
    }
  `);
</script>

<script lang="ts">
  import type { SpaceCardSpaceFragment } from '$lib/gql/graphql';
  import { getGradientForName } from '$lib/utils/gradients';
  import SpaceLogo from './SpaceLogo.svelte';
  import InstancePill from './InstancePill.svelte';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';

  let {
    space,
    joining = false,
    joined = false,
    href,
    instanceId,
    onjoin
  }: {
    space: SpaceCardSpaceFragment;
    joining?: boolean;
    joined?: boolean;
    href?: string;
    /** Instance ID shown as an InstancePill below the space name in multi-instance views. */
    instanceId?: string;
    onjoin?: () => void;
  } = $props();

  const gradientStyle = $derived(space.bannerUrl ? undefined : getGradientForName(space.name));
  const canJoin = $derived(!joined && space.viewerCanJoinSpace);
</script>

<article
  class="group flex flex-col overflow-hidden rounded-xl border border-border bg-background transition-shadow hover:shadow-lg"
  data-testid="space-card"
>
  {#snippet banner()}
    {#if space.bannerUrl}
      <SkeletonImg src={space.bannerUrl} alt="" class="h-full w-full object-cover" />
    {:else if space.logoUrl}
      <div class="flex h-full w-full items-center justify-center" style:background={gradientStyle}>
        <img src={space.logoUrl} alt="" class="h-1/2 w-1/2 rounded-lg object-contain" />
      </div>
    {:else}
      <div class="h-full w-full" style:background={gradientStyle}></div>
    {/if}
  {/snippet}

  <!-- Banner and logo container -->
  <div class="relative shrink-0">
    <!-- Banner -->
    {#if joined && href}
      <a href={href} class="shimmer-hover block aspect-4/3 w-full cursor-pointer">
        {@render banner()}
      </a>
    {:else if canJoin}
      <button
        type="button"
        class="shimmer-hover aspect-4/3 w-full cursor-pointer"
        onclick={onjoin}
        disabled={joining}
        aria-label="Join {space.name}"
      >
        {@render banner()}
      </button>
    {:else}
      <div class="aspect-4/3 w-full">
        {@render banner()}
      </div>
    {/if}

    <!-- Logo overlapping banner/content boundary -->
    {#if joined && href}
      <a
        href={href}
        class="absolute -bottom-6 left-4 z-10 cursor-pointer rounded-xl border-2 border-background shadow-md transition-transform hover:scale-105"
      >
        <SpaceLogo {space} />
      </a>
    {:else if canJoin}
      <button
        type="button"
        class="absolute -bottom-6 left-4 z-10 cursor-pointer rounded-xl border-2 border-background shadow-md transition-transform hover:scale-105"
        onclick={onjoin}
        disabled={joining}
        aria-label="Join {space.name}"
      >
        <SpaceLogo {space} />
      </button>
    {:else}
      <div class="absolute -bottom-6 left-4 z-10 rounded-xl border-2 border-background shadow-md">
        <SpaceLogo {space} />
      </div>
    {/if}
  </div>

  <!-- Content area -->
  <div class="flex flex-1 flex-col p-4 pt-8">
    {#if instanceId}
      <div class="-ml-1 mb-1">
        <InstancePill instanceId={instanceId} />
      </div>
    {/if}
    <h3 class="font-semibold text-text">{space.name}</h3>

    {#if space.description}
      <p class="mt-1 line-clamp-2 text-sm text-muted">{space.description}</p>
    {/if}

    <div class="mt-auto flex items-center justify-between pt-4">
      <span class="text-sm text-muted">
        <span class="iconify inline-block align-text-bottom uil--users-alt"></span>
        {space.memberCount}
        {space.memberCount === 1 ? 'member' : 'members'}
      </span>

      {#if joined && href}
        <a
          href={href}
          class="rounded-md border border-success/30 bg-success/10 px-3 py-1.5 text-center text-sm font-medium text-success"
        >
          Joined
        </a>
      {:else if canJoin}
        <button
          type="button"
          class="btn-primary cursor-pointer btn-sm"
          onclick={onjoin}
          disabled={joining}
        >
          {joining ? 'Joining...' : 'Join'}
        </button>
      {:else}
        <span class="text-sm text-muted">No permission to join</span>
      {/if}
    </div>
  </div>
</article>
