<!--
@component

Renders a user's Markdown bio through the audited Markdown HTML boundary. Source HTML is disabled
by the shared renderer. Plain text remains visible while the lazy renderer loads or if rendering
fails.

**Props:**
- `bio` - User-authored Markdown source
- `class` - Optional classes for layout constraints on the owning profile surface
-->
<script module lang="ts">
	let markdownModule: Promise<typeof import('$lib/markdown')> | null = null;

	function loadMarkdown() {
		markdownModule ??= import('$lib/markdown');
		return markdownModule;
	}
</script>

<script lang="ts">
	import MarkdownHtml from '$lib/ui/MarkdownHtml.svelte';

	let {
		bio,
		class: className
	}: {
		bio: string;
		class?: string;
	} = $props();
</script>

<div data-testid="user-bio" class={['prose prose-compact max-w-none break-words', className]} dir="auto">
	{#await loadMarkdown()}
		<p class="whitespace-pre-line">{bio}</p>
	{:then { renderMarkdown }}
		{#await renderMarkdown(bio)}
			<p class="whitespace-pre-line">{bio}</p>
		{:then html}
			<MarkdownHtml {html} />
		{:catch}
			<p class="whitespace-pre-line">{bio}</p>
		{/await}
	{:catch}
		<p class="whitespace-pre-line">{bio}</p>
	{/await}
</div>
