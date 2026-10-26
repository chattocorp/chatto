<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import CopyButton from './CopyButton.svelte';
  import ToastContainer from './toast/ToastContainer.svelte';

  const { Story } = defineMeta({
    title: 'UI/CopyButton',
    component: CopyButton,
    tags: ['autodocs']
  });
</script>

<Story name="Email address" asChild>
  <div class="inline-flex items-center gap-1.5">
    <span>member@example.test</span>
    <CopyButton value="member@example.test" />
  </div>
  <ToastContainer />
</Story>
