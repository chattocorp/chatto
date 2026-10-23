<!--
@component

Workers section of the System page: the durable workers that perform
reliable effects, and the asset cleanup worker.
-->
<script lang="ts">
  import AssetCleanupPanel from '../AssetCleanupPanel.svelte';
  import DurableWorkersPanel from '../DurableWorkersPanel.svelte';
  import { useSystem } from '../systemContext';

  const system = useSystem();
</script>

<DurableWorkersPanel workers={system.info.durableWorkers} />
<AssetCleanupPanel status={system.info.assetCleanup} />
