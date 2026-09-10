<script lang="ts">
	import { Code, ConnectError } from '@connectrpc/connect';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { completeServerSetup } from '$lib/api-client/setup';
	import { getPublicServerInfo } from '$lib/api-client/server';
	import AuthLayout from '$lib/components/AuthLayout.svelte';
	import { m } from '$lib/i18n/messages';
	import PageTitle from '$lib/ui/PageTitle.svelte';
	import Hint from '$lib/ui/Hint.svelte';
	import { TextInput, Button, Form } from '$lib/ui/form';

	let { data } = $props();
	let step = $state<'server' | 'owner'>('server');
	let serverName = $state('');
	let description = $state('');
	let login = $state('');
	let displayName = $state('');
	let password = $state('');
	let busy = $state(false);
	let error = $state('');
	let closed = $state(false);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (busy) return;
		if (step === 'server') { step = 'owner'; return; }
		busy = true;
		error = '';
		try {
			await completeServerSetup(window.location.origin, { serverName, description, login, displayName, password });
			password = '';
			// Use normal sign-in; setup never mints a second kind of session.
			await goto(resolve('/login'), { invalidateAll: true });
		} catch (cause) {
			// A failed response can follow a committed batch. Re-read discovery
			// before offering a retry, and retain ordinary sign-in as recovery.
			try { closed = !(await getPublicServerInfo(window.location.origin)).setupRequired; } catch { /* Keep the draft for a retry. */ }
			if (closed) password = '';
			error = cause instanceof ConnectError && cause.code === Code.InvalidArgument
				? cause.rawMessage : m('auth.setup.failed');
		} finally {
			busy = false;
		}
	}
</script>

<!-- @component First-run server settings and owner account wizard. -->
<PageTitle title={m('auth.setup.title')} />
<AuthLayout showBranding={false}>
	<div class="mb-8 space-y-3">
		<span class="icon-[uil--chat-bubble-user] text-4xl text-action" aria-hidden="true"></span>
		<h1 class="text-2xl font-semibold">{m('auth.setup.title')}</h1>
		<p class="text-muted">{m('auth.setup.intro')}</p>
	</div>
	{#if closed}
		<div class="space-y-4">
			<Hint>{m('auth.setup.closed')}</Hint>
			<Button href={resolve('/login')}>{m('auth.login.title')}</Button>
		</div>
	{:else if !data.setupServer.directLoginEnabled}
		<Hint tone="warning">{m('auth.setup.login_disabled')}</Hint>
	{:else}
		<Form onsubmit={submit} {error}>
			{#if step === 'server'}
				<TextInput id="setup-server-name" label={m('auth.setup.server_name')} bind:value={serverName} required maxlength={80} />
				<TextInput id="setup-description" label={m('auth.setup.description')} bind:value={description} maxlength={500} />
			{:else}
				<h2 class="font-semibold">{m('auth.setup.owner')}</h2>
				<TextInput id="setup-login" label={m('common.username')} bind:value={login} autocomplete="username" maxlength={32} required disabled={busy} />
				<TextInput id="setup-display-name" label={m('auth.setup.display_name')} bind:value={displayName} maxlength={32} required disabled={busy} />
				<TextInput id="setup-password" label={m('common.password')} type="password" bind:value={password} autocomplete="new-password" minlength={8} required disabled={busy} />
			{/if}
			{#snippet footer()}
				<div class="flex justify-end gap-2">
					{#if step === 'owner'}<Button variant="secondary" disabled={busy} onclick={() => { step = 'server'; error = ''; }}>{m('common.back')}</Button>{/if}
					<Button type="submit" loading={busy}>{step === 'server' ? m('common.continue') : m('auth.setup.finish')}</Button>
				</div>
			{/snippet}
		</Form>
	{/if}
</AuthLayout>
