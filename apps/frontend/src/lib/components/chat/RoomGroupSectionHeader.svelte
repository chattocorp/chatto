<!--
@component

The disclosure heading of a collapsible sidebar section. `RoomGroupSection`
renders it above its items. Virtualized lists render it directly as a list
item, so their headings look and behave like every other sidebar section.
The caller owns the collapsed state (see `roomGroupCollapse.ts`), the
surrounding padding, and the section divider.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';

  interface Props {
    label: string;
    collapsed: boolean;
    ontoggle: () => void;
    /** Optional controls aligned to the end of the heading. */
    headerActions?: Snippet;
    /** Optional action that replaces the disclosure icon on hover or focus. */
    leadingOverlay?: Snippet;
    /** Optional right-click/long-press behavior for the heading. */
    contextMenuTrigger?: Attachment<HTMLElement>;
    /** Optional stable selector for the disclosure button. */
    testid?: string;
  }

  let {
    label,
    collapsed,
    ontoggle,
    headerActions,
    leadingOverlay,
    contextMenuTrigger,
    testid
  }: Props = $props();
</script>

<div
  class="group/section-header relative flex min-h-8 w-full min-w-0 items-center rounded-md text-muted transition-colors feedback-quick hover:text-text"
  {@attach contextMenuTrigger}
>
  <button
    type="button"
    onclick={ontoggle}
    aria-expanded={!collapsed}
    data-testid={testid}
    class="flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-md px-1 py-1 text-start text-xs font-semibold tracking-wider uppercase focus-visible:text-text focus-visible:outline-2 focus-visible:outline-action"
  >
    <span class="relative sidebar-icon">
      <span
        class={[
          'iconify icon-[uil--angle-right-b] transition-transform',
          leadingOverlay
            ? 'group-focus-within/section-header:opacity-0 group-hover/section-header:opacity-0'
            : '',
          collapsed ? 'rtl:-scale-x-100' : 'rotate-90'
        ]}
        aria-hidden="true"
        data-testid="room-group-disclosure-icon"
      ></span>
    </span>
    <span class="min-w-0 flex-1 truncate">{label}</span>
  </button>
  {#if leadingOverlay}
    <span class="pointer-events-none absolute start-0.5 top-1 h-6 w-6">
      {@render leadingOverlay()}
    </span>
  {/if}
  {#if headerActions}
    <div class="flex shrink-0 items-center gap-0.5">
      {@render headerActions()}
    </div>
  {/if}
</div>
