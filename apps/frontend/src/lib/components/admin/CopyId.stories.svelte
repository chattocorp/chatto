<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import CopyId from './CopyId.svelte';

  const componentDescription = `
  Inline admin helper for displaying opaque identifiers with a copy action.
  Use it inside tables, detail pages, and diagnostics where the full ID should
  remain visible but easy to copy.
  `.trim();

  const { Story } = defineMeta({
    title: 'Admin/CopyId',
    component: CopyId,
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
  name="Default"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The copy button uses the compact inline icon-action treatment and stops row-click propagation.'
      }
    }
  }}
>
  <div class="inline-flex surface-box p-3">
    <CopyId value="USR-7Q9M2N" />
  </div>
</Story>
