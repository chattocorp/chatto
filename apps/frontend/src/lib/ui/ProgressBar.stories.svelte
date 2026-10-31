<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ProgressBar from './ProgressBar.svelte';

  const { Story } = defineMeta({ title: 'UI/Progress bar', component: ProgressBar });
</script>

<Story name="Downloading" args={{ value: 42, label: 'Downloading update' }} />
<Story name="Starting" args={{ value: 0, label: 'Downloading update' }} />
<Story name="Complete" args={{ value: 100, label: 'Download complete' }} />
<Story name="Narrow" asChild>
  <div class="w-60 max-w-full">
    <ProgressBar value={42} label="Downloading update" />
  </div>
</Story>
