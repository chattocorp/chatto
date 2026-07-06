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
  const variants = ['primary', 'accent', 'secondary', 'ghost', 'warning', 'danger'] as const;
  const sizes = ['sm', 'md', 'lg'] as const;
</script>

<Story
  name="Variants"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The supported semantic tones. Prefer primary or accent for positive flow actions, secondary for cancellation, warning/danger for risky actions, and ghost only for low-emphasis commands.'
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
