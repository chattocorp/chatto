<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    children,
    footer,
    visible = $bindable(false),
    title,
    size = 'md',
    onclose
  }: {
    visible?: boolean;
    title?: string;
    size?: 'sm' | 'md' | 'lg';
    children: Snippet;
    footer?: Snippet;
    onclose?: () => void;
  } = $props();

  let dialogEl: HTMLDialogElement | undefined = $state();
  let closing = $state(false);

  const sizeClasses = {
    sm: 'w-100 max-w-[60vw]',
    md: 'w-150 max-w-[80vw]',
    lg: 'w-200 max-w-[90vw]'
  };

  $effect(() => {
    if (visible) {
      closing = false;
      dialogEl?.showModal();
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
    e.preventDefault();
    const active = document.activeElement;
    if (active && dialogEl?.contains(active) && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA')) {
      return;
    }
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
>
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
        <header class="mb-4 pr-8">
          <h2 class="text-xl font-semibold text-text">{title}</h2>
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
