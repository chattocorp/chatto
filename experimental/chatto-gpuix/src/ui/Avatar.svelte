<script lang="ts">
  import { mediaUrl, readImage } from '../media.ts';
  let {
    name,
    src,
    serverUrl,
    active = true,
    size = 36
  }: {
    name: string;
    src?: string | null;
    serverUrl?: string;
    active?: boolean;
    size?: number;
  } = $props();
  let data = $state('');
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
  class="avatar"
  style={`width: ${size}px; height: ${size}px; font-size: ${Math.round(size * 0.38)}px; border-radius: ${size / 2}px;`}
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
  {#if data}<img
      src={data}
      alt={name}
      objectfit="cover"
      style={`width: ${size}px; height: ${size}px;`}
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
    border-width: 1px;
    border-color: #484848;
  }
</style>
