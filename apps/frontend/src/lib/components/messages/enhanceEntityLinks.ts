/** Mount reactive entity displays into Markdown links for the lifetime of their DOM nodes. */
import { mount, unmount } from 'svelte';
import type { Attachment } from 'svelte/attachments';
import EntityLink from './EntityLink.svelte';

/** Preserve the original rendered anchor as the fallback when an entity is unavailable. */
export function enhanceEntityLinks(context: Map<unknown, unknown>): Attachment<HTMLAnchorElement> {
  return (anchor) => {
    if (!anchor.hasAttribute('data-entity-autolink')) return;
    const href = anchor.dataset.contentHref;
    if (!href) return;
    // Ordinary web links need no component. Keep candidates mounted so later
    // catalogue and projection changes can resolve them without re-rendering Markdown.
    let url: URL;
    try {
      url = new URL(href, window.location.origin);
    } catch {
      return;
    }
    if (url.pathname !== '/' && !url.pathname.startsWith('/chat/')) return;
    const target = document.createElement('span');
    anchor.replaceWith(target);
    const component = mount(EntityLink, {
      target,
      context,
      props: { href, fallbackHtml: anchor.outerHTML }
    });
    return () => {
      void unmount(component);
      target.replaceWith(anchor);
    };
  };
}
