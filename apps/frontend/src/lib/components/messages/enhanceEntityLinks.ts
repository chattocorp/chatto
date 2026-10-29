/** Mount reactive entity displays into Markdown links for the lifetime of their DOM nodes. */
import { mount, unmount } from 'svelte';
import type { Attachment } from 'svelte/attachments';
import EntityLink from './EntityLink.svelte';
import { parseWebLink } from '$lib/navigation/webLink';

/** Enhance automatic web URLs; explicit labels and non-web links retain their original anchors. */
export function enhanceEntityLinks(context: Map<unknown, unknown>): Attachment<HTMLAnchorElement> {
  return (anchor) => {
    if (!anchor.hasAttribute('data-entity-autolink')) return;
    const href = anchor.dataset.contentHref;
    if (!href || !parseWebLink(href)) return;
    const target = document.createElement('span');
    anchor.replaceWith(target);
    const component = mount(EntityLink, {
      target,
      context,
      props: { href }
    });
    return () => {
      void unmount(component);
      target.replaceWith(anchor);
    };
  };
}
