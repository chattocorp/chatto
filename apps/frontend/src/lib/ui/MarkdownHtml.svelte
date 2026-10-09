<!--
@component

Single audited sink for HTML produced by the full and restricted renderers in
`$lib/markdown`.

The markdown renderer disables source HTML and owns every tag/attribute it emits.
Keep all rendered markdown HTML flowing through this component so raw HTML usage
does not spread into feature components. Callers may pass only markdown renderer
output, plus the mention and edited-marker post-processing in the message path.
This component also owns code copy actions and footnote navigation. Footnote
targets are unique to each component, including when callers reuse rendered HTML.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { trustedMarkdownHtml } from '$lib/security/trustedHtml';
  import { toast } from '$lib/ui/toast';

  let {
    html
  }: {
    html: string;
  } = $props();

  const footnoteScope = $props.id();
  const trusted = $derived(trustedMarkdownHtml(scopeFootnotes(html)));

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
    if (!button) return;

    event.preventDefault();
    event.stopPropagation();
    const code = button.closest('pre')?.getAttribute('data-copy-source');
    if (code === null || code === undefined) return;

    try {
      await navigator.clipboard.writeText(code);
      toast.success(m('common.copied_to_clipboard'));
    } catch {
      toast.error(m('room.message.actions.copy_text_failed'));
    }
  }

  // Markdown actions come from the audited HTML renderer, so they use one
  // delegated listener on this component's DOM root.
  function attachMarkdownActions(element: HTMLDivElement) {
    element.addEventListener('click', handleClick);
    return () => element.removeEventListener('click', handleClick);
  }
</script>

<div class="markdown-html contents" {@attach attachMarkdownActions}>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- rendered markdown from `$lib/markdown`; see component docs above -->
  {@html trusted}
</div>
