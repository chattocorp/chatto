<script lang="ts">
  import type { Snippet } from 'svelte';
  import { PreviewToolbar } from '$lib/ui';
  import { videoFrameStyle } from '$lib/media/videoFrame';

  let {
    width = null,
    height = null,
    hlsUrl = null,
    autoLoop = false,
    onMediaError,
    onOpen,
    actions
  }: {
    width?: number | null;
    height?: number | null;
    hlsUrl?: string | null;
    autoLoop?: boolean;
    onOpen?: () => void;
    actions?: Snippet;
    onMediaError?: () => void | Promise<string | null>;
  } = $props();
</script>

<div
  class="preview-media-frame my-1.5 min-w-14 preview-frame"
  style={videoFrameStyle(width, height, autoLoop)}
>
  <button
    data-testid="message-attachments-video-player"
    data-hls-url={hlsUrl ?? ''}
    data-auto-loop={autoLoop}
    type="button"
    class="mx-auto block w-full"
    style:max-width={autoLoop ? 'var(--preview-media-width)' : undefined}
    onclick={() => (onOpen ? onOpen() : onMediaError?.())}>Test video player</button
  >
  {#if actions}<PreviewToolbar>{@render actions()}</PreviewToolbar>{/if}
</div>
