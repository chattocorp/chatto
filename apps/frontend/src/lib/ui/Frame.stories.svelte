<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Frame from './Frame.svelte';
  import AppHeader from './AppHeader.svelte';

  const { Story } = defineMeta({
    title: 'UI/Frame',
    component: Frame,
    tags: ['autodocs']
  });
</script>

<Story name="Application surface" asChild>
  <div
    class="flex h-80 max-w-4xl bg-surface desktop-presentation:app-frame-shell desktop-presentation:p-3"
  >
    <Frame>
      <aside class="relative z-50 w-48 border-e border-border bg-surface p-4">
        <p class="font-medium">Spaces</p>
        <p class="mt-3 text-sm text-muted">Product</p>
        <p class="mt-2 text-sm text-muted">Support</p>
      </aside>
      <main class="flex-1 p-6">
        <h1 class="text-xl font-semibold">General</h1>
        <p class="mt-2 text-muted">
          The content area sits above the frame in light mode and below it in dark mode, including
          panes with their own background.
        </p>
      </main>
    </Frame>
  </div>
</Story>

<Story name="Narrow application frame" asChild>
  <div
    class="flex h-[80vh] w-full max-w-125 flex-col bg-surface desktop-presentation:px-3 desktop-presentation:pb-3"
  >
    <AppHeader />
    <Frame class="flex-col">
      <h1 class="p-4 text-lg font-semibold">General</h1>
      <main class="flex min-h-0 flex-1 flex-col justify-end gap-4 overflow-y-auto py-4">
        <div class="message-row">
          <p>A narrow window keeps its mouse controls and app frame.</p>
        </div>
        <div class="message-row"><p>Touch presentation also needs a narrow viewport.</p></div>
      </main>
      <div class="m-2 chat-input-surface p-3 text-muted">Type a message…</div>
    </Frame>
  </div>
</Story>
