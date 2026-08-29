<!--
@component

Renders the restricted public Neighbor testimonial Markdown subset. Source
HTML and interactive Markdown features are not supported.
-->
<script lang="ts">
  import { renderTestimonialMarkdown } from '$lib/markdown';
  import MarkdownHtml from '$lib/ui/MarkdownHtml.svelte';

  let { testimonial }: { testimonial: string } = $props();

  const html = $derived(renderTestimonialMarkdown(testimonial));
</script>

<div
  class="min-w-0 leading-relaxed text-text [&_code]:rounded-sm [&_code]:bg-surface-emphasized [&_code]:px-1 [&_em]:italic [&_p+_p]:mt-3 [&_strong]:font-semibold"
  dir="auto"
>
  <MarkdownHtml {html} />
</div>
