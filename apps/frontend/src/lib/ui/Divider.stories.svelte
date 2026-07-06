<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Divider from './Divider.svelte';

  const { Story } = defineMeta({
    title: 'UI/Divider',
    component: Divider,
    tags: ['autodocs']
  });
</script>

<Story name="Plain" asChild>
  <div class="max-w-prose">
    <p>Above the divider.</p>
    <Divider />
    <p>Below the divider.</p>
  </div>
</Story>

<Story name="With label" asChild>
  <div class="max-w-prose">
    <p>Above</p>
    <Divider label="or" />
    <p>Below</p>
  </div>
</Story>
