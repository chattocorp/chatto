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
  data-testid="message-attachments-video-player"
  data-hls-url={hlsUrl ?? ''}
  data-auto-loop={autoLoop}
  onclick={() => onMediaError?.()}
>
  Test video player
</button>
