<script module lang="ts">
	import { defineMeta } from '@storybook/addon-svelte-csf';

	const { Story } = defineMeta({
		title: 'Foundations/Visible pagination',
		tags: ['autodocs']
	});
</script>

<script lang="ts">
	import ScrollArea from '$lib/ui/ScrollArea.svelte';
	import Button from '$lib/ui/form/Button.svelte';
	import { useLoadMoreWhenVisible } from './useLoadMoreWhenVisible.svelte';

	let count = $state(0);
	let generation = $state(0);
	const loadMore = useLoadMoreWhenVisible({
		getCursor: () => count < 30 ? count : null,
		loadMore: async () => { count += 2; }
	});
</script>

<Story name="Nested scroll container" asChild>
	<div class="flex w-72 flex-col gap-3">
		<p>{count} of 30 items loaded</p>
		<Button variant="secondary" onclick={() => { count = 0; generation++; }}>Reset</Button>
		{#key generation}
			<ScrollArea fill={false} class="h-40 rounded border border-border" data-testid="pagination-scroll" aria-label="Example items">
				{#each Array(count) as _, i (i)}
					<div class="flex h-14 items-center px-3">Item {i + 1}</div>
				{/each}
				{#if count < 30}
					<div class="h-4" {@attach loadMore}></div>
				{/if}
			</ScrollArea>
		{/key}
	</div>
</Story>
