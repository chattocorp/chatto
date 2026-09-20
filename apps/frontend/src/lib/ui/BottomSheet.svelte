<!-- @component
Touch action surface. Shares modal lifecycle and handle gestures with Dialog.
Long content scrolls within the available visual viewport.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import ModalSurface from './ModalSurface.svelte';
	let {
		children: body,
		visible = $bindable(false),
		ariaLabel,
		onclose
	}: {
		visible?: boolean;
		ariaLabel?: string;
		children: Snippet;
		onclose?: () => void;
	} = $props();
</script>

<ModalSurface bind:visible sheet {ariaLabel} {onclose}>
	{#snippet children(_close, handle)}
		<div class="sheet-frame flex flex-col overflow-hidden">
			{@render handle()}
			<div class="min-h-0 overflow-y-auto overscroll-contain">{@render body()}</div>
		</div>
	{/snippet}
</ModalSurface>
