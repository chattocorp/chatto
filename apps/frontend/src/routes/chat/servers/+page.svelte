<!--
@component

Full-page Server Directory. The standalone client links here before any server
is registered; the Server Gutter opens the same directory in a view that fills
the app frame instead.
-->
<script lang="ts">
  import ServerDirectory from '$lib/components/ServerDirectory.svelte';
  import { m } from '$lib/i18n/messages';
  import { PageTitle, PaneContent, PaneHeader } from '$lib/ui';
</script>

<PageTitle title={m('add_server.directory.title')} />

<div class="pane-page">
  <PaneHeader
    title={m('add_server.directory.title')}
    subtitle={m('add_server.directory.subtitle')}
    showMobileNav
  />

  <PaneContent>
    <ServerDirectory />
  </PaneContent>
</div>
