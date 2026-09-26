<!--
@component

History-backed Add Server view. It fills the app frame with the Server
Directory so a user can add a server without leaving the current chat view.
-->
<script lang="ts">
  import ServerDirectory from '$lib/components/ServerDirectory.svelte';
  import { m } from '$lib/i18n/messages';
  import { FrameView } from '$lib/ui';

  let {
    onclose
  }: {
    onclose: () => void;
  } = $props();
</script>

<FrameView
  title={m('add_server.directory.title')}
  subtitle={m('add_server.directory.subtitle')}
  {onclose}
>
  <ServerDirectory inAddServerView />
</FrameView>
