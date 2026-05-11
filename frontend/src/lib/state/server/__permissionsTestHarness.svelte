<!--
  Test-only harness for getServerPermissions(). The function reads the
  `getActiveServer` Svelte context, which can only be set from a component
  initializer — hence this tiny wrapper.
-->
<script lang="ts">
  import { setActiveServer } from '$lib/state/activeServer.svelte';
  import { getServerPermissions, type ServerPermissions } from './permissions.svelte';

  let {
    serverId,
    expose
  }: {
    serverId: string;
    expose: (perms: { readonly current: ServerPermissions }) => void;
  } = $props();

  setActiveServer(() => serverId);
  const perms = getServerPermissions();
  $effect(() => {
    expose(perms);
  });
</script>
