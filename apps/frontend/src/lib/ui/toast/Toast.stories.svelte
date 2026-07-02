<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Toast from './Toast.svelte';

  const componentDescription = `
    Transient toast notifications use the same compact menu shell as context menus and popups,
    while preserving tone-specific icons and optional single-action affordances.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/Toast',
    component: Toast,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<script lang="ts">
  const dismiss = () => {};
</script>

<Story name="Tones" asChild>
  <div
    class="flex min-h-72 flex-col items-end justify-end gap-2 rounded-lg border border-border bg-background p-6"
  >
    <Toast tone="success" message="Profile settings saved" onDismiss={dismiss} />
    <Toast tone="info" message="Connecting to the server..." onDismiss={dismiss} />
    <Toast tone="warning" message="Some changes could not be applied" onDismiss={dismiss} />
    <Toast tone="error" message="Failed to send message" onDismiss={dismiss} />
  </div>
</Story>

<Story name="Action" asChild>
  <div
    class="flex min-h-40 items-end justify-end rounded-lg border border-border bg-background p-6"
  >
    <Toast
      tone="info"
      message="A new version is available"
      action={{ label: 'Reload', onClick: () => {} }}
      onDismiss={dismiss}
    />
  </div>
</Story>
