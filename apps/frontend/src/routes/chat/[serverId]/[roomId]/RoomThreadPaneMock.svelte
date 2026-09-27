<script lang="ts">
  import type { PendingHighlight } from './roomNavigationState.svelte';

  let {
    threadRootEventId,
    highlight = null
  }: { threadRootEventId: string; highlight?: PendingHighlight | null } = $props();
</script>

<div data-testid="thread-pane-root-id">{threadRootEventId}</div>
<div data-testid="thread-pane-highlight-id">{highlight?.eventId ?? ''}</div>
