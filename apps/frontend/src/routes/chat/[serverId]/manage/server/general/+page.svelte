<script lang="ts">
  import ServerSettings from '$lib/ServerSettings.svelte';
  import { PaneContent, PaneHeader, PageTitle } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
</script>

<PageTitle
  title={m('admin.common.server_admin_page_title', { title: m('server_settings.general') })}
/>

<div class="pane-page">
  <PaneHeader title={m('server_settings.general')} subtitle={m('admin.general.subtitle')} />

  <PaneContent>
    <ServerSettings />
  </PaneContent>
</div>
