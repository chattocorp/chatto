<!--
@component

Single audited sink for HTML produced by `$lib/markdown`.

The markdown renderer disables source HTML and owns every tag/attribute it emits.
Keep all rendered markdown HTML flowing through this component so raw HTML usage
does not spread into feature components. Callers may pass only markdown renderer
output, plus the mention and edited-marker post-processing in the message path.
-->
<script lang="ts">
  import { trustedMarkdownHtml } from '$lib/security/trustedHtml';

  let {
    html
  }: {
    html: string;
  } = $props();

  const trusted = $derived(trustedMarkdownHtml(html));
</script>

<!-- eslint-disable-next-line svelte/no-at-html-tags -- rendered markdown from `$lib/markdown`; see component docs above -->
{@html trusted}
