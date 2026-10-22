<script lang="ts">
  import { page } from '$app/state';
  import Layout from './+layout.svelte';
  import General from './+page.svelte';
  import Permissions from './permissions/+page.svelte';
</script>

<Layout>
  {#if page.route.id?.endsWith('/permissions')}
    <Permissions />
  {:else}
    <General />
  {/if}
</Layout>
