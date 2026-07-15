<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- external image URLs */
  import * as m from '$lib/i18n/messages';

  export type ImageItem = {
    id?: string;
    src: string;
    originalSrc?: string;
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

  function showDialog(node: HTMLDialogElement) {
    node.showModal();
  }

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
  {@attach showDialog}
  onclose={close}
  onkeydown={handleKeydown}
  onclick={(e) => {
    if (e.target === e.currentTarget) close();
  }}
  class="image-modal fixed inset-0 m-0 flex h-dvh max-h-dvh w-dvw max-w-dvw items-center justify-center border-none bg-black/80 p-0 backdrop:bg-transparent"
>
  {#if current}
    <div class="flex flex-col items-center gap-3">
      <div class="relative flex items-center gap-2">
        {#if hasMultiple}
          <button
            type="button"
            onclick={() => navigate(-1)}
            class="flex size-10 shrink-0 cursor-pointer items-center justify-center rounded-full text-white opacity-60 transition-opacity duration-150 hover:opacity-100"
            aria-label={m['ui.image_modal.previous']()}
          >
            <span class="iconify text-2xl uil--angle-left-b"></span>
          </button>
        {/if}

        <img
          src={current.src}
          alt={current.alt ?? current.filename ?? m['ui.image_modal.fallback_alt']()}
          class="max-h-[85vh] max-w-[85vw] object-contain"
        />

        {#if hasMultiple}
          <button
            type="button"
            onclick={() => navigate(1)}
            class="flex size-10 shrink-0 cursor-pointer items-center justify-center rounded-full text-white opacity-60 transition-opacity duration-150 hover:opacity-100"
            aria-label={m['ui.image_modal.next']()}
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
          href={current.originalSrc ?? current.src}
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
