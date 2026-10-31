<script lang="ts">
  import type { LinkPreviewView } from '@chatto/client/timeline/linkPreviews';
  import { provideLinkPreview } from './linkPreviewContext';
  import LinkHover from './LinkHover.svelte';
  import UrlLink from './UrlLink.svelte';

  let {
    href,
    preview,
    loading = false
  }: { href: string; preview?: LinkPreviewView; loading?: boolean } = $props();
  provideLinkPreview(
    (url) => (preview?.url === url ? preview : undefined),
    (url) => url === href && loading
  );
</script>

<LinkHover {href}>
  {#snippet children(title, loading)}
    <UrlLink {href} {title} {loading} />
  {/snippet}
</LinkHover>
