<!--
@component

Single audited sink for HTML produced by the full and restricted renderers in
`$lib/markdown`.

The markdown renderer disables source HTML and owns every tag/attribute it emits.
Keep all rendered markdown HTML flowing through this component so raw HTML usage
does not spread into feature components. Callers may pass only markdown renderer
output, plus the mention and edited-marker post-processing in the message path.
This component also owns the copy action for rendered code blocks.
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

  const trusted = $derived(trustedMarkdownHtml(html));

  async function handleClick(event: MouseEvent) {
    const target = event.target;
    if (!(target instanceof Element)) return;
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

  // Markdown buttons come from the audited HTML renderer, so they use one
  // delegated listener on this component's DOM root.
  function attachCopyHandler(element: HTMLDivElement) {
    element.addEventListener('click', handleClick);
    return () => element.removeEventListener('click', handleClick);
  }
</script>

<div class="markdown-html contents" {@attach attachCopyHandler}>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- rendered markdown from `$lib/markdown`; see component docs above -->
  {@html trusted}
</div>
