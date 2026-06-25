<script lang="ts">
  let {
    error
  }: {
    error?: string | null;
  } = $props();
</script>

{#if error}
  <div class="form-error" role="alert">{error}</div>
{/if}
