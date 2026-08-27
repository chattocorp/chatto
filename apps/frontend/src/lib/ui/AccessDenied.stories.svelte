<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import AccessDenied from './AccessDenied.svelte';

  const { Story } = defineMeta({
    title: 'UI/AccessDenied',
    component: AccessDenied,
    tags: ['autodocs']
  });
</script>

<Story name="Default" asChild>
  <div class="h-96">
    <AccessDenied />
  </div>
</Story>

<Story name="Custom return destination" asChild>
  <div class="h-96">
    <AccessDenied
      message="You do not have permission to manage this server."
      backHref="/chat"
      backLabel="Return to server"
    />
  </div>
</Story>
