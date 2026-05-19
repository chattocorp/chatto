<script lang="ts">
  let {
    compact = false,
    lines = 1
  }: {
    compact?: boolean;
    lines?: number;
  } = $props();
</script>

<div class={['group relative', compact ? '' : 'mt-4']}>
  <div class={['flex gap-4 px-2 py-1 md:mx-2', compact ? 'items-baseline' : 'items-start']}>
    <!-- Left column: timestamp placeholder (compact) or avatar placeholder (full) -->
    {#if compact}
      <div class="w-11 shrink-0"></div>
    {:else}
      <div class="skeleton h-11 w-11 shrink-0 rounded-full"></div>
    {/if}

    <!-- Content column -->
    <div class="min-w-0 flex-1 space-y-2">
      {#if !compact}
        <!-- Name + timestamp row -->
        <div class="flex items-center gap-2">
          <div class="skeleton h-4 w-24 rounded"></div>
          <div class="skeleton h-3 w-12 rounded"></div>
        </div>
      {/if}

      <!-- Body lines -->
      {#each Array(lines) as _, i (i)}
        <div class="skeleton h-4 w-full rounded"></div>
      {/each}
    </div>
  </div>
</div>
