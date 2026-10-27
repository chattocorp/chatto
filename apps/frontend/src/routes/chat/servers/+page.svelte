<!--
@component

The shared Server Directory and standalone start page. The Server Gutter
opens this page in every host.
-->
<script lang="ts">
  import ServerDirectory from '$lib/components/ServerDirectory.svelte';
  import { m } from '$lib/i18n/messages';
  import { PageTitle, PaneContent, PaneHeader } from '$lib/ui';
</script>

<PageTitle title={m('add_server.directory.title')} />

<div class="pane-page">
  <PaneHeader title={m('add_server.directory.title')} />

  <PaneContent wide>
    <ServerDirectory />
  </PaneContent>
</div>
