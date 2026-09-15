<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { getPublicServerInfo } from '$lib/api-client/server';
	import { m } from '$lib/i18n/messages';
	import { queryClient } from '$lib/query/client';
	import type { RegisteredServer } from '$lib/state/server/registry.svelte';
	import { Button } from '$lib/ui/form';

	let { server, connecting, disabled, onSignIn }: {
		server: RegisteredServer;
		connecting: boolean;
		disabled: boolean;
		onSignIn: () => void;
	} = $props();

	// Public discovery is separate from saved identity and authentication. Recheck
	// on each mount; cached success must not enable sign-in during a fresh check.
	const discovery = createQuery(
		() => ({
			queryKey: ['login-server-discovery', server.url],
			queryFn: ({ signal }) => getPublicServerInfo(server.url, {
				signal: AbortSignal.any([signal, AbortSignal.timeout(10_000)])
			}),
			staleTime: 0,
			gcTime: 0,
			retry: false,
			networkMode: 'always' as const
		}),
		() => queryClient
	);
</script>

<!-- @component Checks a remembered server before offering sign-in. Failed checks keep the saved server and allow a manual retry. -->
<div class="surface-box flex items-center gap-3 p-3">
	<div class="min-w-0 flex-1">
		<div class="truncate font-semibold"><bdi>{server.name}</bdi></div>
		<div class="truncate text-muted" dir="ltr">{new URL(server.url).host}</div>
		<div class="text-muted" role="status">
			{#if discovery.isFetching || discovery.isPending}
				{m('auth.login.checking_server')}
			{:else if discovery.isError}
				{m('auth.login.server_unavailable')}
			{:else if !discovery.data?.authorizeUrl}
				{m('add_server.directory.sign_in_unavailable')}
			{/if}
		</div>
	</div>
	{#if discovery.isError && !discovery.isFetching}
		<Button variant="secondary" disabled={disabled} onclick={() => discovery.refetch()}>
			{m('common.retry')}
		</Button>
	{:else}
		<Button
			variant="secondary"
			onclick={onSignIn}
			loading={connecting}
			disabled={disabled || discovery.isFetching || !discovery.isSuccess || !discovery.data?.authorizeUrl}
		>
			{m('add_server.sign_in')}
		</Button>
	{/if}
</div>
