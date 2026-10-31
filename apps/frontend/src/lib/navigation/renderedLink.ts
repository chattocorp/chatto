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

/** Route plain Chatto clicks locally; leave external links and browser gestures native. */
export function handleRenderedLinkClick(event: MouseEvent, href: string): void {
  if (
    event.defaultPrevented ||
    event.button !== 0 ||
    event.metaKey ||
    event.ctrlKey ||
    event.shiftKey ||
    event.altKey
  )
    return;
  const path = resolveChatRoute(href);
  if (path === null) return;
  // Keep a non-null ResolvedPathname binding for navigation's type-aware lint rule.
  const localPath = path;
  event.preventDefault();
  void goto(localPath);
}
