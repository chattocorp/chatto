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
  name="selectable list"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          '`selectable-list` and `selectable-list-item` provide the canonical inset, independently rounded treatment for navigable or actionable records.'
      }
    }
  }}
>
  <div class="selectable-list max-w-md rounded-lg bg-background">
    <a href="https://example.com/alice" class="flex items-center gap-3 selectable-list-item p-3">
      <span
        class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-surface text-sm font-semibold"
        >A</span
      >
      <span class="min-w-0 flex-1">
        <span class="block truncate font-medium">Alice</span>
        <span class="block truncate text-start text-sm text-muted"><bdi dir="ltr">@alice</bdi></span
        >
      </span>
      <span class="iconify icon-[uil--angle-right] text-muted rtl:-scale-x-100" aria-hidden="true"
      ></span>
    </a>
    <a href="https://example.com/bob" class="flex items-center gap-3 selectable-list-item p-3">
      <span
        class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-surface text-sm font-semibold"
        >B</span
      >
      <span class="min-w-0 flex-1">
        <span class="block truncate font-medium">Bob</span>
        <span class="block truncate text-start text-sm text-muted"><bdi dir="ltr">@bob</bdi></span>
      </span>
      <span class="iconify icon-[uil--angle-right] text-muted rtl:-scale-x-100" aria-hidden="true"
      ></span>
    </a>
  </div>
</Story>

<Story
  name="RTL review fixture"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'A compact review fixture for logical alignment, isolated Latin identifiers, mixed-script messages, and directional controls. Use the Direction toolbar to compare LTR and RTL across every story.'
      }
    }
  }}
>
  <div class="flex w-full max-w-xl flex-col gap-5 rounded-lg bg-background p-4" dir="rtl">
    <div class="selectable-list rounded-lg bg-background">
      <button
        type="button"
        class="flex w-full items-center gap-3 selectable-list-item p-3 text-start"
      >
        <span
          class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-surface text-sm font-semibold"
          >أ</span
        >
        <span class="min-w-0 flex-1">
          <bdi class="block truncate font-medium">أليس سميث</bdi>
          <span class="block truncate text-start text-sm text-muted"
            ><bdi dir="ltr">@alice-smith</bdi></span
          >
        </span>
        <span class="iconify icon-[uil--angle-right] text-muted rtl:-scale-x-100" aria-hidden="true"
        ></span>
      </button>
      <button
        type="button"
        class="flex w-full items-center gap-3 selectable-list-item p-3 text-start"
      >
        <span
          class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-surface text-sm font-semibold"
          >ד</span
        >
        <span class="min-w-0 flex-1">
          <bdi class="block truncate font-medium">דנה כהן</bdi>
          <span class="block truncate text-start text-sm text-muted"
            ><bdi dir="ltr">@dana-cohen</bdi></span
          >
        </span>
        <span class="iconify icon-[uil--angle-right] text-muted rtl:-scale-x-100" aria-hidden="true"
        ></span>
      </button>
    </div>

    <div class="flex items-center gap-3 px-2 text-sm text-muted">
      <div class="flex -space-x-1.5 rtl:space-x-reverse">
        <span class="grid size-7 place-items-center rounded-full bg-surface">A</span>
        <span class="grid size-7 place-items-center rounded-full bg-surface">B</span>
      </div>
      <span>انضمّ <bdi>Alice</bdi> و<bdi>Bob</bdi> إلى الغرفة</span>
    </div>

    <div class="rounded-lg bg-surface p-3 text-start text-text">
      <p dir="auto">مرحبا يا <bdi>Alice</bdi>، موعدنا الساعة 10:30 في room-42.</p>
      <p dir="auto" class="mt-2">שלום <bdi>Bob</bdi> — https://example.com/room-42</p>
    </div>

    <div class="flex min-h-12 items-center gap-3 rounded-xl bg-surface px-3">
      <button type="button" class="icon-action" aria-label="إرسال">
        <span class="iconify icon-[uil--message] rtl:-scale-x-100" aria-hidden="true"></span>
      </button>
      <span class="min-w-0 flex-1 text-start text-muted" dir="auto">اكتب رسالة…</span>
      <button type="button" class="icon-action" aria-label="إرفاق ملف">
        <span class="iconify icon-[uil--paperclip]" aria-hidden="true"></span>
      </button>
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
          '`panel-shell` is the canonical bordered surface frame. Add `panel-shell-raised` only when the shell needs to sit above a scrolling page surface.'
      }
    }
  }}
>
  <div class="flex max-w-3xl flex-col gap-4">
    <p class="max-w-prose text-sm text-muted">
      Use <code>panel-shell</code> for admin panels, directory group cards, and other durable
      content containers. Combine it with <code>panel-header</code>, then wrap content in a
      side-and-bottom frame and <code>panel-inset</code> work plane.
    </p>

    <div class="grid gap-4 md:grid-cols-2">
      <section class="overflow-hidden panel-shell">
        <header class="panel-header px-6 py-3">
          <h3 class="font-semibold">Flat shell</h3>
          <p class="text-sm text-muted">Embedded in a dense view.</p>
        </header>
        <div class="px-1 pb-1">
          <div class="panel-inset p-4 text-sm text-muted">
            Bordered, token-backed background, no elevation.
          </div>
        </div>
      </section>

      <section class="overflow-hidden panel-shell panel-shell-raised">
        <header class="panel-header px-6 py-3">
          <h3 class="font-semibold">Raised shell</h3>
          <p class="text-sm text-muted">Used by admin panels.</p>
        </header>
        <div class="px-1 pb-1">
          <div class="panel-inset p-4 text-sm text-muted">
            Same shell with the approved quiet shadow.
          </div>
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
        <span class="iconify icon-[uil--user]"></span>
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
        <span class="iconify icon-[uil--bars] text-xl"></span>
      </button>
      <button type="button" class="relative app-header-icon" aria-label="Notifications">
        <span class="iconify icon-[uil--bell] text-lg"></span>
      </button>
      <button type="button" class="app-header-icon" aria-label="Quick switcher">
        <span class="iconify icon-[uil--apps] text-lg"></span>
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
          '`icon-action` is the compact icon button with a standard hit area for row/input affordances. Use HeaderIconButton for toolbar actions instead.'
      }
    }
  }}
>
  <div class="flex max-w-md flex-col gap-3">
    <p class="max-w-prose text-sm text-muted">
      Use for low-emphasis actions that sit inside another element and still need a standard hit
      area, such as clearing a field or dismissing a row.
    </p>
    <div class="flex items-center gap-2 surface-box p-3">
      <span class="text-muted">Active filter</span>
      <button type="button" class="ml-auto icon-action" title="Dismiss">
        <span class="iconify icon-[uil--times]"></span>
      </button>
    </div>
  </div>
</Story>

<Story
  name="mini icon button"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          '`mini-icon-action` is an unframed, icon-sized action that changes only text colour on hover and focus. Reserve it for a subordinate command directly beside the value it affects.'
      }
    }
  }}
>
  <div class="flex max-w-md flex-col gap-3">
    <p class="max-w-prose text-sm text-muted">
      Use for a small secondary affordance whose associated value supplies the surrounding context.
    </p>
    <div class="inline-flex items-center gap-1.5 self-start surface-box p-3">
      <code class="text-xs">USR-7Q9M2N</code>
      <button type="button" class="mini-icon-action" title="Copy to clipboard">
        <span class="iconify icon-[uil--copy]"></span>
      </button>
    </div>
  </div>
</Story>
