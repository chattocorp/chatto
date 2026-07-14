<script lang="ts">
  let {
    threadRootEventId,
    highlightEventId = null
  }: { threadRootEventId: string; highlightEventId?: string | null } = $props();
</script>

<div data-testid="thread-pane-root-id">{threadRootEventId}</div>
<div data-testid="thread-pane-highlight-id">{highlightEventId ?? ''}</div>
