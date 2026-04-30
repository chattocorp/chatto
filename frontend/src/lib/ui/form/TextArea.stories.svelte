<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import TextArea from './TextArea.svelte';

  const { Story } = defineMeta({
    title: 'Form/TextArea',
    component: TextArea,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let value = $state('');
  let withCounter = $state('A short bio.');
</script>

<Story name="Default" asChild>
  <div class="max-w-md">
    <TextArea id="default" label="Description" bind:value placeholder="What is this room about?" />
  </div>
</Story>

<Story name="With description and rows" asChild>
  <div class="max-w-md">
    <TextArea
      id="rows"
      label="Welcome message"
      bind:value
      rows={6}
      description="Markdown is supported."
    />
  </div>
</Story>

<Story name="With max length" asChild>
  <div class="max-w-md">
    <TextArea id="max" label="Bio" bind:value={withCounter} maxlength={140} rows={3} />
    <p class="mt-1 text-xs text-muted">{withCounter.length} / 140</p>
  </div>
</Story>

<Story name="With error" asChild>
  <div class="max-w-md">
    <TextArea id="err" label="Description" bind:value error="Description is required." />
  </div>
</Story>
