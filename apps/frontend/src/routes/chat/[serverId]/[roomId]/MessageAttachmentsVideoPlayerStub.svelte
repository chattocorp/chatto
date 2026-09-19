<script lang="ts">
  let {
    hlsUrl = null,
    autoLoop = false,
    onMediaError
  }: {
    hlsUrl?: string | null;
    autoLoop?: boolean;
    onMediaError?: () => void | Promise<string | null>;
  } = $props();
</script>

<button
  type="button"
  class="embed-frame"
  data-testid="message-attachments-video-player"
  data-hls-url={hlsUrl ?? ''}
  data-auto-loop={autoLoop}
  style="width: 480px; max-width: 100%; aspect-ratio: 16 / 9"
  onclick={() => onMediaError?.()}
>
  Test video player
</button>
