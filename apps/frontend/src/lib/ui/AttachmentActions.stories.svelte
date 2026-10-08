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
          <div class="absolute end-2 top-2 flex items-center gap-1">
            {@render actions(true)}
          </div>
        </div>
        <div class="embed-frame mt-4 attachment-card w-64">
          <button
            type="button"
            class="flex min-h-10 min-w-0 flex-1 cursor-pointer items-center gap-3 text-start"
            aria-label="View test.html"
          >
            <span class="iconify icon-[uil--file] shrink-0 text-2xl text-muted" aria-hidden="true"
            ></span>
            <span class="min-w-0 text-sm wrap-anywhere">test.html</span>
          </button>
          <div class="flex shrink-0 gap-1">
            {@render actions()}
          </div>
        </div>
        <div class="embed-frame mt-4 attachment-card w-64 flex-wrap">
          <audio
            controls
            preload="none"
            class="h-10 max-w-full min-w-[min(12rem,100%)] flex-1 basis-48"
          ></audio>
          <div class="flex max-w-full flex-wrap items-center gap-1">
            {@render actions(true)}
          </div>
        </div>
      </div>
    {/each}
  </div>
</Story>

<Story name="Narrow media" asChild>
  <div class="flex max-w-full flex-wrap items-start gap-6">
    <div class="w-30 max-w-full">
      <p class="mb-2">Video</p>
      <div class="group/attachment attachment-video-frame">
        <video controls preload="none" class="embed-frame w-full"><track kind="captions" /></video>
        <div
          class="absolute end-2 top-2 z-10 flex items-center gap-1 transition-opacity feedback-quick group-hover/attachment:opacity-100 focus-within:opacity-100 compact-input:hover-actions:opacity-0"
        >
          {@render actions(true)}
        </div>
      </div>
    </div>
    <div class="w-64 max-w-full">
      <p class="mb-2">Image gallery</p>
      <div class="flex gap-2 overflow-x-auto">
        {#each [40, 180, 240] as width (width)}
          <div class="group/attachment attachment-media-frame min-w-19 shrink-0">
            <img
              src={'data:image/svg+xml,' +
                encodeURIComponent(
                  '<svg xmlns="http://www.w3.org/2000/svg" width="240" height="200"><rect width="240" height="200" fill="#408cab"/><circle cx="120" cy="100" r="32" fill="white"/></svg>'
                )}
              alt="White circle on a blue background"
              class="embed-frame h-50 object-cover"
              style:width={`${width}px`}
            />
            <div
              class="absolute end-2 top-2 z-10 flex items-center gap-1 transition-opacity feedback-quick group-hover/attachment:opacity-100 focus-within:opacity-100 compact-input:hover-actions:opacity-0"
            >
              {@render actions()}
            </div>
          </div>
        {/each}
      </div>
    </div>
  </div>
</Story>
