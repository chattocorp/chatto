<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ActivityIndicator from './ActivityIndicator.svelte';
  import Pill from './Pill.svelte';

  const { Story } = defineMeta({ title: 'UI/ActivityIndicator', component: ActivityIndicator });
</script>

<Story name="In a status pill" asChild>
  <div class="flex items-center gap-3">
    <Pill tone="action">
      <span class="inline-flex items-center gap-1.5"><ActivityIndicator /> Running</span>
    </Pill>
    <span class="inline-flex items-center gap-1.5 text-sm text-muted">
      <ActivityIndicator /> Working
    </span>
  </div>
</Story>
