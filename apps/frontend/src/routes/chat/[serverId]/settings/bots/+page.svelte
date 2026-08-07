<script lang="ts">
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { viewerResponseToState } from '$lib/api-client/viewer';
  import { BotManagement } from '$lib/components/bots';
  import { AccessDenied, PaneContent, PaneHeader, PageTitle } from '$lib/ui';
  import { m } from '$lib/i18n/messages';

  const store = serverRegistry.getStore(getActiveServer());
  const viewer = $derived(
    store.projection.viewer ? viewerResponseToState(store.projection.viewer) : null
  );
  const supported = $derived(store.serverInfo.supportsFeature('bots'));
  const canCreate = $derived(viewer?.viewerPermissions['bot.create'] ?? false);
  const accessReady = $derived(
    !store.serverInfo.loading && store.permissions.loaded && viewer !== null
  );
  let scrollContainer = $state<HTMLDivElement>();
</script>

<PageTitle title={m('bots.settings.page_title')} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={m('bots.settings.title')}
    subtitle={m('bots.settings.subtitle')}
    showMobileNav
  />
  <PaneContent bind:scrollContainer>
    {#if !accessReady}
      <!-- Keep the settings shell stable while discovery and viewer permissions hydrate. -->
    {:else if supported && canCreate}
      <BotManagement scope="owner" {canCreate} {scrollContainer} />
    {:else}
      <AccessDenied message={m('bots.unavailable.owner')} />
    {/if}
  </PaneContent>
</div>
