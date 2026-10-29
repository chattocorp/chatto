<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import QuickSwitcher from './QuickSwitcher.svelte';

  const { Story } = defineMeta({
    title: 'Navigation/Quick finder',
    component: QuickSwitcher
  });
</script>

<script lang="ts">
  import { onMount } from 'svelte';
  import { quickSwitcher } from '$lib/state/globals.svelte';
  import { toast } from '$lib/ui/toast';

  onMount(() => {
    quickSwitcher.open();
    return () => quickSwitcher.close();
  });
</script>

<Story name="Search palette" asChild>
  <button type="button" class="btn-secondary" onclick={() => quickSwitcher.open()}
    >Open quick finder</button
  >
  <QuickSwitcher
    onNextUnread={async () => {
      toast.info('No unread notifications');
    }}
  />
</Story>
