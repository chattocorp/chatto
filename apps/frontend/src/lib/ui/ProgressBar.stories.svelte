<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ProgressBar from './ProgressBar.svelte';
  import Button from './form/Button.svelte';

  const { Story } = defineMeta({ title: 'UI/Progress bar', component: ProgressBar });
</script>

<script lang="ts">
  let progress = $state(42);
</script>

<Story name="Downloading" args={{ value: 42, label: 'Downloading update' }} />
<Story name="Starting" args={{ value: 0, label: 'Downloading update' }} />
<Story name="Just started" args={{ value: 1.5, label: 'Downloading update' }} />
<Story name="Nearly complete" args={{ value: 97, label: 'Downloading update' }} />
<Story name="Complete" args={{ value: 100, label: 'Download complete' }} />
<Story name="Narrow" asChild>
  <div class="w-60 max-w-full">
    <ProgressBar value={42} label="Downloading update" />
  </div>
</Story>

<Story name="Interactive" asChild>
  <div class="flex max-w-2xl flex-col gap-4">
    <ProgressBar value={progress} label="Downloading update" />
    <div class="flex flex-wrap gap-2">
      <Button variant="secondary" onclick={() => (progress = 0)}>Reset</Button>
      <Button variant="secondary" onclick={() => (progress = 42)}>Download</Button>
      <Button variant="secondary" onclick={() => (progress = 100)}>Complete</Button>
    </div>
  </div>
</Story>
