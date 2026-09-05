<!--
@component

Shows a welcome banner to newly verified users. The banner auto-dismisses
after 5 seconds and can be manually dismissed. When shown, removes the
`welcome` query parameter from the URL.

Renders when the `welcome=true` query parameter or page state flag is set.
Consumes both flags and preserves other URL and page state values.
-->
<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { base, resolve } from '$app/paths';
	import Deadline from '$lib/lifecycle/Deadline.svelte';
	import { page } from '$app/state';
	import { m } from '$lib/i18n/messages';
	import { Hint } from '$lib/ui';

	let showWelcome = $state(
		page.url.searchParams.get('welcome') === 'true' || page.state.welcome === true
	);

	const dismissAt = Date.now() + 5000;

	onMount(() => {
		if (!showWelcome) return;

		let mounted = true;
		// Let SvelteKit finish its initial render before replacing router state.
		void tick().then(() => {
			if (!mounted) return;
			const url = new URL(page.url);
			url.searchParams.delete('welcome');
			const state = { ...page.state };
			delete state.welcome;
			// This is the current route; the assertion permits its dynamic pathname.
			replaceState(
				resolve((url.pathname.slice(base.length) + url.search + url.hash) as '/'),
				state
			);
		});
		return () => {
			mounted = false;
		};
	});
</script>

{#if showWelcome}
	<Deadline at={dismissAt} onreached={() => (showWelcome = false)} />
	<div class="mb-2">
		<Hint tone="success">
			<div class="flex items-start justify-between gap-3">
				<span>{m('welcome.verified')}</span>
				<button
					type="button"
					class="-m-1 icon-action"
					onclick={() => (showWelcome = false)}
					title={m('common.dismiss')}
				>
					<span class="iconify icon-[uil--times]"></span>
				</button>
			</div>
		</Hint>
	</div>
{/if}
