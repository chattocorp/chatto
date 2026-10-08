<!--
@component

Test harness that renders the bot detail layout with one section page, as
SvelteKit does for the matching route.
-->
<script lang="ts">
  import BotDetailLayout from './+layout.svelte';
  import BotOverviewPage from './+page.svelte';
  import BotIntegrationsPage from './integrations/+page.svelte';
  import BotPermissionsPage from './permissions/+page.svelte';

  let { section }: { section: 'overview' | 'integrations' | 'permissions' } = $props();
</script>

<BotDetailLayout>
  {#if section === 'overview'}
    <BotOverviewPage />
  {:else if section === 'integrations'}
    <BotIntegrationsPage />
  {:else}
    <BotPermissionsPage />
  {/if}
</BotDetailLayout>
