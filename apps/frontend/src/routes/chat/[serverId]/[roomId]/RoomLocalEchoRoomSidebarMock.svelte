<script lang="ts">
  let {
    maximized = false,
    onToggleMaximized,
    onOpenFileMessage,
    onClose
  }: {
    maximized?: boolean;
    onToggleMaximized?: () => void;
    onOpenFileMessage?: (messageEventId: string, threadRootEventId: string | null) => void;
    onClose?: () => void;
  } = $props();
</script>

<button
  type="button"
  data-testid="open-file-message"
  onclick={() => onOpenFileMessage?.('msg-linked', null)}
>
  Open file message
</button>
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
