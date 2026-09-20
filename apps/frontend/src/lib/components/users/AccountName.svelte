<!-- @component
Displays a resolved account name with its bot identity. Callers own name
fallbacks and live profile updates. Only the name truncates in narrow layouts.
-->
<script lang="ts">
	import type { ClassValue } from 'svelte/elements';
	import { BOT_ACCOUNT_LABEL, isBotAccount, type AccountNameIdentity } from '$lib/render/accountName';
	// Keep this identity leaf independent of app-shell modules in the UI barrel.
	import Pill from '$lib/ui/Pill.svelte';

	let {
		name,
		identity,
		badgeSize = 'sm',
		class: className
	}: {
		name: string;
		identity?: AccountNameIdentity | null;
		/** Message-area labels use md; other surfaces use the smaller, centred badge. */
		badgeSize?: 'sm' | 'md';
		class?: ClassValue;
	} = $props();
</script>

<span class={['inline-flex max-w-full min-w-0 gap-1.5 align-baseline', badgeSize === 'md' ? 'items-baseline' : 'items-center', className]}>
	<bdi class="min-w-0 truncate">{name}</bdi>{#if isBotAccount(identity)}
		<span class={['shrink-0', badgeSize === 'sm' && 'flex']} data-testid="bot-badge"
			><Pill tone="default" paddingClass={badgeSize === 'md' ? 'px-1.5 py-[2.5px]' : 'px-1 py-px'} class="leading-tight">{BOT_ACCOUNT_LABEL}</Pill></span
		>
	{/if}
</span>
