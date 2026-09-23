<!-- @component Renders account badges inside a localized plain-text sentence. -->
<script lang="ts">
	import type { AccountNameIdentity } from '$lib/render/accountName';
	import AccountName from './AccountName.svelte';

	let {
		text,
		accounts
	}: {
		/** Translation with account tokens made by accountNameToken. */
		text: string;
		accounts: readonly { name: string; identity?: AccountNameIdentity | null }[];
	} = $props();

	const parts = $derived(text.split(/(\uFFF0\d+\uFFF1)/g));
</script>

{#each parts as part, index (index)}
	{@const match = /^\uFFF0(\d+)\uFFF1$/.exec(part)}
	{#if match && accounts[Number(match[1])]}
		{@const account = accounts[Number(match[1])]}
		<AccountName name={account.name} identity={account.identity} />
	{:else}{part}{/if}
{/each}
