<!-- @component
Interactive preview fixture for desktop reveal and touch-sheet regression checks.
-->
<script lang="ts">
  import PreviewFrame from './PreviewFrame.svelte';
  import PreviewAction from './PreviewAction.svelte';

  let { onaction, oncontent }: { onaction?: () => void; oncontent?: () => void } = $props();
  let paused = $state(false);
</script>

<PreviewFrame class="w-64 max-w-full">
  <button type="button" class="h-40 w-full cursor-pointer preview-content" onclick={oncontent}>
    Open preview
  </button>
  {#snippet actions()}
    <PreviewAction
      label="Pause animation"
      icon="icon-[uil--pause]"
      aria-pressed={paused}
      aria-describedby="preview-description"
      onclick={() => {
        paused = !paused;
        onaction?.();
      }}
    />
    <PreviewAction label="Unavailable action" icon="icon-[uil--trash-alt]" disabled />
  {/snippet}
</PreviewFrame>
<p id="preview-description">A sample animation</p>
<button type="button" class="btn-secondary">Outside preview</button>
