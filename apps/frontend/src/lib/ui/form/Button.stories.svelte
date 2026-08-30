<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Button from './Button.svelte';

  const componentDescription = `
    Use Button for committed actions, form submits, destructive commands, and link-styled calls to
    action. Keep modal footer actions visible and horizontal, using secondary for cancel and the
    strongest applicable tone for the action.
  `.trim();

  const { Story } = defineMeta({
    title: 'Form/Button',
    component: Button,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<script lang="ts">
  const variants = [
    'action',
    'neutral',
    'secondary',
    'ghost',
    'warning',
    'danger',
    'danger-secondary'
  ] as const;
  const sizes = ['sm', 'md', 'lg'] as const;
</script>

<Story
  name="Variants"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Use action for the recommended flow action, neutral for neutral emphasis, secondary for cancellation, warning/danger for risky actions, danger-secondary for a quiet destructive action, and ghost only for low-emphasis commands.'
      }
    }
  }}
>
  <div class="flex flex-wrap items-center gap-3">
    {#each variants as variant (variant)}
      <Button {variant}>{variant}</Button>
    {/each}
  </div>
</Story>

<Story
  name="Tonal hierarchy"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Buttons use flat fills and tonal borders. Secondary buttons use a quiet surface fill. Ghost buttons use an action tint on hover.'
      }
    }
  }}
>
  <div class="flex flex-wrap items-center gap-3 rounded-lg bg-surface p-5">
    <Button>Send message</Button>
    <Button variant="secondary">Sign in</Button>
    <Button variant="ghost">Save draft</Button>
  </div>
</Story>

<Story
  name="Sizes"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Use md by default. Use sm in dense tables/toolbars and lg only when the surrounding layout has matching scale.'
      }
    }
  }}
>
  <div class="flex flex-wrap items-center gap-3">
    {#each sizes as size (size)}
      <Button {size}>{size}</Button>
    {/each}
  </div>
</Story>

<Story
  name="Loading"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Buttons own their busy state, including disabling interaction and preserving a stable label width.'
      }
    }
  }}
>
  <div class="flex flex-wrap items-center gap-3">
    <Button loading>Saving...</Button>
    <Button loading loadingText="Sending...">Send</Button>
    <Button variant="danger" loading>Deleting...</Button>
  </div>
</Story>

<Story
  name="Disabled"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Disabled buttons keep their semantic tone but reduce emphasis enough to communicate inactivity.'
      }
    }
  }}
>
  <div class="flex flex-wrap items-center gap-3">
    {#each variants as variant (variant)}
      <Button {variant} disabled>{variant}</Button>
    {/each}
  </div>
</Story>

<Story
  name="As link"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'Use href when navigation should look like a button while retaining anchor behavior.'
      }
    }
  }}
>
  <Button href="https://www.chatto.run" variant="secondary">Visit chatto.run</Button>
</Story>

<Story
  name="In a new tab"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Use opensInNewTab for an explicit handoff to another site. The component adds safe link attributes.'
      }
    }
  }}
>
  <Button href="https://www.chatto.run" opensInNewTab variant="secondary">
    <span>Open Chatto</span>
    <span class="iconify icon-[uil--external-link-alt]" aria-hidden="true"></span>
  </Button>
</Story>

<Story
  name="Full width"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Full-width buttons are reserved for narrow form flows where the action belongs to the whole column.'
      }
    }
  }}
>
  <div class="max-w-md">
    <Button fullWidth>Continue</Button>
  </div>
</Story>

<Story
  name="Icon only"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Icon-only buttons require an accessible label. Add a matching title when a concise hover hint is useful.'
      }
    }
  }}
>
  <div class="flex items-center gap-2">
    <Button variant="secondary" size="sm" label="Mark read" title="Mark read">
      <span class="iconify icon-[uil--check]" aria-hidden="true"></span>
    </Button>
    <Button variant="danger-secondary" size="sm" label="Delete" title="Delete">
      <span class="iconify icon-[uil--trash-alt]" aria-hidden="true"></span>
    </Button>
  </div>
</Story>
