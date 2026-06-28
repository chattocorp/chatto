<script lang="ts">
	import type { Snippet } from 'svelte';
	import { provideConnection } from '$lib/state/server/connection.svelte';
	import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';

	let { children }: { children: Snippet } = $props();

	// Provide the home instance's connection state by default.
	// The [[serverId=hostname]] layout overrides this with the
	// correct instance-specific connection for its subtree.
	provideConnection(() => serverConnectionManager.originClient);
</script>

{@render children()}
