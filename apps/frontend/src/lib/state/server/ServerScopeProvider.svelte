<script lang="ts">
  import { onDestroy, untrack, type Snippet } from 'svelte';
  import type { ServerConnection } from './serverConnection.svelte';
  import { provideServerScope } from './scope.svelte';
  import type { ServerStateStore } from './store.svelte';

  let {
    serverId,
    connection,
    store,
    children
  }: {
    serverId: string;
    connection: ServerConnection;
    store: ServerStateStore;
    children: Snippet;
  } = $props();

  let current = true;
  onDestroy(() => {
    current = false;
  });

  // This provider is remounted by the keyed route layout. The snapshot keeps
  // pending work from an old subtree attached to its original server, while
  // isCurrent() lets continuations suppress UI effects after that teardown.
  const snapshot = untrack(() => ({ serverId, connection, store }));
  provideServerScope({
    ...snapshot,
    isCurrent: () => current
  });
</script>

{@render children()}
