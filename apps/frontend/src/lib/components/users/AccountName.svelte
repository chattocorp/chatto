<!-- @component
Displays a resolved account name with its bot identity. Callers own name
fallbacks and live profile updates. Only the name truncates in narrow layouts.
-->
<script lang="ts">
	import type { ClassValue } from 'svelte/elements';
	import { isBotAccount, type AccountNameIdentity } from '$lib/render/accountName';
	import BotBadge from './BotBadge.svelte';

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
		<BotBadge size={badgeSize} />
	{/if}
</span>
