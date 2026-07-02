<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import CallTileActionToolbar from './CallTileActionToolbar.svelte';
  import CallTileActionButton from './CallTileActionButton.svelte';

  const { Story } = defineMeta({
    title: 'Voice/Call tile actions',
    component: CallTileActionToolbar,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Hover toolbar used on call media and voice participant tiles for viewer-local actions.'
        }
      }
    }
  });
</script>

<Story name="Media tile toolbar" asChild>
  <div class="w-80 rounded-md bg-background p-4">
    <div class="group/media relative overflow-hidden rounded-md border border-border bg-surface-100 p-1.5">
      <div class="aspect-video rounded-md bg-black"></div>
      <CallTileActionToolbar forceVisible>
        <CallTileActionButton icon="mdi--fullscreen" label="Fullscreen feed" />
        <CallTileActionButton icon="uil--volume-up" label="Mute locally" />
      </CallTileActionToolbar>
    </div>
  </div>
</Story>

<Story name="Muted state" asChild>
  <div class="w-80 rounded-md bg-background p-4">
    <div class="group/media relative flex items-center gap-2 rounded-md border border-border bg-surface-100 p-1.5 pr-12">
      <div class="grid h-8 w-8 place-items-center rounded-full bg-surface-300 text-sm font-semibold">
        A
      </div>
      <span class="min-w-0 flex-1 truncate text-sm font-medium">Alice</span>
      <CallTileActionToolbar forceVisible>
        <CallTileActionButton icon="uil--volume-mute" label="Unmute locally" active />
      </CallTileActionToolbar>
    </div>
  </div>
</Story>
