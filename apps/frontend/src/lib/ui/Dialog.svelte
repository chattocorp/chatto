<!--
@component

The standard task-dialog shell. It owns the framed tray, inset work plane,
fixed header and footer, scrollable body, focus behavior, and dismissal.

Pass footer actions as direct children of the `footer` snippet. The dialog
owns their single-line, end-aligned layout. The selected size is the baseline
width. Footer actions can make the dialog wider when the viewport has room;
labels truncate only after the dialog reaches its viewport limit.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { m } from '$lib/i18n/messages';
  import { shouldAutoFocus } from '$lib/utils/shouldAutoFocus';

  let {
    children,
    footer,
    footerDetails,
    mediaViewer = false,
    visible = $bindable(false),
    title,
    size = 'md',
    describedBy,
    onclose
  }: {
    visible?: boolean;
    title?: string;
    size?: 'sm' | 'md' | 'lg' | 'xl';
    /** ID of an element that describes the dialog (forwarded to aria-describedby). */
    describedBy?: string;
    children: Snippet;
    footer?: Snippet;
    /** Optional file or task details beside the fixed footer actions. */
    footerDetails?: Snippet;
    /** Full-screen on small viewports, with a flexible media area and fixed controls. */
    mediaViewer?: boolean;
    onclose?: () => void;
  } = $props();

  let dialogEl: HTMLDialogElement | undefined;
  let closing = $state(false);
  // True when the current press started inside the content. Prevents a drag
  // that began inside (e.g. text selection) from closing on release outside.
  // Defaults to `true` so a click that reaches the dialog without an observed
  // pointerdown is treated as "not a backdrop click" and ignored — only a
  // real pointerdown on the backdrop arms the close path. This also protects
  // against programmatic or keyboard-synthesized clicks being mistaken for a
  // backdrop dismissal.
  let pressStartedInside = true;
  let previousFocus: Element | null = null;

  // Stable per-instance id for the title (so screen readers announce it
  // when the dialog opens). $props.id() is hydration-safe.
  const dialogId = $props.id();
  const titleId = `${dialogId}-title`;

  const sizeWidths = {
    sm: '400px',
    md: '600px',
    lg: '800px',
    xl: 'min(90vw, 1440px)'
  };

  // History-backed dialogs can unmount without a native close. Restore the
  // trigger in that path too, after releasing the browser's modal focus trap.
  function restoreFocusOnUnmount(node: HTMLDialogElement) {
    return () => {
      node.close();
      if (previousFocus instanceof HTMLElement && previousFocus.isConnected) {
        previousFocus.focus({ preventScroll: true });
      }
    };
  }

  function getDefaultAction(node: ParentNode): HTMLButtonElement | null {
    return node.querySelector<HTMLButtonElement>('button[data-dialog-default]:not([disabled])');
  }

  function syncDialogVisibility(node: HTMLDialogElement) {
    dialogEl = node;
    if (visible) {
      closing = false;
      pressStartedInside = true;
      if (!node.open) {
        previousFocus = document.activeElement;
        node.showModal();
      }
      // showModal() naturally focuses the first focusable element, which
      // for our layout is the Close (X) button in the header — not what
      // users expect. Move focus to the first form field, falling back
      // to the form's submit button or explicitly marked default action (so
      // Enter confirms instead of closing). Skipped on touch devices to avoid
      // popping the on-screen keyboard. A field that already received focus
      // via the native `autofocus` attribute is left alone.
      if (shouldAutoFocus()) {
        queueMicrotask(() => {
          const fieldSelector =
            'input:not([type="hidden"]):not([disabled]),textarea:not([disabled]),select:not([disabled])';
          const active = document.activeElement;
          const alreadyOnField =
            active instanceof HTMLElement && node.contains(active) && active.matches(fieldSelector);
          if (alreadyOnField) return;
          const target =
            node.querySelector<HTMLElement>(fieldSelector) ??
            node.querySelector<HTMLElement>('button[type="submit"]:not([disabled])') ??
            getDefaultAction(node);
          target?.focus();
        });
      }
    } else if (node.open && !closing) {
      // Already closed via close() function
      node.close();
    }
  }

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

  function handleKeydown(event: KeyboardEvent) {
    if (event.key !== 'Enter' || event.defaultPrevented || event.isComposing || event.repeat) {
      return;
    }

    if (!dialogEl) return;
    const defaultAction = getDefaultAction(dialogEl);
    if (!defaultAction) return;

    const target = event.target;
    // Forms own Enter through native submission. Textareas use Enter for a
    // new line, so neither should invoke a dialog-level default action.
    if (
      target instanceof HTMLTextAreaElement ||
      (target instanceof Element &&
        target.closest('form, button, a, select, [role="button"], [contenteditable="true"]'))
    ) {
      return;
    }

    event.preventDefault();
    defaultAction.click();
  }
</script>

<dialog
  {@attach restoreFocusOnUnmount}
  {@attach syncDialogVisibility}
  onclose={handleNativeClose}
  onkeydown={handleKeydown}
  oncancel={(e) => {
    // Always run our animated close path; never let the browser close the
    // dialog instantly without the fade-out.
    e.preventDefault();
    close();
  }}
  onpointerdown={(e) => {
    pressStartedInside = e.target !== dialogEl;
  }}
  onclick={(e) => {
    // Synthetic clicks (Enter/Space on a focused button, programmatic
    // .click(), implicit form submission) have detail=0 and clientX/Y=0,
    // which would otherwise be misread as a click on the backdrop. Only
    // real pointer clicks should dismiss the dialog.
    if (e.detail === 0 || pressStartedInside) return;
    // Use coordinate check instead of e.target to handle mobile keyboard viewport shifts
    const content = dialogEl?.firstElementChild as HTMLElement | null;
    if (!content) return;
    const rect = content.getBoundingClientRect();
    if (
      e.clientX < rect.left ||
      e.clientX > rect.right ||
      e.clientY < rect.top ||
      e.clientY > rect.bottom
    ) {
      close();
    }
  }}
  class={[
    'm-auto bg-transparent backdrop:bg-black/50',
    mediaViewer
      ? 'h-dvh max-h-dvh w-dvw max-w-dvw sm:h-fit sm:w-fit sm:max-w-[calc(100vw-2rem)]'
      : 'w-fit max-w-[calc(100vw-2rem)]'
  ]}
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
    <div
      class={[
        'dialog-frame flex max-w-full flex-col overflow-hidden bg-surface shadow-xl',
        mediaViewer ? 'sm:floating-frame' : 'floating-frame',
        mediaViewer
          ? 'h-dvh max-h-dvh w-full sm:h-[85dvh] sm:max-h-[85dvh] sm:w-max sm:rounded-lg sm:border sm:border-text/10 sm:p-2'
          : 'max-h-[calc(100dvh-2rem)] w-max rounded-lg border border-text/10 p-2 sm:max-h-[78vh]'
      ]}
      style:--dialog-baseline-width={sizeWidths[size]}
    >
      <div
        class={[
          'flex min-h-0 w-max max-w-full min-w-full flex-1 flex-col overflow-hidden bg-background p-3',
          mediaViewer ? 'sm:floating-inset' : 'floating-inset',
          mediaViewer
            ? 'ps-[max(0.75rem,env(safe-area-inset-left))] pe-[max(0.75rem,env(safe-area-inset-right))] pt-[max(0.75rem,env(safe-area-inset-top))] pb-[max(0.75rem,env(safe-area-inset-bottom))] sm:rounded-md'
            : 'rounded-md'
        ]}
      >
        <!--
          Header row holds the title (if any) and the close button, so
          they share a baseline and the title is not indented relative to
          the body content.
        -->
        <header
          class={[
            'flex w-0 min-w-full shrink-0 items-center justify-between gap-3',
            title ? 'mb-4' : 'mb-2'
          ]}
        >
          {#if title}
            <h2
              id={titleId}
              class={[
                'min-w-0 text-xl font-semibold text-balance wrap-anywhere text-text-top',
                mediaViewer && 'line-clamp-2'
              ]}
            >
              <bdi>{title}</bdi>
            </h2>
          {:else}
            <span></span>
          {/if}
          <button
            type="button"
            onclick={close}
            class="-m-2 icon-action shrink-0"
            aria-label={m('ui.close')}
          >
            <span class="iconify icon-[uil--times] text-xl"></span>
          </button>
        </header>

        <div
          class={[
            'min-h-0 w-0 min-w-full text-text',
            mediaViewer ? 'flex flex-1 flex-col overflow-hidden' : 'overflow-y-auto'
          ]}
        >
          {@render children()}
        </div>

        {#if footer}
          <footer
            class={footerDetails
              ? 'mt-3 flex w-0 min-w-full shrink-0 items-center justify-between gap-4'
              : 'dialog-actions'}
          >
            {#if footerDetails}
              <div class="min-w-0 flex-1">{@render footerDetails()}</div>
            {/if}
            {@render footer()}
          </footer>
        {/if}
      </div>
    </div>
  {/if}
</dialog>

<style>
  dialog {
    border: 0;
    padding: 0;
  }

  .dialog-frame {
    min-width: min(var(--dialog-baseline-width), calc(100vw - 2rem));
  }

  dialog[open] {
    animation: fade-in var(--motion-duration-overlay-enter) var(--motion-easing-overlay-enter);
  }

  dialog[open]::backdrop {
    animation: backdrop-fade-in var(--motion-duration-overlay-enter) var(--motion-easing-overlay-enter);
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

  @media (prefers-reduced-motion: reduce) {
    dialog[open],
    dialog[open]::backdrop,
    dialog[open].closing,
    dialog[open].closing::backdrop {
      animation: none;
    }
  }
</style>
