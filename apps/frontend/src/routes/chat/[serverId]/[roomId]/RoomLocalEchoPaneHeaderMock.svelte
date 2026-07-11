<script lang="ts">
  let {
    title,
    loading = false
  }: {
    title: string;
    loading?: boolean;
  } = $props();
</script>

<header data-testid="pane-header-mock">
  {#if !loading}
    <h1>{title}</h1>
  {/if}
</header>
