<!-- @component A queued or processing video frame with shared loading fog and a visible status label. -->
<script lang="ts">
  import LoadingFog from '$lib/ui/LoadingFog.svelte';

  let { label }: { label: string } = $props();
</script>

<div class="relative h-full w-full overflow-hidden">
  <LoadingFog class="absolute inset-0 h-full w-full rounded-none" {label} />
  <span
    aria-hidden="true"
    class="text-foreground pointer-events-none absolute start-3 bottom-3 rounded border border-border bg-background/85 px-2.5 py-1.5 text-sm backdrop-blur-sm"
  >
    {label}
  </span>
</div>
