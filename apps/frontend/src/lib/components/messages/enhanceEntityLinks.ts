/** Mount reactive entity displays into Markdown links for the lifetime of their DOM nodes. */
import { createRawSnippet, mount, unmount } from 'svelte';
import type { Attachment } from 'svelte/attachments';
import EntityLink from './EntityLink.svelte';
import { parseWebLink } from '$lib/navigation/webLink';

/**
 * Add hover previews; only recognised Chatto links can replace automatic URL text with pills.
 * `currentServerId` names the server that the content belongs to when no server scope
 * provides it, such as in the app header. Its links then show no server name.
 */
export function enhanceEntityLinks(
  context: Map<unknown, unknown>,
  currentServerId?: () => string | null | undefined
): Attachment<HTMLAnchorElement> {
  return (anchor) => {
    const href = anchor.dataset.contentHref;
    if (!href || !parseWebLink(href)) return;
    const target = document.createElement('span');
    anchor.replaceWith(target);
    const component = mount(EntityLink, {
      target,
      context,
      props: {
        href,
        currentServerId: currentServerId?.() ?? undefined,
        // Move the audited Markdown anchor intact, including formatted labels,
        // images, title, and navigation attributes. Never serialize it as HTML.
        children: anchor.hasAttribute('data-entity-autolink')
          ? undefined
          : createRawSnippet(() => ({
              render: () => '<span></span>',
              setup: (element) => {
                element.append(anchor);
              }
            }))
      }
    });
    return () => {
      void unmount(component);
      target.replaceWith(anchor);
    };
  };
}
