<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import AppHeader from './AppHeader.svelte';
  import PaneHeader from './PaneHeader.svelte';

  const { Story } = defineMeta({
    title: 'UI/App header',
    component: AppHeader,
    tags: ['autodocs']
  });
</script>

<Story name="Unauthenticated shell" asChild>
  <div class="max-w-4xl rounded-lg bg-background p-2 ring-1 ring-border">
    <AppHeader />
  </div>
</Story>

<Story name="Narrow shell with input-aware height" asChild>
  <div class="w-80 max-w-full bg-background">
    <AppHeader />
  </div>
</Story>

<Story name="With pane header" asChild>
  <div class="max-w-4xl bg-background">
    <AppHeader />
    <PaneHeader title="Preferences" />
  </div>
</Story>

<Story name="Native window controls" asChild>
  <div
    class="relative max-w-4xl overflow-hidden rounded-xl bg-surface"
    style="--app-header-titlebar-x: 80px; --app-header-titlebar-width: calc(100vw - 80px)"
  >
    <div
      class="pointer-events-none absolute top-0 left-4 flex h-[var(--app-header-height)] items-center gap-2"
      aria-hidden="true"
    >
      <span class="size-3 rounded-full bg-red-400"></span>
      <span class="size-3 rounded-full bg-yellow-400"></span>
      <span class="size-3 rounded-full bg-green-400"></span>
    </div>
    <AppHeader />
  </div>
</Story>
