<script lang="ts">
  let {
    maximized = false,
    onToggleMaximized,
    onClose
  }: {
    maximized?: boolean;
    onToggleMaximized?: () => void;
    onClose?: () => void;
  } = $props();
</script>

<button type="button" data-testid="close-room-sidebar" onclick={() => onClose?.()}>
  Close sidebar
</button>
<button
  type="button"
  data-testid="toggle-maximized-call"
  data-maximized={String(maximized)}
  onclick={() => onToggleMaximized?.()}
>
  Toggle maximized call
</button>
