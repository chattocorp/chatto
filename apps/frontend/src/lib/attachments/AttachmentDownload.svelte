<!-- @component
Download the original attachment. In the iOS Home Screen app, prepare the file
on request so a fresh user gesture can open the native share sheet.
-->
<script lang="ts">
  import { onDestroy } from 'svelte';
  import { isBackendCapableOrigin } from '@chatto/client/util/runtimeOrigin';
  import { Button } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';
  import { isIosBrowserContext, isStandaloneDisplayMode } from '$lib/utils/browserDisplayMode';

  let {
    filename,
    contentType,
    downloadUrl,
    resolveDownloadUrl,
    ondownload,
    busy
  }: {
    filename: string;
    contentType: string;
    downloadUrl: string | null;
    resolveDownloadUrl?: () => Promise<string | null>;
    ondownload: (event: MouseEvent) => void;
    busy: boolean;
  } = $props();

  const homeScreen =
    isIosBrowserContext() && isStandaloneDisplayMode() && isBackendCapableOrigin(window.location);
  // Check the type and filename before fetching a potentially large unsupported file.
  const canPrepare = $derived(
    homeScreen &&
      resolveDownloadUrl &&
      typeof navigator.share === 'function' &&
      navigator.canShare?.({ files: [new File([], filename, { type: contentType })] })
  );
  let file = $state.raw<File | null>(null);
  let pending = $state(false);
  let failed = $state(false);
  let unsupported = $state(false);
  const lifetime = new AbortController();
  onDestroy(() => lifetime.abort());

  async function save() {
    if (busy || pending || !resolveDownloadUrl) return;
    pending = true;
    failed = false;
    try {
      if (file) {
        await navigator.share({ files: [file] });
        file = null;
        return;
      }
      const url = await resolveDownloadUrl();
      if (!url || lifetime.signal.aborted) return;
      const response = await fetch(url, {
        credentials: 'omit',
        referrerPolicy: 'no-referrer',
        signal: lifetime.signal
      });
      if (!response.ok) throw new Error('Attachment download failed');
      const prepared = new File([await response.blob()], filename, { type: contentType });
      if (lifetime.signal.aborted) return;
      if (navigator.canShare({ files: [prepared] })) file = prepared;
      else unsupported = true;
    } catch (error) {
      if (
        !lifetime.signal.aborted &&
        !(error instanceof DOMException && error.name === 'AbortError')
      )
        failed = true;
    } finally {
      pending = false;
    }
  }
</script>

{#if canPrepare && !unsupported}
  {#if failed}<span role="alert">{m('common.error.generic')}</span>{/if}
  <Button variant="secondary" disabled={busy || pending} onclick={save}>
    <span class="iconify icon-[uil--download-alt]" aria-hidden="true"></span>
    {pending && !file
      ? m('common.loading')
      : file
        ? m('room.attachment.save_or_share')
        : m('room.attachment.html_viewer.download')}
  </Button>
{:else}
  <Button
    href={downloadUrl ?? '#'}
    download={filename}
    opensInNewTab
    onclick={ondownload}
    disabled={busy}
    variant="secondary"
  >
    <span class="iconify icon-[uil--download-alt]" aria-hidden="true"></span>
    {m('room.attachment.html_viewer.download')}
  </Button>
{/if}
