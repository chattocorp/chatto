/** Mount reactive entity displays into Markdown links for the lifetime of their DOM nodes. */
import { createRawSnippet, mount, unmount } from 'svelte';
import type { Attachment } from 'svelte/attachments';
import EntityLink from './EntityLink.svelte';
import { parseWebLink } from '$lib/navigation/webLink';

/** Add web-link previews; only automatic URLs replace the author's label with a pill. */
export function enhanceEntityLinks(context: Map<unknown, unknown>): Attachment<HTMLAnchorElement> {
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
