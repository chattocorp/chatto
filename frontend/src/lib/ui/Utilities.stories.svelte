<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  const { Story } = defineMeta({
    title: 'Foundations/Utilities',
    tags: ['autodocs']
  });
</script>

<Story name="app-header-icon" asChild>
  <div class="flex flex-col gap-3">
    <p class="text-sm text-muted max-w-prose">
      Icon button used in the global app header (hamburger, notifications, quick switcher).
      44px tap target with a negative margin so the visual footprint stays tight, plus a
      hover text-color shift and a subtle <code>active:bg-surface-200</code> press state.
    </p>
    <div class="flex items-center gap-3 rounded border border-input-border bg-surface p-2 text-muted">
      <button type="button" class="app-header-icon" aria-label="Menu">
        <span class="iconify text-xl uil--bars"></span>
      </button>
      <button type="button" class="app-header-icon relative" aria-label="Notifications">
        <span class="iconify text-lg uil--bell"></span>
      </button>
      <button type="button" class="app-header-icon" aria-label="Quick switcher">
        <span class="iconify text-lg uil--apps"></span>
      </button>
    </div>
  </div>
</Story>
