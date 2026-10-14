<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ShowOnceCredentialDialog from './ShowOnceCredentialDialog.svelte';

  const { Story } = defineMeta({
    title: 'Bots/ShowOnceCredentialDialog',
    component: ShowOnceCredentialDialog,
    args: {
      visible: true,
      value: 'example-credential',
      title: 'New credential',
      warning: 'Save this credential now. It is shown only once.',
      copiedMessage: 'Copied'
    }
  });
</script>

<Story name="Default" />
