<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ExpirySelect from './ExpirySelect.svelte';

  const componentDescription = `
  Form control for choosing an expiry timestamp from common presets or a custom
  date/time. Use it in moderation and administrative forms where "temporary vs
  indefinite" needs to be explicit.

  Bind \`value\` to receive an ISO timestamp or \`null\` for no expiry. Bind
  \`valid\` when the parent submit button should be disabled for an invalid
  custom date.
  `.trim();

  const { Story } = defineMeta({
    title: 'Form/ExpirySelect',
    component: ExpirySelect,
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

<script lang="ts">
  let value = $state<string | null>(null);
  let valid = $state(true);
</script>

<Story
  name="Default"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The default state is indefinite. Selecting a preset writes an ISO timestamp; selecting custom reveals a date/time field.'
      }
    }
  }}
>
  <div class="max-w-md">
    <ExpirySelect id="expiry-default" bind:value bind:valid />
    <p class="mt-3 text-xs text-muted">
      Current value: <code>{value ?? 'null'}</code>
      · valid: <code>{String(valid)}</code>
    </p>
  </div>
</Story>

<Story
  name="Disabled"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Disabled expiry controls follow the same input disabled treatment as other form fields.'
      }
    }
  }}
>
  <div class="max-w-md">
    <ExpirySelect id="expiry-disabled" value={null} disabled />
  </div>
</Story>
