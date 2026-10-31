<script lang="ts">
  import type { LinkPreviewView } from '@chatto/client/timeline/linkPreviews';
  import { provideLinkPreview } from './linkPreviewContext';
  import LinkHover from './LinkHover.svelte';
  import UrlLink from './UrlLink.svelte';

  let { href, preview }: { href: string; preview?: LinkPreviewView } = $props();
  provideLinkPreview((url) => (preview?.url === url ? preview : undefined));
</script>

<LinkHover {href}>
  {#snippet children(title)}
    <UrlLink {href} {title} />
  {/snippet}
</LinkHover>
