<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Button from './Button.svelte';
  import Select from './Select.svelte';

  const componentDescription = `
    Use Button for committed actions, form submits, destructive commands, and link-styled calls to
    action. Keep modal footer actions visible and horizontal, using secondary for cancel and the
    strongest applicable tone for the action. Labelled buttons share the rounded-xl radius
    of chat input surfaces; icon-only buttons keep rounded-md corners. Filled buttons have a
    quiet shell lighting without a cast shadow. Ghost buttons stay flat; disabled buttons lose the raised finish.
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
          'Filled buttons share the soft rim and lighting of the composer. Fill changes give press feedback. Secondary buttons keep a quiet surface fill; ghost buttons stay flat and use an action tint on hover.'
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

<Story name="Shared quiet depth" asChild>
  <div class="flex flex-col gap-6">
    {#each [{ label: 'Flat', strength: 0, width: 1 }, { label: 'Kinda 3D', strength: 0.75, width: 1 }, { label: 'Very 3D', strength: 1.75, width: 1.5 }] as mode (mode.label)}
      <section
        class="flex flex-col gap-3"
        style:--depth-strength={mode.strength}
        style:--depth-width={mode.width}
      >
        <h2 class="font-semibold">{mode.label}</h2>
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex chat-input-surface items-center px-4 text-muted">Composer surface</div>
          <button type="button" class="shell-action">Start call</button>
          <Button variant="secondary">Cancel</Button>
          <Button>Current Server</Button>
          <Button variant="danger">All Servers</Button>
        </div>
        <div class="flex flex-wrap items-end gap-3">
          <Select
            id={`quiet-depth-${mode.strength}`}
            label="Visibility"
            value="public"
            options={[
              { value: 'public', label: 'Public' },
              { value: 'private', label: 'Private' }
            ]}
          />
          <Button variant="ghost">Save draft</Button>
          <Button href="#">Button link</Button>
          <Button disabled>Disabled</Button>
          <Button loading loadingText="Saving…">Save</Button>
          <Button href="#" disabled>Disabled link</Button>
        </div>
      </section>
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
