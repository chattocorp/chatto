<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  const componentDescription = `
  Shared Tailwind utilities that are part of Chatto's visual system but are not
  owned by a single Svelte component. Reach for these before repeating a long
  shell, menu, or icon-button class string in feature code.
  `.trim();

  const { Story } = defineMeta({
    title: 'Foundations/Utilities',
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component: componentDescription
        }
      }
    }
  });
</script>

<Story
  name="composer mode surface"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          '`composer-mode-surface` keeps simple mode unadorned and adds an action-gradient ring when `data-composer-mode="rich"` is present.'
      }
    }
  }}
>
  <div class="flex max-w-2xl flex-col gap-5">
    <p class="max-w-prose text-sm text-muted">
      The composer uses the same surface in both modes. Rich mode adds emphasis without changing
      layout, and fades cleanly as the composer moves between these states.
    </p>
    <div class="grid gap-5 sm:grid-cols-2">
      <section>
        <p class="mb-2 text-sm font-medium text-muted">Simple mode</p>
        <div
          class="composer-mode-surface flex min-h-12 items-center rounded-xl bg-surface px-4"
          data-composer-mode="simple"
        >
          <span class="text-muted">Write a message…</span>
        </div>
      </section>
      <section>
        <p class="mb-2 text-sm font-medium text-muted">Rich mode</p>
        <div
          class="composer-mode-surface flex min-h-12 items-center rounded-xl bg-surface px-4"
          data-composer-mode="rich"
        >
          <span class="text-text">A message with multiple paragraphs</span>
        </div>
      </section>
    </div>
  </div>
</Story>

<Story
  name="link"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Use `link` for inline navigation in body text, auth flows, and secondary chrome copy.'
      }
    }
  }}
>
  <div class="flex flex-col gap-3">
    <p class="max-w-prose text-sm text-muted">
      Inline link styling. <code>link</code> applies <code>text-action hover:underline</code> — sky
      (our action tone) so links stand out against the slate/neutral body copy, with an underline
      only on hover for a quiet resting state. Use this anywhere an <code>&lt;a&gt;</code>
      should read as a link in body or chrome text.
    </p>
    <div class="flex flex-col gap-3 rounded border border-input-border bg-surface p-4 text-text">
      <p>
        Forgot your password? <a href="https://example.com/reset" class="link"
          >Request a reset link</a
        >.
      </p>
      <p>
        Head to the <a href="https://example.com/overview" class="link">Overview</a> to browse
        rooms, or jump back to the
        <a href="https://example.com/back" class="link">← previous page</a>.
      </p>
      <p class="text-sm text-muted">
        Combines cleanly with text-size utilities:
        <a href="https://example.com/roles" class="text-sm link">Edit role</a>.
      </p>
    </div>
  </div>
</Story>

<Story
  name="panel shell"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          '`panel-shell` is the canonical bordered surface. Add `panel-shell-raised` only when the shell needs to sit above a scrolling page surface.'
      }
    }
  }}
>
  <div class="flex max-w-3xl flex-col gap-4">
    <p class="max-w-prose text-sm text-muted">
      Use <code>panel-shell</code> for admin panels, directory group cards, and other durable
      content containers. Combine with <code>panel-header</code> when the surface has a
      structured header.
    </p>

    <div class="grid gap-4 md:grid-cols-2">
      <section class="overflow-hidden panel-shell">
        <header class="rounded-t-xl panel-header px-4 py-3">
          <h3 class="font-semibold">Flat shell</h3>
          <p class="text-sm text-muted">Embedded in a dense view.</p>
        </header>
        <div class="p-4 text-sm text-muted">
          Bordered, token-backed background, no elevation.
        </div>
      </section>

      <section class="overflow-hidden panel-shell panel-shell-raised">
        <header class="rounded-t-xl panel-header px-4 py-3">
          <h3 class="font-semibold">Raised shell</h3>
          <p class="text-sm text-muted">Used by admin panels.</p>
        </header>
        <div class="p-4 text-sm text-muted">
          Same shell with the approved quiet shadow.
        </div>
      </section>
    </div>
  </div>
</Story>

<Story
  name="choice row"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          '`choice-row` is the settings radio-row pattern for one-of-many choices. Pair it with `choice-indicator` for the leading dot.'
      }
    }
  }}
>
  <div class="flex max-w-md flex-col gap-3">
    <p class="max-w-prose text-sm text-muted">
      Use this utility family for settings lists where each row selects one value and carries a
      label plus optional helper text.
    </p>

    <div class="flex flex-col gap-2" role="radiogroup" aria-label="Display theme">
      <button type="button" class="choice-row choice-row-selected" role="radio" aria-checked="true">
        <span class="choice-indicator choice-indicator-selected">
          <span class="choice-indicator-dot"></span>
        </span>
        <span>
          <span class="font-medium">System</span>
          <span class="block text-sm text-muted">Follow this device.</span>
        </span>
      </button>

      <button type="button" class="choice-row" role="radio" aria-checked="false">
        <span class="choice-indicator"></span>
        <span>
          <span>Dark</span>
          <span class="block text-sm text-muted">Use the dark theme.</span>
        </span>
      </button>
    </div>
  </div>
</Story>

<Story
  name="surface box"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          '`surface-box` is the compact framed background for inline rows inside dialogs, forms, and settings. Add layout and padding at the call site.'
      }
    }
  }}
>
  <div class="flex max-w-md flex-col gap-3">
    <p class="max-w-prose text-sm text-muted">
      Use this for small durable rows that need a boundary but are not full panels or alert-style
      hints.
    </p>

    <div class="flex items-center gap-3 surface-box p-3">
      <span
        class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-surface-emphasized text-muted"
      >
        <span class="iconify uil--user"></span>
      </span>
      <span class="min-w-0 flex-1">
        <span class="block truncate font-medium">Taylor Morgan</span>
        <span class="block truncate text-sm text-muted">@taylor</span>
      </span>
    </div>
  </div>
</Story>

<Story
  name="app-header-icon"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Use `app-header-icon` for icon-only controls in the global app header so tap targets and alignment stay consistent.'
      }
    }
  }}
>
  <div class="flex flex-col gap-3">
    <p class="max-w-prose text-sm text-muted">
      Icon button used in the global app header (hamburger, notifications, quick switcher). 44px tap
      target with a negative margin so the visual footprint stays tight, plus a hover text-color
      shift and a subtle <code>active:bg-surface-emphasized</code> press state.
    </p>
    <div
      class="flex items-center gap-3 rounded border border-input-border bg-surface p-2 text-muted"
    >
      <button type="button" class="app-header-icon" aria-label="Menu">
        <span class="iconify text-xl uil--bars"></span>
      </button>
      <button type="button" class="relative app-header-icon" aria-label="Notifications">
        <span class="iconify text-lg uil--bell"></span>
      </button>
      <button type="button" class="app-header-icon" aria-label="Quick switcher">
        <span class="iconify text-lg uil--apps"></span>
      </button>
    </div>
  </div>
</Story>

<Story
  name="icon action"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          '`icon-action` is the compact inline icon button for row/input affordances. Use HeaderIconButton for toolbar actions instead.'
      }
    }
  }}
>
  <div class="flex max-w-md flex-col gap-3">
    <p class="max-w-prose text-sm text-muted">
      Use for low-emphasis actions that sit inside another element: copy an ID, clear a field,
      dismiss a row.
    </p>
    <div class="flex items-center gap-2 surface-box p-3">
      <code class="text-xs">USR-7Q9M2N</code>
      <button type="button" class="icon-action" title="Copy to clipboard">
        <span class="iconify uil--copy"></span>
      </button>
      <button type="button" class="ml-auto icon-action" title="Dismiss">
        <span class="iconify uil--times"></span>
      </button>
    </div>
  </div>
</Story>
