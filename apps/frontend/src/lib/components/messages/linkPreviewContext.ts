/** Share existing message/composer metadata with inline links without another request. */
import { getContext, setContext } from 'svelte';
import type { LinkPreviewView } from '@chatto/client/timeline/linkPreviews';

type PreviewLookup = (href: string) => LinkPreviewView | null | undefined;
type PreviewContext = { lookup: PreviewLookup; isLoading: (href: string) => boolean };
const key = Symbol('inline link preview');

/** Provide a reactive lookup scoped to one message or composer. */
export function provideLinkPreview(
  lookup: PreviewLookup,
  isLoading: (href: string) => boolean = () => false
): void {
  setContext<PreviewContext>(key, { lookup, isLoading });
}

/** Read existing metadata; absence of a provider never triggers a request. */
export function useLinkPreview(): PreviewContext {
  return (
    getContext<PreviewContext | undefined>(key) ?? {
      lookup: () => undefined,
      isLoading: () => false
    }
  );
}
