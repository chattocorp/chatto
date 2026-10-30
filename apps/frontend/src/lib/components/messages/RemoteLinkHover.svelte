<!-- @component Static server-fetched metadata. Mounted only while its hover card is open; never embeds third-party players. -->
<script lang="ts">
  import { createLinkPreviewAPI } from '@chatto/client/api/linkPreviews';
  import type { ServerScope } from '$lib/state/server/scope.svelte';
  import { createQuery } from '$lib/query/client';
  import { serverSessionQueryRoot } from '$lib/query/keys';
  import LinkHoverContent from './LinkHoverContent.svelte';

  let { href, scope }: { href: string; scope: ServerScope } = $props();
  const preview = createQuery(() => ({
    queryKey: [...serverSessionQueryRoot(scope.serverId, scope.connection), 'link-hover', href],
    queryFn: () => scope.connection.getAPI(createLinkPreviewAPI).fetchLinkPreview(href),
    staleTime: 5 * 60_000,
    gcTime: 5 * 60_000,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false
  }));
</script>

<LinkHoverContent {href} preview={preview.data} loading={preview.isPending} />
