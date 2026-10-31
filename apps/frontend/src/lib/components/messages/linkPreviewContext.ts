/** Share existing message/composer metadata with inline links without another request. */
import { getContext, setContext } from 'svelte';
import type { LinkPreviewView } from '@chatto/client/timeline/linkPreviews';

type PreviewLookup = (href: string) => LinkPreviewView | null | undefined;
const key = Symbol('inline link preview');

/** Provide a reactive lookup scoped to one message or composer. */
export function provideLinkPreview(lookup: PreviewLookup): void {
  setContext(key, lookup);
}

/** Read existing metadata; absence of a provider never triggers a request. */
export function useLinkPreview(): PreviewLookup {
  return getContext<PreviewLookup | undefined>(key) ?? (() => undefined);
}
