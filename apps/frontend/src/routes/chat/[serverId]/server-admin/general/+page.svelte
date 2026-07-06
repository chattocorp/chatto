<script lang="ts">
  import ServerSettings from '$lib/ServerSettings.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import * as m from '$lib/i18n/messages';
</script>

<PageTitle
  title={m['admin.common.server_admin_page_title']({ title: m['server_settings.general']() })}
/>

<PaneHeader
  title={m['server_settings.general']()}
  subtitle={m['admin.general.subtitle']()}
  showMobileNav
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <ServerSettings />
</div>
