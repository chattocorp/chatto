<!-- @component Server role catalog shared by role managers and assigners. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import RoleCatalog from '$lib/components/rbac/RoleCatalog.svelte';
  import { PageTitle, PaneHeader, PaneContent } from '$lib/ui';
</script>

<PageTitle title={m('admin.common.server_admin_page_title', { title: m('admin.common.roles') })} />
<div class="pane-page">
  <PaneHeader
    title={m('admin.common.roles')}
    subtitle={m('settings.overview.descriptions.roles')}
  />
  <PaneContent>
    <RoleCatalog />
  </PaneContent>
</div>
