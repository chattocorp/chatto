<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import HelpTooltip from './HelpTooltip.svelte';

  const componentDescription = `
  Inline help affordance for optional explanatory copy. Use it next to labels,
  permission names, and compact technical concepts where persistent visible
  helper text would make the interface too dense.

  The trigger is an info icon. Hover/focus opens it transiently; click/tap pins
  it open. The popover uses \`FloatingPopover\`, so it escapes clipping and
  stacking contexts.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/HelpTooltip',
    component: HelpTooltip,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component: componentDescription
        }
      }
    }
  });
</script>

<Story
  name="Beside label"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Place HelpTooltip inline with labels or table headings when the extra context is useful but not required to complete the task.'
      }
    }
  }}
>
  <div
    class="flex min-h-32 items-start gap-2 rounded border border-input-border bg-surface p-6 text-text"
  >
    <span class="text-sm font-medium">Can manage rooms</span>
    <HelpTooltip label="About room management">
      Create, rename, archive, and delete rooms in this scope.
    </HelpTooltip>
  </div>
</Story>
