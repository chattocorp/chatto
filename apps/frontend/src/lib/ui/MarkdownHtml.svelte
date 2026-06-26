<!--
@component

Single audited sink for HTML produced by `$lib/markdown`.

The markdown renderer disables source HTML and owns every tag/attribute it emits.
Keep all rendered markdown HTML flowing through this component so raw HTML usage
does not spread into feature components.
-->
<script lang="ts">
  let {
    html
  }: {
    html: string;
  } = $props();
</script>

<!-- eslint-disable-next-line svelte/no-at-html-tags -- rendered markdown from `$lib/markdown`; see component docs above -->
{@html html}
