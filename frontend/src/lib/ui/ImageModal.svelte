<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- external image URLs */

  export type ImageItem = {
    id?: string;
    src: string;
    alt?: string;
    filename?: string;
  };

  let {
    items,
    index = $bindable(0),
    onclose
  }: {
    items: ImageItem[];
    index?: number;
    onclose: () => void;
  } = $props();

  let current = $derived(items[index]);
  let hasMultiple = $derived(items.length > 1);

  let dialogEl: HTMLDialogElement | undefined = $state();

  $effect(() => {
    dialogEl?.showModal();
  });

  function close() {
    onclose();
  }

  function navigate(direction: -1 | 1) {
    index = (index + direction + items.length) % items.length;
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      close();
    } else if (e.key === 'ArrowLeft' && hasMultiple) {
      e.preventDefault();
      navigate(-1);
    } else if (e.key === 'ArrowRight' && hasMultiple) {
      e.preventDefault();
      navigate(1);
    }
  }
</script>

<dialog
  bind:this={dialogEl}
  onclose={close}
  onkeydown={handleKeydown}
  onclick={(e) => {
    if (e.target === dialogEl) close();
  }}
  class="fixed inset-0 m-0 flex h-dvh max-h-dvh w-dvw max-w-dvw items-center justify-center border-none bg-black/80 p-0 backdrop:bg-transparent"
>
  {#if current}
    <div class="flex flex-col items-center gap-3">
      <div class="relative flex items-center gap-2">
        {#if hasMultiple}
          <button
            type="button"
            onclick={() => navigate(-1)}
            class="nav-button"
            aria-label="Previous image"
          >
            <span class="iconify text-2xl uil--angle-left-b"></span>
          </button>
        {/if}

        <img
          src={current.src}
          alt={current.alt ?? current.filename ?? 'Image'}
          class="max-h-[85vh] max-w-[85vw] object-contain"
        />

        {#if hasMultiple}
          <button
            type="button"
            onclick={() => navigate(1)}
            class="nav-button"
            aria-label="Next image"
          >
            <span class="iconify text-2xl uil--angle-right-b"></span>
          </button>
        {/if}
      </div>

      <div class="flex items-center gap-4 text-white/80">
        {#if current.filename}
          <span class="text-sm">{current.filename}</span>
        {/if}

        {#if hasMultiple}
          <span class="text-sm text-white/50">{index + 1} / {items.length}</span>
        {/if}

        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external image URL -->
        <a
          href={current.src}
          target="_blank"
          rel="noopener noreferrer"
          class="flex items-center gap-1 text-sm text-white/60 hover:text-white"
        >
          <span class="iconify uil--external-link-alt"></span>
          Open original
        </a>
      </div>
    </div>
  {/if}
</dialog>

<style>
  dialog[open] {
    animation: fade-in 150ms ease-out;
  }

  @keyframes fade-in {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }

  .nav-button {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2.5rem;
    height: 2.5rem;
    flex-shrink: 0;
    border-radius: 9999px;
    color: white;
    opacity: 0.6;
    cursor: pointer;
    transition: opacity 150ms;

    &:hover {
      opacity: 1;
    }
  }
</style>
