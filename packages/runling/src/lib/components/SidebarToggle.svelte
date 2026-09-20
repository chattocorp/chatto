<script lang="ts">
  let { expanded, onclick }: { expanded: boolean; onclick: () => void } =
    $props();
  let label = $derived(expanded ? "Collapse sidebars" : "Expand sidebars");
</script>

<button
  class="btn btn-square btn-sm size-9 shrink-0"
  type="button"
  {onclick}
  aria-label={label}
  title={`${label} (Cmd/Ctrl+B)`}
  aria-keyshortcuts="Meta+b Control+b"
  aria-expanded={expanded}
  aria-controls="webhook-sidebar runs-sidebar"
>
  <svg
    class="stroke-current stroke-2 [stroke-linecap:round] [stroke-linejoin:round]"
    width="20"
    height="20"
    viewBox="0 0 24 24"
    fill="none"
    aria-hidden="true"
  >
    <rect x="3" y="4" width="18" height="16" rx="3" />
    <path d="M9 4v16" />
    <path d={expanded ? "m16 9-3 3 3 3" : "m13 9 3 3-3 3"} />
  </svg>
</button>
