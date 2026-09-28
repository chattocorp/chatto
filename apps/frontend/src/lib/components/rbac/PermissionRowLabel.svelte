<!--
@component
Permission matrix row label with an info button that opens the row's help
dialog. The button stays visible on every device, so touch users can reach
the explanation too.
-->
<script lang="ts">
  let {
    label,
    highlighted,
    helpLabel,
    onhelp
  }: {
    /** Visible row name, usually the permission ID. */
    label: string;
    /** The row is highlighted because a cell in it has hover or focus. */
    highlighted: boolean;
    /** Accessible name of the info button. */
    helpLabel: string;
    onhelp: () => void;
  } = $props();
</script>

<span class="inline-flex items-center gap-1.5">
  <span
    data-testid="permission-name"
    class={['text-sm whitespace-nowrap', highlighted ? 'text-action' : '']}>{label}</span
  >
  <button type="button" class="mini-icon-action" aria-label={helpLabel} onclick={onhelp}>
    <span class="iconify icon-[uil--info-circle] text-base" aria-hidden="true"></span>
  </button>
</span>
