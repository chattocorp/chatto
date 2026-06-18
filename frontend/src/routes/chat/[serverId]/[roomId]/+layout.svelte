<script lang="ts">
  import RoomRouteLayout from './RoomRouteLayout.svelte';

  let { data, children } = $props();
</script>

<RoomRouteLayout {data} {children} />
