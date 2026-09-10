<script lang="ts">
	import { Code, ConnectError } from '@connectrpc/connect';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { completeServerSetup } from '$lib/api-client/setup';
	import { getPublicServerInfo } from '$lib/api-client/server';
	import { m } from '$lib/i18n/messages';
	import chattoIcon from '$lib/assets/chatto-icon.png';
	import { PageTitle, Hint, PaneHeader, PaneContent, Panel, FormSection } from '$lib/ui';
	import { TextInput, TextArea, Button, Form } from '$lib/ui/form';

	let { data } = $props();
	let serverName = $state('');
	let description = $state('');
	let login = $state('');
	let displayName = $state('');
	let password = $state('');
	let passwordConfirmation = $state('');
	let fieldErrors = $state<Record<string, string>>({});
	let busy = $state(false);
	let error = $state('');
	let closed = $state(false);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (busy) return;

		error = '';
		fieldErrors = {};
		if (password !== passwordConfirmation) {
			fieldErrors.confirmation = m('common.validation.passwords_match');
			return;
		}
		busy = true;
		try {
			await completeServerSetup(window.location.origin, {
				serverName,
				description,
				login,
				displayName,
				password
			});
			password = '';
			passwordConfirmation = '';
			// Use normal sign-in; setup never mints a second kind of session.
			await goto(resolve('/login'), { invalidateAll: true });
		} catch (cause) {
			// A failed response can follow a committed batch. Re-read discovery
			// before offering a retry, and retain ordinary sign-in as recovery.
			try {
				closed = !(await getPublicServerInfo(window.location.origin)).setupRequired;
			} catch {
				/* Keep the draft for a retry. */
			}
			if (closed) {
				password = '';
				passwordConfirmation = '';
			}
			const field = cause instanceof ConnectError ? cause.metadata.get('Chatto-Error-Field') : null;
			if (
				!closed &&
				cause instanceof ConnectError &&
				field &&
				['login', 'display_name', 'password'].includes(field)
			) {
				fieldErrors[field] = cause.rawMessage;
			} else {
				error =
					cause instanceof ConnectError && cause.code === Code.InvalidArgument
						? cause.rawMessage
						: m('auth.setup.failed');
			}
		} finally {
			busy = false;
		}
	}
</script>

<!-- @component Single-form origin-server setup inside the shared client shell. -->
<PageTitle title={m('auth.setup.title')} />
<div class="pane-page">
	<div class="shrink-0" data-page-reveal>
		<PaneHeader title={m('auth.setup.title')} />
	</div>
	<PaneContent>
		<div class="flex flex-col gap-6">
			<div class="flex flex-col items-start gap-5 py-2 sm:flex-row sm:items-center">
				<div class="shrink-0" aria-hidden="true" data-page-reveal>
					<img src={chattoIcon} alt="" width="96" height="96" class="size-24 outline-none" />
				</div>
				<div class="flex max-w-xl flex-col gap-2" data-page-reveal>
					<h2 class="text-2xl font-bold text-balance text-text-top">{m('auth.setup.welcome')}</h2>
					<p class="text-pretty text-muted">{m('auth.setup.intro')}</p>
				</div>
			</div>
			<div data-page-reveal>
				<Panel title={m('auth.setup.title')}>
					{#if closed}
						<div class="flex flex-col items-start gap-4">
							<Hint>{m('auth.setup.closed')}</Hint>
							<Button href={resolve('/login')}>{m('auth.login.title')}</Button>
						</div>
					{:else if !data.setupServer.directLoginEnabled}
						<Hint tone="warning">{m('auth.setup.login_disabled')}</Hint>
					{:else}
						<Form onsubmit={submit} {error}>
							<div class="grid gap-8 md:grid-cols-2">
								<FormSection title={m('auth.setup.server_details')}>
									<div class="flex flex-col gap-4">
										<TextInput
											id="setup-server-name"
											label={m('auth.setup.server_name')}
											description={m('auth.setup.server_name_help')}
											bind:value={serverName}
											required
											maxlength={80}
											disabled={busy}
										/>
										<TextArea
											id="setup-description"
											label={m('auth.setup.description')}
											description={m('auth.setup.description_help')}
											bind:value={description}
											maxlength={500}
											rows={4}
											disabled={busy}
										/>
										<p class="text-sm text-muted">{m('auth.setup.change_later')}</p>
									</div>
								</FormSection>
								<FormSection title={m('auth.setup.owner')}>
									<div class="flex flex-col gap-4">
										<TextInput
											id="setup-login"
											error={fieldErrors.login}
											oninput={() => {
												delete fieldErrors.login;
												delete fieldErrors.confirmation;
											}}
											label={m('common.username')}
											bind:value={login}
											autocomplete="username"
											maxlength={32}
											required
											disabled={busy}
										/>
										<TextInput
											id="setup-display-name"
											error={fieldErrors.display_name}
											oninput={() => {
												delete fieldErrors.display_name;
												delete fieldErrors.confirmation;
											}}
											label={m('auth.setup.display_name')}
											bind:value={displayName}
											maxlength={32}
											required
											disabled={busy}
										/>
										<TextInput
											id="setup-password"
											error={fieldErrors.password}
											oninput={() => {
												delete fieldErrors.password;
												delete fieldErrors.confirmation;
											}}
											label={m('common.password')}
											type="password"
											bind:value={password}
											autocomplete="new-password"
											minlength={8}
											required
											disabled={busy}
										/>
										<TextInput
											id="setup-password-confirmation"
											label={m('common.confirm_password')}
											type="password"
											bind:value={passwordConfirmation}
											autocomplete="new-password"
											error={fieldErrors.confirmation}
											oninput={() => {
												delete fieldErrors.confirmation;
											}}
											required
											disabled={busy}
										/>
									</div>
								</FormSection>
							</div>
							{#snippet footer()}
								<div class="flex w-full justify-end">
									<Button type="submit" loading={busy} disabled={busy}>
										<span class="iconify icon-[uil--rocket]" aria-hidden="true"></span>
										{m('auth.setup.finish')}
									</Button>
								</div>
							{/snippet}
						</Form>
					{/if}
				</Panel>
			</div>
		</div>
	</PaneContent>
</div>
