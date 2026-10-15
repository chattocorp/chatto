/** Shared navigation for links supplied through Markdown and sidebar content. */
import { goto } from '$app/navigation';
import { classifyMessageBodyChatLink } from '$lib/chatLinks';

/** Rewrite known conversation links; keep all other web destinations external. */
export function renderedLinkTarget(href: string) {
  const chatLink = classifyMessageBodyChatLink(href);
  return chatLink
    ? { href: chatLink.path }
    : { href, target: '_blank' as const, rel: 'noopener noreferrer' as const };
}

/** Navigate with the current catalogue, including links rendered before registration. */
export function handleRenderedLinkClick(event: MouseEvent, href: string): void {
  if (event.defaultPrevented || /^mailto:/i.test(href)) return;
  event.preventDefault();
  const chatLink = classifyMessageBodyChatLink(href);
  if (chatLink) {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.button === 1) {
      window.open(chatLink.path, '_blank', 'noopener,noreferrer');
    } else {
      void goto(chatLink.path);
    }
    return;
  }
  // Explicit opening also keeps same-origin non-chat URLs outside a PWA.
  window.open(href, '_blank', 'noopener,noreferrer');
}
