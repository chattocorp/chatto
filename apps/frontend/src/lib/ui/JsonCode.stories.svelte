<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import JsonCode from './JsonCode.svelte';
  import Panel from './Panel.svelte';

  const { Story } = defineMeta({
    title: 'UI/JsonCode',
    component: JsonCode,
    tags: ['autodocs']
  });

  const text = JSON.stringify(
    {
      id: 'example-event',
      messageBody: {
        roomId: 'example-room',
        body: { encryptionVersion: 2, enabled: true, deletedAt: null },
        assetIds: ['example-asset'],
        content: '<img src="example" onerror="alert(1)">',
        encryptedBody: 'abcdefghijklmnopqrstuvwxyz0123456789'.repeat(6)
      }
    },
    null,
    2
  );
</script>

<Story name="Event payload" asChild>
  <Panel title="Payload"><JsonCode {text} /></Panel>
</Story>

<Story name="Narrow panel" asChild>
  <div class="w-80"><Panel title="Payload"><JsonCode {text} /></Panel></div>
</Story>
