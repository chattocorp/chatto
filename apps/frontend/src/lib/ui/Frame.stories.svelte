<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Frame from './Frame.svelte';

  const { Story } = defineMeta({
    title: 'UI/Frame',
    component: Frame,
    tags: ['autodocs']
  });
</script>

<Story name="Application surface" asChild>
  <div class="flex h-80 max-w-4xl bg-surface p-4 md:app-frame-shell">
    <Frame>
      <aside class="w-48 border-e border-border bg-surface p-4">
        <p class="font-medium">Spaces</p>
        <p class="mt-3 text-sm text-muted">Product</p>
        <p class="mt-2 text-sm text-muted">Support</p>
      </aside>
      <main class="flex-1 p-6">
        <h1 class="text-xl font-semibold">General</h1>
        <p class="mt-2 text-muted">The content area sits above the frame in light mode and below it in dark mode, including panes with their own background.</p>
      </main>
    </Frame>
  </div>
</Story>
