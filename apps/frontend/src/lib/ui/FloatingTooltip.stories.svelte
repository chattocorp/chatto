<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import FloatingTooltip from './FloatingTooltip.svelte';

  const componentDescription = `
FloatingTooltip is a lightweight, non-interactive tooltip surface for short contextual labels.
It renders through \`FloatingPopover\`, so it appears in the browser top layer and avoids clipping
inside virtualized lists, sticky panes, or transformed parents.

### Composition

The trigger owns open/closed state. Read the trigger's \`getBoundingClientRect()\` when opening,
pass that rect as \`anchor\`, and wire the tooltip id to the trigger with \`aria-describedby\`.
When the trigger lives inside a measured or virtualized list, keep the tooltip mounted and toggle
the \`open\` prop instead of conditionally rendering it; that avoids child-list mutations inside
the measured row.

\`\`\`svelte
<FloatingTooltip open={!!tooltipAnchor} anchor={tooltipAnchor} id="reaction-tooltip">
  <span><strong class="font-semibold">Thumbs up</strong> · Alice, Bob</span>
</FloatingTooltip>
\`\`\`

Use \`HelpTooltip\` when the trigger is an info icon with pin/dismiss behavior. Use \`ContextMenu\`
for interactive floating content.
`.trim();

  const { Story } = defineMeta({
    title: 'UI/FloatingTooltip',
    component: FloatingTooltip,
    tags: ['autodocs'],
    parameters: { docs: { description: { component: componentDescription } } }
  });
</script>

<script lang="ts">
  let reactionAnchor = $state<{ top: number; bottom: number; left: number } | null>(null);
  const reactionTooltipId = 'storybook-reaction-tooltip';

  function showReactionTooltip(e: MouseEvent | FocusEvent) {
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    reactionAnchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
  }

  function hideReactionTooltip() {
    reactionAnchor = null;
  }
</script>

<Story
  name="Reaction detail"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'A compact, one-line tooltip for reaction metadata. The tooltip stays mounted and toggles `open`, matching virtualized-list usage.'
      }
    }
  }}
>
  <div class="flex min-h-32 items-center gap-3 rounded border border-input-border bg-surface p-6 text-text">
    <button
      type="button"
      class="meta-badge h-[25px] cursor-pointer gap-1 border-accent/50 px-2 text-sm text-muted"
      aria-describedby={reactionAnchor ? reactionTooltipId : undefined}
      onmouseenter={showReactionTooltip}
      onmouseleave={hideReactionTooltip}
      onfocus={showReactionTooltip}
      onblur={hideReactionTooltip}
    >
      <span aria-hidden="true">👍</span>
      <span class="text-xs" aria-hidden="true">2</span>
    </button>
    <span class="text-sm text-muted">Hover or focus the reaction badge.</span>

    <FloatingTooltip open={!!reactionAnchor} anchor={reactionAnchor} id={reactionTooltipId}>
      <span class="whitespace-nowrap">
        <strong class="font-semibold">Thumbs up</strong>
        <span> · Alice, Bob</span>
      </span>
    </FloatingTooltip>
  </div>
</Story>
