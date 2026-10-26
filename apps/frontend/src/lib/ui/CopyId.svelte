<script lang="ts">
  import CopyButton from './CopyButton.svelte';

  let { value }: { value: string } = $props();
</script>

<span class="inline-flex min-w-0 items-center gap-1.5 align-middle">
  <code class="font-mono text-xs leading-6">{value}</code>
  <CopyButton {value} />
</span>
