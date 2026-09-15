<!-- @component
A document viewer with an explicit privacy gate. The owner supplies a preview
URL only after consent. Download and Close remain outside the sandbox.
-->
<script lang="ts">
  import { scale } from 'svelte/transition';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { expoOutTransition } from './motion';
  import Dialog from './Dialog.svelte';

  let {
    filename,
    contentType = 'text/html',
    size = null,
    sizeLoading = false,
    downloadUrl,
    previewUrl = null,
    busy = false,
    error = null,
    onpreview,
    ondownload,
    onclose
  }: {
    filename: string;
    contentType?: string;
    /** Original document size in bytes; null means metadata is unavailable. */
    size?: number | null;
    sizeLoading?: boolean;
    downloadUrl: string | null;
    previewUrl?: string | null;
    busy?: boolean;
    error?: string | null;
    onpreview: () => void;
    ondownload: (event: MouseEvent) => void;
    onclose: () => void;
  } = $props();

  const id = $props.id();
  const documentType = $derived(
    contentType.split(';', 1)[0].trim().toLowerCase() === 'application/xhtml+xml'
      ? m('room.attachment.html_viewer.xhtml_document')
      : m('room.attachment.html_viewer.html_document')
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

<Dialog visible title={filename} size="xl" {onclose}>
  <div class="flex h-[60dvh] min-h-0 flex-col">
    {#if error}
      <p role="alert" class="shrink-0 px-4 py-2 text-sm text-danger">{error}</p>
    {/if}
    {#if previewUrl}
      <iframe
        in:scale={{ start: 0.98, ...expoOutTransition() }}
        src={previewUrl}
        title={m('room.attachment.html_viewer.preview_title', { filename })}
        sandbox=""
        referrerpolicy="no-referrer"
        class="min-h-0 w-full flex-1 overflow-hidden rounded-md border-0 bg-white"
      ></iframe>
    {:else}
      <div
        class="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 overflow-y-auto p-6 text-center"
      >
        <button
          type="button"
          class="btn-action"
          disabled={busy}
          aria-describedby={`${id}-warning`}
          onclick={onpreview}
        >
          {busy ? m('common.loading') : m('room.attachment.html_viewer.show_preview')}
        </button>
        <p id={`${id}-warning`} class="max-w-md text-sm text-muted">
          {m('room.attachment.html_viewer.warning')}
        </p>
      </div>
    {/if}
  </div>
  {#snippet footerDetails()}
    <dl class="flex min-w-0 flex-wrap gap-x-6 gap-y-2 leading-5">
      <div class="min-w-0">
        <dt class="text-muted">{m('room.attachment.html_viewer.document_type')}</dt>
        <dd title={contentType}>{documentType}</dd>
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
  {/snippet}
  {#snippet footer()}
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
  {/snippet}
</Dialog>
