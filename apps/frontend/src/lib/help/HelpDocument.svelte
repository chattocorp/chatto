<!-- @component Article typography scoped to Markdown blocks, without restyling embedded examples. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { attachHelpCodeFades } from './codeFades';
  import type { Locale } from '$lib/i18n/locales';
  let { children, locale }: { children: Snippet; locale: Locale } = $props();
  /** Each mounted article owns its code-block observers and overlays. */
  function attachArticleCodeFades(element: HTMLElement) {
    const cleanups = [...element.querySelectorAll<HTMLElement>(':scope > pre')].map(
      attachHelpCodeFades
    );
    return () => cleanups.forEach((cleanup) => cleanup());
  }
  /** Restore deep links after the lazy article has mounted, including reloads. */
  function restoreAnchor(element: HTMLDivElement) {
    let id: string;
    try {
      id = decodeURIComponent(window.location.hash.slice(1));
    } catch {
      return;
    }
    if (!id.startsWith('section-')) return;
    const target = [
      ...element.querySelectorAll<HTMLElement>(':scope > h2[id], :scope > h3[id]')
    ].find((heading) => heading.id === id);
    target?.scrollIntoView({ block: 'start' });
  }
</script>

<div
  {@attach restoreAnchor}
  {@attach attachArticleCodeFades}
  lang={locale}
  dir={locale === 'ar' || locale === 'he-IL' ? 'rtl' : 'ltr'}
  class="leading-relaxed [&_code]:[direction:ltr] [&_code]:[unicode-bidi:isolate] [&>:first-child]:mt-0 [&>:last-child]:mb-0 [&>blockquote]:my-5 [&>blockquote]:border-s-3 [&>blockquote]:border-action [&>blockquote]:ps-4 [&>h2]:mt-12 [&>h2]:mb-4 [&>h2]:scroll-mt-6 [&>h2]:border-b [&>h2]:border-border [&>h2]:pb-3 [&>h2]:text-2xl [&>h2]:font-bold [&>h2]:tracking-tight [&>h2]:text-balance [&>h2]:text-text-top [&>h3]:mt-8 [&>h3]:mb-3 [&>h3]:scroll-mt-6 [&>h3]:text-lg [&>h3]:font-semibold [&>h3]:tracking-tight [&>h3]:text-text-top [&>ol]:my-5 [&>ol]:list-decimal [&>ol]:ps-6 [&>ol_a]:text-link [&>ol_a]:underline [&>ol:last-child>li:last-child]:mb-0 [&>ol>li]:my-3 [&>p]:my-5 [&>p_a]:text-link [&>p_a]:underline [&>pre]:relative [&>pre]:rounded-lg [&>pre]:bg-surface [&>pre]:p-4 [&>pre]:[--message-code-background:var(--color-surface)] [&>pre]:[direction:ltr] [&>pre>code]:block [&>pre>code]:overflow-x-auto [&>ul]:my-5 [&>ul]:list-disc [&>ul]:ps-6 [&>ul_a]:text-link [&>ul_a]:underline [&>ul:last-child>li:last-child]:mb-0 [&>ul>li]:my-3"
>
  {@render children()}
</div>
