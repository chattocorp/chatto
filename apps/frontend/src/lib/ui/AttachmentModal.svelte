<!-- @component
Shared attachment viewer shell. Preview loading belongs to the owner.
Header, gallery controls and download stay outside the preview on all screens.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { toInlineEndDelta } from '$lib/i18n/direction';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';

  import Dialog from './Dialog.svelte';

  let {
    filename,
    contentType,
    description,
    children,
    index = 0,
    count = 1,
    onnavigate,
    size = null,
    sizeLoading = false,
    downloadUrl,

    busy = false,
    error = null,

    ondownload,
    onretry,
    onclose
  }: {
    filename: string;
    contentType: string;
    description?: string;
    children: Snippet;
    index?: number;
    count?: number;
    onnavigate?: (direction: -1 | 1) => void;
    /** Original document size in bytes; null means metadata is unavailable. */
    size?: number | null;
    sizeLoading?: boolean;
    downloadUrl: string | null;

    busy?: boolean;
    error?: string | null;

    ondownload: (event: MouseEvent) => void;
    onretry?: () => void;
    onclose: () => void;
  } = $props();

  const descriptionId = $props.id();

  function handleKeydown(event: KeyboardEvent) {
    if (count < 2 || event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey)
      return;
    if (
      event.target instanceof Element &&
      event.target.closest(
        'input, textarea, select, video, audio, media-player, [contenteditable="true"]'
      )
    )
      return;
    if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
      event.preventDefault();
      onnavigate?.(toInlineEndDelta(event.key === 'ArrowLeft' ? -1 : 1) as -1 | 1);
    }
  }
  const mediaType = $derived(contentType.split(';', 1)[0].trim().toLowerCase());
  const typeLabel = $derived(
    mediaType === 'text/html'
      ? m('room.attachment.html_viewer.html_document')
      : mediaType === 'application/xhtml+xml'
        ? m('room.attachment.html_viewer.xhtml_document')
        : mediaType
  );
  const formattedSize = $derived.by(() => {
    if (size === null || !Number.isFinite(size) || size < 0) return null;
    const unit = Math.min(4, Math.floor(Math.log2(Math.max(1, size)) / 10));
    const value = new Intl.NumberFormat(getLocale(), { maximumFractionDigits: 1 }).format(
      size / 1024 ** unit
    );
    return `${value} ${['B', 'KiB', 'MiB', 'GiB', 'TiB'][unit]}`;
  });
</script>

<svelte:window onkeydown={handleKeydown} />

<Dialog
  visible
  title={filename}
  size="xl"
  mediaViewer
  describedBy={description ? descriptionId : undefined}
  {onclose}
>
  <div class="flex min-h-0 flex-1 flex-col gap-3 lg:flex-row lg:gap-6">
    <div class="flex min-h-0 min-w-0 flex-1 flex-col">
      {#if error}
        <div class="mb-3 flex shrink-0 items-center justify-between gap-3">
          <p role="alert" class="text-sm text-danger">{error}</p>
          {#if onretry}
            <button type="button" class="btn-secondary shrink-0" disabled={busy} onclick={onretry}>
              {m('common.retry')}
            </button>
          {/if}
        </div>
      {/if}
      <div class="flex min-h-0 flex-1 items-center justify-center overflow-hidden rounded-md">
        {@render children()}
      </div>
    </div>
    <div class="flex min-h-0 shrink-0 flex-col lg:w-64">
      {#if description}
        <p
          id={descriptionId}
          class="max-h-[20dvh] shrink-0 overflow-y-auto text-center wrap-anywhere whitespace-pre-wrap lg:max-h-none lg:min-h-0 lg:shrink lg:text-start"
          dir="auto"
        >
          {description}
        </p>
      {/if}
      <div
        class="mt-3 flex shrink-0 items-center justify-between gap-4 lg:mt-auto lg:flex-col lg:items-stretch lg:gap-6 lg:pt-6"
      >
        <dl class="flex min-w-0 flex-wrap gap-x-6 gap-y-2 leading-5 lg:flex-col lg:gap-4">
          <div class="min-w-0">
            <dt class="text-muted">{m('room.attachment.html_viewer.document_type')}</dt>
            <dd class="wrap-anywhere" title={contentType}>{typeLabel}</dd>
          </div>
          <div class="min-w-0">
            <dt class="text-muted">{m('room.attachment.html_viewer.file_size')}</dt>
            <dd>
              {sizeLoading
                ? m('common.loading')
                : (formattedSize ?? m('room.attachment.html_viewer.unavailable'))}
            </dd>
          </div>
        </dl>

        <div class="flex shrink-0 flex-col gap-3">
          {#if count > 1}
            <nav
              class="flex shrink-0 items-center justify-center gap-3"
              aria-label={m('ui.image_modal.fallback_alt')}
            >
              <button
                type="button"
                class="icon-action"
                aria-label={m('ui.image_modal.previous')}
                onclick={() => onnavigate?.(-1)}
              >
                <span
                  class="iconify icon-[uil--angle-left-b] text-xl rtl:-scale-x-100"
                  aria-hidden="true"
                ></span>
              </button>
              <span class="tabular-nums" aria-live="polite">{index + 1} / {count}</span>
              <button
                type="button"
                class="icon-action"
                aria-label={m('ui.image_modal.next')}
                onclick={() => onnavigate?.(1)}
              >
                <span
                  class="iconify icon-[uil--angle-right-b] text-xl rtl:-scale-x-100"
                  aria-hidden="true"
                ></span>
              </button>
            </nav>
          {/if}
          <a
            href={downloadUrl ?? '#'}
            download={filename}
            target="_blank"
            rel="external noopener noreferrer"
            onclick={ondownload}
            aria-disabled={busy}
            class="btn-secondary shrink-0"
          >
            <span class="iconify icon-[uil--download-alt]" aria-hidden="true"></span>
            {m('room.attachment.html_viewer.download')}
          </a>
        </div>
      </div>
    </div>
  </div>
</Dialog>
