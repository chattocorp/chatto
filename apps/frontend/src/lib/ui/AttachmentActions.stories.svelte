<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  const { Story } = defineMeta({ title: 'UI/Attachment actions' });
</script>

{#snippet actions(showViewer = false)}
  <button
    type="button"
    class="btn-danger-secondary attachment-action-button"
    aria-label="Delete attachment"
    title="Delete attachment"
  >
    <span class="iconify icon-[uil--trash-alt]" aria-hidden="true"></span>
  </button>
  {#if showViewer}
    <button
      type="button"
      class="btn-secondary attachment-action-button"
      aria-label="View sample media"
      title="View sample media"
    >
      <span class="iconify icon-[uil--expand-alt]" aria-hidden="true"></span>
    </button>
  {/if}
  <button
    type="button"
    class="btn-secondary attachment-action-button"
    aria-label="Edit description"
    title="Edit description"
  >
    <span class="iconify icon-[uil--file-edit-alt]" aria-hidden="true"></span>
  </button>
{/snippet}

<Story name="Depth modes" asChild>
  <div class="flex flex-wrap gap-6">
    {#each [{ label: 'Flat', strength: 0, width: 1 }, { label: 'Kinda 3D', strength: 0.75, width: 1 }, { label: 'Very 3D', strength: 1.75, width: 1.5 }] as mode (mode.label)}
      <div style:--depth-strength={mode.strength} style:--depth-width={mode.width}>
        <p class="mb-2">{mode.label}</p>
        <div class="group/attachment relative embed-frame h-48 w-64 bg-surface-emphasized">
          <div class="absolute top-3 right-2 flex flex-col gap-1">
            {@render actions(true)}
          </div>
        </div>
        <div class="embed-frame mt-4 flex w-64 items-center">
          <button
            type="button"
            class="flex min-h-16 min-w-0 flex-1 cursor-pointer items-center gap-3 px-3 py-3 text-start"
            aria-label="View test.html"
          >
            <span class="iconify icon-[uil--file] shrink-0 text-2xl text-muted" aria-hidden="true"
            ></span>
            <span class="min-w-0 text-sm wrap-anywhere">test.html</span>
          </button>
          <div class="me-2 flex shrink-0 gap-1">
            {@render actions()}
          </div>
        </div>
        <div
          class="embed-frame mt-4 flex min-h-16 w-64 max-w-full flex-wrap items-center gap-3 p-3"
        >
          <audio
            controls
            preload="none"
            class="h-8 max-w-full min-w-[min(12rem,100%)] flex-1 basis-48"
          ></audio>
          <div class="me-2 flex max-w-full flex-wrap items-center gap-1">
            {@render actions(true)}
          </div>
        </div>
      </div>
    {/each}
  </div>
</Story>
