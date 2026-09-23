<!-- @component Shared badge for an active bot account. -->
<script lang="ts">
	import { BOT_ACCOUNT_LABEL } from '$lib/render/accountName';
	import Pill from '$lib/ui/Pill.svelte';

	let { size = 'sm' }: { size?: 'sm' | 'md' } = $props();
</script>

<span class={['shrink-0', size === 'sm' && 'flex']} data-testid="bot-badge">
	<Pill tone="default" paddingClass={size === 'md' ? 'px-1.5 py-[2.5px]' : 'px-1 py-px'} class="leading-tight">{BOT_ACCOUNT_LABEL}</Pill>
</span>
