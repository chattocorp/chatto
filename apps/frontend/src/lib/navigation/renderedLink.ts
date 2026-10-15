/** Shared navigation for links supplied through Markdown and sidebar content. */
import { goto } from '$app/navigation';
import { resolveChatRoute } from '$lib/chatLinks';

/** Rewrite registered Chatto routes; keep other web destinations external. */
export function renderedLinkTarget(href: string) {
  const path = resolveChatRoute(href);
  return path
    ? { href: path }
    : { href, target: '_blank' as const, rel: 'noopener noreferrer' as const };
}

/** Open a content URL with the current catalogue, also for menu and composer actions. */
export function openRenderedLink(href: string, newWindow = false): void {
  const path = resolveChatRoute(href);
  if (path !== null && !newWindow) {
    // Keep a non-null ResolvedPathname binding for navigation's type-aware lint rule.
    const localPath = path;
    void goto(localPath);
  } else {
    window.open(path ?? href, '_blank', 'noopener,noreferrer');
  }
}

/** Navigate with the current catalogue, including links rendered before registration. */
export function handleRenderedLinkClick(event: MouseEvent, href: string): void {
  if (event.defaultPrevented || event.button > 1 || /^mailto:/i.test(href)) return;
  event.preventDefault();
  openRenderedLink(href, event.metaKey || event.ctrlKey || event.shiftKey || event.button === 1);
}
