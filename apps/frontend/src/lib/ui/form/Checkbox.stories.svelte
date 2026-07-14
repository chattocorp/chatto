<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { UNIVERSAL_ROOM_HELP_TEXT } from '$lib/utils/roomCopy';
  import Checkbox from './Checkbox.svelte';

  const componentDescription = `
    Use Checkbox for independent boolean settings. Prefer immediate-save behavior for settings where
    one checkbox maps to one backend change, and keep supporting text inside the component instead of
    building custom option rows.
  `.trim();

  const { Story } = defineMeta({
    title: 'Form/Checkbox',
    component: Checkbox,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<script lang="ts">
  let a = $state(false);
  let b = $state(true);
  let c = $state(false);
  let d = $state(false);
  let loadingValue = $state(true);
</script>

<Story
  name="Default"
  asChild
  parameters={{
    docs: {
      description: { story: 'Default boolean option with a visible label.' }
    }
  }}
>
  <Checkbox id="a" bind:checked={a} label="Send me email notifications" />
</Story>

<Story
  name="Pre-checked"
  asChild
  parameters={{
    docs: {
      description: { story: 'Checked state uses the shared action treatment.' }
    }
  }}
>
  <Checkbox id="b" bind:checked={b} label="Public room (visible in listings)" />
</Story>

<Story
  name="Option with help text"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Descriptions belong inside the checkbox component when they explain the specific option.'
      }
    }
  }}
>
  <Checkbox
    id="with-description"
    bind:checked={b}
    label="Universal room"
    description={UNIVERSAL_ROOM_HELP_TEXT}
  />
</Story>

<Story
  name="With error"
  asChild
  parameters={{
    docs: {
      description: { story: 'Errors are attached to the option they block.' }
    }
  }}
>
  <Checkbox
    id="c"
    bind:checked={c}
    label="I accept the terms"
    error="You must accept the terms to continue."
  />
</Story>

<Story name="Disabled" asChild>
  <Checkbox id="d" bind:checked={d} label="Disabled option" disabled />
</Story>

<Story name="Saving" asChild>
  <Checkbox
    id="loading"
    bind:checked={loadingValue}
    label="Community moderator"
    description="Role assignment is being updated."
    loading
  />
</Story>

<Story
  name="Group"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'Use a simple vertical stack when several independent settings belong together.'
      }
    }
  }}
>
  <div class="flex flex-col gap-2">
    <Checkbox id="g1" bind:checked={a} label="Mentions" />
    <Checkbox id="g2" bind:checked={b} label="Direct messages" />
    <Checkbox id="g3" bind:checked={c} label="Replies in my threads" />
    <Checkbox id="g4" bind:checked={d} label="All messages in subscribed rooms" />
  </div>
</Story>
