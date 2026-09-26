<!--
@component

History-backed Add Server dialog. It hosts the Server Directory so a user can
add a server without leaving the current chat view.
-->
<script lang="ts">
  import ServerDirectory from '$lib/components/ServerDirectory.svelte';
  import { m } from '$lib/i18n/messages';
  import { Dialog } from '$lib/ui';

  let {
    onclose
  }: {
    onclose: () => void;
  } = $props();
</script>

<Dialog visible title={m('chat.server_gutter.add_server')} size="xl" {onclose}>
  <ServerDirectory inDialog />
</Dialog>
