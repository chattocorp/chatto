<script lang="ts">
  import { videoFrameStyle } from '$lib/media/videoFrame';

  let {
    width = null,
    height = null,
    hlsUrl = null,
    autoLoop = false,
    onMediaError
  }: {
    width?: number | null;
    height?: number | null;
    hlsUrl?: string | null;
    autoLoop?: boolean;
    onMediaError?: () => void | Promise<string | null>;
  } = $props();
</script>

<button
  type="button"
  class="preview-media-frame my-1.5 preview-frame"
  data-testid="message-attachments-video-player"
  data-hls-url={hlsUrl ?? ''}
  data-auto-loop={autoLoop}
  style={videoFrameStyle(width, height, autoLoop)}
  onclick={() => onMediaError?.()}
>
  <span class="block w-full">Test video player</span>
</button>
