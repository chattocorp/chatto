<script lang="ts">
  import type { Snippet } from 'svelte';
  import { shouldAutoFocus } from '$lib/utils/shouldAutoFocus';

  let {
    children,
    footer,
    visible = $bindable(false),
    title,
    size = 'md',
    describedBy,
    onclose
  }: {
    visible?: boolean;
    title?: string;
    size?: 'sm' | 'md' | 'lg';
    /** ID of an element that describes the dialog (forwarded to aria-describedby). */
    describedBy?: string;
    children: Snippet;
    footer?: Snippet;
    onclose?: () => void;
  } = $props();

  let dialogEl: HTMLDialogElement | undefined = $state();
  let closing = $state(false);

  // Stable per-instance id for the title (so screen readers announce it
  // when the dialog opens). $props.id() is hydration-safe.
  const dialogId = $props.id();
  const titleId = `${dialogId}-title`;

  const sizeClasses = {
    sm: 'w-100 max-w-[60vw]',
    md: 'w-150 max-w-[80vw]',
    lg: 'w-200 max-w-[90vw]'
  };

  $effect(() => {
    if (visible) {
      closing = false;
      dialogEl?.showModal();
      // showModal() naturally focuses the first focusable element, which
      // for our layout is the absolute-positioned Close (X) button — not
      // what users expect. Move focus to the first form field instead so
      // typing can begin immediately. Skipped on touch devices to avoid
      // popping the on-screen keyboard. A field with `autofocus` set wins
      // (the browser will already have focused it; we leave it alone).
      if (shouldAutoFocus()) {
        queueMicrotask(() => {
          if (!dialogEl) return;
          const fieldSelector =
            'input:not([type="hidden"]):not([disabled]),textarea:not([disabled]),select:not([disabled])';
          const active = document.activeElement;
          const alreadyOnField =
            active instanceof HTMLElement &&
            dialogEl.contains(active) &&
            active.matches(fieldSelector);
          if (alreadyOnField) return;
          dialogEl.querySelector<HTMLElement>(fieldSelector)?.focus();
        });
      }
    } else if (dialogEl?.open && !closing) {
      // Already closed via close() function
      dialogEl?.close();
    }
  });

  function handleNativeClose() {
    visible = false;
    closing = false;
    onclose?.();
  }

  function close() {
    if (!dialogEl?.open || closing) return;
    closing = true;
    // Wait for exit animation, then close
    setTimeout(() => {
      dialogEl?.close();
    }, 100);
  }
</script>

<dialog
  bind:this={dialogEl}
  onclose={handleNativeClose}
  oncancel={(e) => {
    // Always run our animated close path; never let the browser close the
    // dialog instantly without the fade-out.
    e.preventDefault();
    close();
  }}
  onclick={(e) => {
    // Use coordinate check instead of e.target to handle mobile keyboard viewport shifts
    const content = dialogEl?.firstElementChild as HTMLElement | null;
    if (!content) return;
    const rect = content.getBoundingClientRect();
    if (e.clientX < rect.left || e.clientX > rect.right || e.clientY < rect.top || e.clientY > rect.bottom) {
      close();
    }
  }}
  class="m-auto bg-transparent backdrop:bg-black/50 {sizeClasses[size]}"
  class:closing
  aria-labelledby={title ? titleId : undefined}
  aria-describedby={describedBy}
>
  <!--
    Only render the dialog's contents while the dialog is open (or playing
    its closing animation). This keeps form fields, submit buttons, and any
    other interactive children out of the surrounding page's DOM when the
    dialog is closed — important because callers often mount a Dialog
    permanently and toggle `visible`, and otherwise their submit buttons
    leak into selectors like `button[type="submit"]` on the host page.
  -->
  {#if visible || closing}
    <!-- Outer "tray" frame, mirroring the .menu utility used by ContextMenu/QuickSwitcher. -->
    <div class="rounded-lg border border-text/10 bg-surface-100 p-2 shadow-xl">
      <!-- Inner content well, mirroring .menu-section. -->
      <div class="relative max-h-[78vh] overflow-y-auto rounded-md bg-background p-3">
        <button
          onclick={close}
          class="absolute top-3 right-3 cursor-pointer text-text/50 transition-colors hover:text-text"
          aria-label="Close"
        >
          <span class="iconify text-xl uil--times"></span>
        </button>

        {#if title}
          <!-- px-2 matches FormField's label indent so title aligns with form labels. -->
          <header class="mb-4 px-2 pr-10">
            <h2 id={titleId} class="text-xl font-semibold text-text">{title}</h2>
          </header>
        {/if}

        <div class="text-text">
          {@render children()}
        </div>

        {#if footer}
          <footer class="mt-6">
            {@render footer()}
          </footer>
        {/if}
      </div>
    </div>
  {/if}
</dialog>

<style>
  dialog[open] {
    animation: fade-in 100ms ease-out;
  }

  dialog[open]::backdrop {
    animation: backdrop-fade-in 100ms ease-out;
  }

  dialog[open].closing {
    animation: fade-out 100ms ease-in forwards;
  }

  dialog[open].closing::backdrop {
    animation: backdrop-fade-out 100ms ease-in forwards;
  }

  @keyframes fade-in {
    from {
      opacity: 0;
      transform: scale(0.95);
    }
    to {
      opacity: 1;
      transform: scale(1);
    }
  }

  @keyframes fade-out {
    from {
      opacity: 1;
      transform: scale(1);
    }
    to {
      opacity: 0;
      transform: scale(0.95);
    }
  }

  @keyframes backdrop-fade-in {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }

  @keyframes backdrop-fade-out {
    from {
      opacity: 1;
    }
    to {
      opacity: 0;
    }
  }
</style>
