<script lang="ts">
  import { scaled } from './scale.svelte.ts';
  import { mediaUrl, readImage } from '../media.ts';
  let {
    name,
    src,
    serverUrl,
    active = true,
    size = 36,
    square = false
  }: {
    name: string;
    src?: string | null;
    serverUrl?: string;
    active?: boolean;
    size?: number;
    square?: boolean;
  } = $props();
  let data = $state('');
  // Reserve the circular frame border; server logos have no inner border.
  const imageSize = $derived(square ? size : Math.max(0, size - 2));
  const imageRadius = $derived(square ? size / 4 : imageSize / 2);
  const initials = $derived(
    name
      .trim()
      .split(/\s+/)
      .slice(0, 2)
      .map((word) => [...word][0] || '')
      .join('')
      .toUpperCase() || '?'
  );
</script>

<div
  class={['avatar', { square }]}
  style={`width: ${scaled(size)}px; height: ${scaled(size)}px; font-size: ${scaled(Math.round(size * 0.38))}px; border-radius: ${scaled(square ? size / 4 : size / 2)}px;`}
  {@attach () => {
    const url = serverUrl ? mediaUrl(src, serverUrl) : null;
    const enabled = active && url && url.origin === new URL(serverUrl!).origin;
    const controller = new AbortController();
    data = '';
    // External avatars keep their initials: scrolling must not contact new hosts.
    if (enabled)
      void readImage(url!, controller.signal)
        .then((value) => {
          if (!controller.signal.aborted) data = value;
        })
        .catch(() => {});
    return () => controller.abort();
  }}
>
  <!-- GPUI clips the image in its shader; a rounded parent alone does not mask it. -->
  {#if data}<img
      src={data}
      alt={name}
      objectfit={square ? 'contain' : 'cover'}
      style={`width: ${scaled(imageSize)}px; height: ${scaled(imageSize)}px; border-radius: ${scaled(imageRadius)}px;`}
    />
  {:else}<text>{initials}</text>{/if}
</div>

<style>
  .avatar {
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    overflow: hidden;
    background-color: #3a3a3a;
    color: var(--color-text-top);
    font-weight: 600;
    border-width: var(--size-1);
    border-color: #484848;
  }
  .square {
    border-width: 0;
    background-color: var(--color-surface);
  }
</style>
