<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { serverIdToSegment } from '$lib/navigation';
	import { getActiveServer } from '$lib/state/activeServer.svelte';
	import { serverRegistry } from '$lib/state/server/registry.svelte';
	import { viewerResponseToState } from '$lib/api-client/viewer';
	import { BOT_ACCOUNTS_CAPABILITY } from '$lib/state/server/compatibility';
	import { BotDetail } from '$lib/components/bots';
	import { AccessDenied } from '$lib/ui';
	import * as m from '$lib/i18n/messages';

	const store = serverRegistry.getStore(getActiveServer());
	const viewer = $derived(
		store.projection.viewer ? viewerResponseToState(store.projection.viewer) : null
	);
	const accessReady = $derived(
		!store.serverInfo.loading && store.permissions.loaded && viewer !== null
	);
	const supported = $derived(
		store.serverInfo.supportsProtocolCapability(BOT_ACCOUNTS_CAPABILITY) === true
	);
	const canManageOwnedBots = $derived(viewer?.viewerPermissions['bot.create'] ?? false);
	const botId = $derived(page.params.botId!);
	const backHref = $derived(
		resolve('/chat/[serverId]/settings/bots', {
			serverId: serverIdToSegment(getActiveServer())
		})
	);
</script>

{#if !accessReady}
	<!-- Keep the settings shell stable while discovery and permissions hydrate. -->
{:else if supported && canManageOwnedBots}
	<BotDetail {botId} scope="owner" />
{:else}
	<AccessDenied message={m['bots.unavailable.owner']()} {backHref} />
{/if}
