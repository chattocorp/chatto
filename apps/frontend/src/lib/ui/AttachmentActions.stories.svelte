<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PreviewAction from './PreviewAction.svelte';
  import PreviewToolbar from './PreviewToolbar.svelte';

  const { Story } = defineMeta({ title: 'UI/Attachment actions' });
</script>

{#snippet actions(showViewer = false)}
  <PreviewAction label="Delete attachment" icon="icon-[uil--trash-alt]" />
  {#if showViewer}
    <PreviewAction label="View sample media" icon="icon-[uil--expand-alt]" />
  {/if}
  <PreviewAction label="Edit description" icon="icon-[uil--file-edit-alt]" />
{/snippet}

<Story name="Depth modes" asChild>
  <div class="flex flex-wrap gap-6">
    {#each [{ label: 'Flat', strength: 0, width: 1 }, { label: 'Kinda 3D', strength: 0.75, width: 1 }, { label: 'Very 3D', strength: 1.75, width: 1.5 }] as mode (mode.label)}
      <div style:--depth-strength={mode.strength} style:--depth-width={mode.width}>
        <p class="mb-2">{mode.label}</p>
        <div class="group/preview relative my-1.5 h-48 w-64 preview-frame bg-surface-emphasized">
          <PreviewToolbar>
            {@render actions(true)}
          </PreviewToolbar>
        </div>
        <div class="my-1.5 mt-4 attachment-card w-64 preview-frame">
          <button
            type="button"
            class="flex min-h-10 min-w-0 flex-1 cursor-pointer items-center gap-3 text-start"
            aria-label="View test.html"
          >
            <span class="iconify icon-[uil--file] shrink-0 text-2xl text-muted" aria-hidden="true"
            ></span>
            <span class="min-w-0 text-sm wrap-anywhere">test.html</span>
          </button>
          <PreviewToolbar layout="row">
            {@render actions()}
          </PreviewToolbar>
        </div>
        <div class="my-1.5 mt-4 attachment-card w-64 flex-wrap preview-frame">
          <audio
            controls
            preload="none"
            class="h-10 max-w-full min-w-[min(12rem,100%)] flex-1 basis-48"
          ></audio>
          <PreviewToolbar layout="row">
            {@render actions(true)}
          </PreviewToolbar>
        </div>
      </div>
    {/each}
  </div>
</Story>

<Story name="Narrow media" asChild>
  <div class="flex max-w-full flex-wrap items-start gap-6">
    <div class="w-30 max-w-full">
      <p class="mb-2">Video</p>
      <div class="group/preview attachment-video-frame">
        <video controls preload="none" class="my-1.5 w-full preview-frame"
          ><track kind="captions" /></video
        >
        <PreviewToolbar>
          {@render actions(true)}
        </PreviewToolbar>
      </div>
    </div>
    <div class="w-64 max-w-full">
      <p class="mb-2">Image gallery</p>
      <div class="flex gap-2 overflow-x-auto">
        {#each [40, 180, 240] as width (width)}
          <div class="group/preview attachment-media-frame min-w-19 shrink-0">
            <img
              src={'data:image/svg+xml,' +
                encodeURIComponent(
                  '<svg xmlns="http://www.w3.org/2000/svg" width="240" height="200"><rect width="240" height="200" fill="#408cab"/><circle cx="120" cy="100" r="32" fill="white"/></svg>'
                )}
              alt="White circle on a blue background"
              class="my-1.5 h-50 preview-frame object-cover"
              style:width={`${width}px`}
            />
            <PreviewToolbar>
              {@render actions()}
            </PreviewToolbar>
          </div>
        {/each}
      </div>
    </div>
  </div>
</Story>
