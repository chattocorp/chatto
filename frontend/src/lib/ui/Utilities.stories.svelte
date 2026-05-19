<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  const { Story } = defineMeta({
    title: 'Foundations/Utilities',
    tags: ['autodocs']
  });
</script>

<Story name="link" asChild>
  <div class="flex flex-col gap-3">
    <p class="text-sm text-muted max-w-prose">
      Inline link styling. <code>link</code> applies <code>text-accent hover:underline</code> —
      violet (our accent tone) so links stand out against the slate/neutral body copy, with an
      underline only on hover for a quiet resting state. Use this anywhere an <code>&lt;a&gt;</code>
      should read as a link in body or chrome text.
    </p>
    <div class="flex flex-col gap-3 rounded border border-input-border bg-surface p-4 text-text">
      <p>
        Forgot your password? <a href="#" class="link">Request a reset link</a>.
      </p>
      <p>
        Head to the <a href="#" class="link">Overview</a> to browse rooms, or jump back to the
        <a href="#" class="link">← previous page</a>.
      </p>
      <p class="text-sm text-muted">
        Combines cleanly with text-size utilities:
        <a href="#" class="link text-sm">Edit role</a>.
      </p>
    </div>
  </div>
</Story>

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
