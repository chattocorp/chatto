<!--
@component

Single audited sink for HTML produced by the full and restricted renderers in
`$lib/markdown`.

The markdown renderer disables source HTML and owns every tag/attribute it emits.
Keep all rendered markdown HTML flowing through this component so raw HTML usage
does not spread into feature components. Callers may pass only markdown renderer
output, plus the mention and edited-marker post-processing in the message path.
This component also owns link navigation, code copy actions, and footnote navigation. Footnote
targets are unique to each component, including when callers reuse rendered HTML.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { trustedMarkdownHtml, parseTrustedMarkdownHtml } from '$lib/security/trustedHtml';
  import { renderArtifactFallback } from '$lib/markdown';
  import { toast } from '$lib/ui/toast';
  import { handleRenderedLinkClick } from '$lib/navigation/renderedLink';
  import { attachCodeFrames } from '$lib/markdownArtifacts';
  import { attachCodeScrollFades } from '$lib/markdownCodeScroll';
  import type { Attachment } from 'svelte/attachments';
  import { CopyFeedback } from '$lib/dom/actionFeedback';
  const feedback = new CopyFeedback();

  let {
    html,
    enhanceLink
  }: {
    html: string;
    /** Optional message-owned enhancement; its lifetime follows this rendered HTML. */
    enhanceLink?: Attachment<HTMLAnchorElement>;
  } = $props();

  const footnoteScope = $props.id();
  const trusted = $derived(trustedMarkdownHtml(scopeFootnotes(html)));

  /** Keep failure markup in the same audited sink as the initial Markdown. */
  function showArtifactSource(frame: HTMLElement, source: string, language: string) {
    const fallback = renderArtifactFallback(source, language);
    frame.replaceChildren(...parseTrustedMarkdownHtml(fallback).body.childNodes);
    frame.dataset.presentation = 'code';
    delete frame.dataset.artifactSource;
    return attachCodeScrollFades(html)(frame);
  }

  /** Qualifies only the numeric footnote attributes emitted by markdown-it. */
  function scopeFootnotes(value: string): string {
    return value.replace(
      /\b(id|href)="(#?)(fn(?:ref)?\d+(?::\d+)?)"/g,
      (_match, attribute: string, fragment: string, id: string) =>
        `${attribute}="${fragment}${footnoteScope}-${id}"`
    );
  }

  async function handleClick(event: MouseEvent) {
    const target = event.target;
    if (!(target instanceof Element)) return;

    const footnote = target.closest<HTMLAnchorElement>('.footnote-ref > a, a.footnote-backref');
    if (footnote) {
      // Handle this before message links or SvelteKit can navigate the route.
      event.preventDefault();
      event.stopPropagation();
      const href = footnote.getAttribute('href');
      const root = event.currentTarget;
      if (!href?.startsWith('#') || !(root instanceof HTMLDivElement)) return;
      const destination = root.querySelector<HTMLElement>(`#${CSS.escape(href.slice(1))}`);
      destination?.focus({ preventScroll: true });
      destination?.scrollIntoView({ block: 'nearest' });
      return;
    }

    const button = target.closest<HTMLButtonElement>('button[data-markdown-copy]');
    if (!button) {
      const anchor = target.closest('a');
      if (anchor?.href) handleRenderedLinkClick(event, anchor.dataset.contentHref ?? anchor.href);
      return;
    }

    event.preventDefault();
    event.stopPropagation();
    const code = button
      .closest('.markdown-code-frame')
      ?.querySelector('[data-copy-source]')
      ?.getAttribute('data-copy-source');
    if (code === null || code === undefined) return;

    try {
      await navigator.clipboard.writeText(code);
      if (button.isConnected) feedback.show(button, m('common.copied_to_clipboard'));
      toast.success(m('common.copied_to_clipboard'));
    } catch {
      toast.error(m('room.message.actions.copy_text_failed'));
    }
  }

  // Markdown actions come from the audited HTML renderer, so they use one
  // delegated listener on this component's DOM root.
  function attachMarkdownActions(element: HTMLDivElement) {
    // Rerun after the HTML changes, and dispose mounted link content before replacement.
    const linkCleanups =
      enhanceLink && html.includes('data-content-href=')
        ? Array.from(
            element.querySelectorAll<HTMLAnchorElement>('a[data-content-href]'),
            enhanceLink
          )
        : [];
    element.addEventListener('click', handleClick);
    // Block the message row's long-press/selection gestures, while letting
    // pointer events reach document-level outside-press dismissal handlers.
    const gestures = ['mousedown', 'mouseup', 'touchstart', 'touchend'] as const;
    const consumeControlGesture = (event: Event) => {
      if (event.target instanceof Element && event.target.closest('.markdown-code-header button')) {
        event.stopPropagation();
      }
    };
    for (const type of gestures) element.addEventListener(type, consumeControlGesture);
    return () => {
      for (const cleanup of linkCleanups) cleanup?.();
      feedback.dispose();
      element.removeEventListener('click', handleClick);
      for (const type of gestures) element.removeEventListener(type, consumeControlGesture);
    };
  }
</script>

<div
  class="markdown-html contents"
  {@attach attachMarkdownActions}
  {@attach attachCodeFrames(html, footnoteScope, showArtifactSource)}
  {@attach attachCodeScrollFades(html)}
>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- rendered markdown from `$lib/markdown`; see component docs above -->
  {@html trusted}
</div>
