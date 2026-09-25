<!-- @component Displays a compact account identity marker beside a name. -->
<script lang="ts">
	import Pill from '$lib/ui/Pill.svelte';

	let {
		label,
		testId,
		size = 'sm',
		uppercase = false,
		class: className
	}: {
		label: string;
		testId: string;
		size?: 'sm' | 'md';
		uppercase?: boolean;
		class?: string;
	} = $props();
</script>

<span class={['shrink-0', size === 'sm' && 'flex', className]} data-testid={testId}>
	<Pill
		tone="default"
		paddingClass={size === 'md' ? 'px-1.5 py-[2.5px]' : 'px-1 py-px'}
		class={uppercase ? 'leading-tight uppercase' : 'leading-tight'}>{label}</Pill
	>
</span>
