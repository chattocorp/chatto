/** Share server-fetched metadata between visible link pills and hover cards. */
import { QueryObserver } from '@tanstack/svelte-query';
import { createLinkPreviewAPI } from '@chatto/client/api/linkPreviews';
import type { LinkPreviewView } from '@chatto/client/timeline/linkPreviews';
import type { ServerScope } from '$lib/state/server/scope.svelte';
import { queryClient } from './queryClient';
import { serverSessionQueryRoot } from './keys';

/** Keep the pill and card on the same cached result until the trigger is disposed. */
export function observeLinkHover(
  scope: ServerScope,
  href: string,
  update: (preview: LinkPreviewView | null | undefined, pending: boolean) => void
): () => void {
  const observer = new QueryObserver(queryClient, {
    queryKey: [...serverSessionQueryRoot(scope.serverId, scope.connection), 'link-hover', href],
    queryFn: () => scope.connection.getAPI(createLinkPreviewAPI).fetchLinkMetadata(href),
    staleTime: 5 * 60_000,
    gcTime: 5 * 60_000,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false
  });
  const stop = observer.subscribe((result) => update(result.data, result.isPending));
  const result = observer.getCurrentResult();
  update(result.data, result.isPending);
  return stop;
}
