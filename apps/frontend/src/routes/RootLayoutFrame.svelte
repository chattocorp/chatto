<script lang="ts">
  import type { Snippet } from 'svelte';
  import ConnectionIndicator from '$lib/components/ConnectionIndicator.svelte';
  import { AppHeader, Frame } from '$lib/ui';
  import RootSidebarChrome from './RootSidebarChrome.svelte';

  let { children }: { children?: Snippet } = $props();
</script>

<div
  class="flex h-full w-full flex-col overscroll-y-contain bg-surface-100 pt-[env(safe-area-inset-top,0px)] md:p-3 md:pt-0"
>
  <ConnectionIndicator />

  <AppHeader />

  <Frame class="relative flex-col">
    <RootSidebarChrome>
      {@render children?.()}
    </RootSidebarChrome>
  </Frame>
</div>
