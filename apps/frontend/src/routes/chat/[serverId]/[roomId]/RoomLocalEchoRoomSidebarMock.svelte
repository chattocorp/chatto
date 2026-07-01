<script lang="ts">
  let {
    maximized = false,
    onToggleMaximized
  }: {
    maximized?: boolean;
    onToggleMaximized?: () => void;
  } = $props();
</script>

<button
  type="button"
  data-testid="toggle-maximized-call"
  data-maximized={String(maximized)}
  onclick={() => onToggleMaximized?.()}
>
  Toggle maximized call
</button>
