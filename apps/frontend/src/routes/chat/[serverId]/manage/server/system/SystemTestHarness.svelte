<!--
@component

Test harness that renders the System layout with one section page, as
SvelteKit does for the matching route.
-->
<script lang="ts">
  import SystemLayout from './+layout.svelte';
  import SystemOverviewPage from './+page.svelte';
  import SystemStreamsPage from './streams/+page.svelte';
  import SystemProjectionsPage from './projections/+page.svelte';
  import SystemWorkersPage from './workers/+page.svelte';
  import SystemLiveKitPage from './livekit/+page.svelte';
  import type { SystemSection } from './systemSections';

  let { section }: { section: SystemSection } = $props();
</script>

<SystemLayout>
  {#if section === 'overview'}
    <SystemOverviewPage />
  {:else if section === 'streams'}
    <SystemStreamsPage />
  {:else if section === 'projections'}
    <SystemProjectionsPage />
  {:else if section === 'workers'}
    <SystemWorkersPage />
  {:else}
    <SystemLiveKitPage />
  {/if}
</SystemLayout>
