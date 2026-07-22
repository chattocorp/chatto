<script lang="ts">
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { viewerResponseToState } from '$lib/api-client/viewer';
  import { BotManagement } from '$lib/components/bots';
  import { AdminPageContent } from '$lib/components/admin';
  import { PaneHeader, PageTitle } from '$lib/ui';
  import * as m from '$lib/i18n/messages';

  const store = serverRegistry.getStore(getActiveServer());
  const viewer = $derived(
    store.projection.viewer ? viewerResponseToState(store.projection.viewer) : null
  );
  const canCreate = $derived(viewer?.viewerPermissions['bot.create'] ?? false);
  let scrollContainer = $state<HTMLDivElement>();
</script>

<PageTitle title={m['admin.common.server_admin_page_title']({ title: m['bots.admin.title']() })} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title={m['bots.admin.title']()} subtitle={m['bots.admin.subtitle']()} showMobileNav />
  <AdminPageContent bind:scrollContainer>
    <BotManagement scope="admin" {canCreate} {scrollContainer} />
  </AdminPageContent>
</div>
