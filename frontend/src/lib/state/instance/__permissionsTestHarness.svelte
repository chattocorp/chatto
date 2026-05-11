<!--
  Test-only harness for getServerPermissions(). The function reads the
  `getActiveInstance` Svelte context, which can only be set from a component
  initializer — hence this tiny wrapper.
-->
<script lang="ts">
  import { setActiveInstance } from '$lib/state/activeInstance.svelte';
  import { getServerPermissions, type ServerPermissions } from './permissions.svelte';

  let {
    instanceId,
    expose
  }: {
    instanceId: string;
    expose: (perms: { readonly current: ServerPermissions }) => void;
  } = $props();

  setActiveInstance(() => instanceId);
  const perms = getServerPermissions();
  $effect(() => {
    expose(perms);
  });
</script>
