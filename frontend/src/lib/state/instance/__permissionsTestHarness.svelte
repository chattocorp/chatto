<!--
  Test-only harness for getInstancePermissions(). The function reads the
  `getActiveInstance` Svelte context, which can only be set from a component
  initializer — hence this tiny wrapper.
-->
<script lang="ts">
  import { setActiveInstance } from '$lib/state/activeInstance.svelte';
  import { getInstancePermissions, type InstancePermissions } from './permissions.svelte';

  let {
    instanceId,
    expose
  }: {
    instanceId: string;
    expose: (perms: { readonly current: InstancePermissions }) => void;
  } = $props();

  setActiveInstance(() => instanceId);
  const perms = getInstancePermissions();
  $effect(() => {
    expose(perms);
  });
</script>
