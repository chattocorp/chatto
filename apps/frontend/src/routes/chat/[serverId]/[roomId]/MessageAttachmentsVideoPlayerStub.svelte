<script lang="ts">
  let {
    hlsUrl = null,
    onMediaError
  }: {
    hlsUrl?: string | null;
    onMediaError?: () => void | Promise<string | null>;
  } = $props();
</script>

<button
  type="button"
  data-testid="message-attachments-video-player"
  data-hls-url={hlsUrl ?? ''}
  onclick={() => onMediaError?.()}
>
  Test video player
</button>
