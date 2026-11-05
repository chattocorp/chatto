<script lang="ts">
  import { untrack } from 'svelte';
  import ActionButton from './ActionButton.svelte';
  import { mediaUrl, readImage } from './media.ts';

  let {
    src,
    serverUrl,
    active,
    alt = '',
    resolveSource
  }: {
    src: string | null | undefined;
    serverUrl: string;
    active: boolean;
    alt?: string;
    /** Refresh expiring attachment URLs immediately before a visible read. */
    resolveSource?: () => Promise<string | null>;
  } = $props();
  const url = $derived(mediaUrl(src, serverUrl));
  const external = $derived(!!url && url.origin !== new URL(serverUrl).origin);
  let allowedSource = $state('');
  let attempt = $state(0);
  let data = $state('');
  let failed = $state(false);
</script>

<div
  class="image-frame"
  {@attach () => {
    const current = url;
    const permitted = !external || allowedSource === current?.href;
    // These dependencies restart the read on scrolling, URL changes, or retry.
    const retry = attempt;
    const resolve = resolveSource;
    const enabled = active;
    const origin = serverUrl;
    const controller = new AbortController();
    data = '';
    failed = false;
    if (enabled && current && permitted) {
      void (async () => {
        // The callback reads mutable SDK metadata. Only the explicit source,
        // viewport, consent and retry inputs own this attachment's lifetime.
        const fresh = resolve ? mediaUrl(await untrack(resolve), origin) : current;
        // A refreshed URL must not silently add another external host.
        if (!fresh || fresh.origin !== current.origin) throw new Error('Image location changed');
        return readImage(fresh, controller.signal);
      })()
        .then((value) => {
          if (!controller.signal.aborted) data = value;
        })
        .catch(() => {
          if (!controller.signal.aborted) failed = true;
        });
    }
    void retry;
    return () => controller.abort();
  }}
>
  {#if data}
    <img src={data} {alt} objectfit="contain" class="image" testid="media-image" />
  {:else if !url}
    <text class="status">Image unavailable</text>
  {:else if external && allowedSource !== url.href}
    <text class="status">Loading this image shares your IP address with {url.host}.</text>
    <ActionButton
      label={`Load image from ${url.host}`}
      onclick={() => (allowedSource = url!.href)}
      testid="load-external-image"
    />
  {:else if failed}
    <text class="status">Could not load image (PNG, JPEG, GIF or WebP, up to 12 MB).</text>
    <ActionButton label="Retry image" onclick={() => attempt++} testid="retry-image" />
  {:else}
    <text class="status">{active ? 'Loading image…' : 'Image preview'}</text>
  {/if}
</div>

<style>
  .image-frame {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    width: 100%;
    height: 220px;
    flex-shrink: 0;
    background-color: var(--color-background);
    border-radius: 8px;
  }
  .image {
    width: 100%;
    height: 220px;
  }
  .status {
    font-size: 12px;
    color: var(--color-muted);
    padding: 12px;
  }
</style>
