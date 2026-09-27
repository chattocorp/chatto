<!--
@component

A hidden marker that end-to-end tests use to see when the active server's
realtime projection is ready.
-->
<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';

  const serverScope = useServerScope();
</script>

<div
  data-testid="server-subscription-active"
  data-projection-ready={serverScope.store.realtimeSync.phase === 'ready'}
  class="hidden"
></div>
