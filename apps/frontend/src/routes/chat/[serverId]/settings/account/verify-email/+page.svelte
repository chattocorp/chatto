<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { Code, ConnectError } from '@connectrpc/connect';
	import { createAccountAPI } from '$lib/api-client/account';
	import { m } from '$lib/i18n/messages';
	import { serverIdToSegment } from '$lib/navigation';
	import { queryClient } from '$lib/query/client';
	import { adminQueryKeys } from '$lib/query/admin';
	import { settingsQueryKeys } from '$lib/query/settings';
	import { useServerScope } from '$lib/state/server/scope.svelte';
	import Panel from '$lib/ui/Panel.svelte';
	import { PaneContent, PaneHeader } from '$lib/ui';
	import { Button, FormError, VerificationCodeInput } from '$lib/ui/form';
	import { toast } from '$lib/ui/toast/toastState.svelte';
	import {
		clearPendingEmailVerification,
		readPendingEmailVerification
	} from '$lib/verifiedEmailChallenge';

	const serverScope = useServerScope();
	const accountPath = $derived(
		resolve('/chat/[serverId]/settings/account', {
			serverId: serverIdToSegment(serverScope.serverId)
		})
	);
	const userId = $derived(serverScope.store.currentUser.user?.id ?? '');
	let pendingEmail = $derived(
		userId ? readPendingEmailVerification(serverScope.serverId, userId) : ''
	);

	let code = $state('');
	let error = $state('');
	let confirming = $state(false);
	let resending = $state(false);

	async function confirm(event: SubmitEvent) {
		event.preventDefault();
		if (!pendingEmail || code.length !== 6) {
			error = m('auth.register.code.missing');
			return;
		}
		const connection = serverScope.connection;
		confirming = true;
		error = '';
		try {
			const emails = await connection
				.getAPI(createAccountAPI)
				.confirmEmailVerification(pendingEmail, code);
			if (!serverScope.isCurrent()) return;
			queryClient.setQueryData(
				settingsQueryKeys.verifiedEmails(serverScope.serverId, connection),
				emails
			);
			void queryClient.invalidateQueries({
				queryKey: adminQueryKeys.membersRoot(serverScope.serverId, connection)
			});
			void queryClient.invalidateQueries({
				queryKey: adminQueryKeys.member(serverScope.serverId, connection, userId),
				exact: true
			});
			clearPendingEmailVerification(serverScope.serverId, userId, pendingEmail);
			toast.success(m('settings.account.email.verified'));
			await goto(accountPath, { replaceState: true });
		} catch (reason) {
			if (!serverScope.isCurrent()) return;
			error =
				reason instanceof Error ? reason.message : m('settings.account.email.confirm_failed');
		} finally {
			if (serverScope.isCurrent()) confirming = false;
		}
	}

	async function resend() {
		if (!pendingEmail) return;
		const connection = serverScope.connection;
		resending = true;
		error = '';
		try {
			await connection.getAPI(createAccountAPI).requestEmailVerification(pendingEmail);
			if (!serverScope.isCurrent()) return;
			code = '';
			toast.success(m('settings.account.email.code_sent'));
		} catch (reason) {
			if (!serverScope.isCurrent()) return;
			if (reason instanceof ConnectError && reason.code === Code.AlreadyExists) {
				clearPendingEmailVerification(serverScope.serverId, userId, pendingEmail);
				pendingEmail = '';
			}
			error =
				reason instanceof ConnectError && reason.code === Code.AlreadyExists
					? m('settings.account.email.already_verified')
					: reason instanceof Error
						? reason.message
						: m('settings.account.email.request_failed');
		} finally {
			if (serverScope.isCurrent()) resending = false;
		}
	}
</script>

<PaneHeader
	title={m('settings.account.email.code_label')}
	subtitle={m('settings.account.email.title')}
	backHref={accountPath}
/>

<PaneContent>
	<Panel title={m('settings.account.email.verify')} icon="iconify icon-[uil--envelope-check]">
		{#if pendingEmail}
			<form class="mx-auto flex max-w-md flex-col gap-5" onsubmit={confirm}>
				<div class="text-center">
					<p class="text-muted">{m('auth.register.code.sent_to')}</p>
					<p class="mt-1 font-semibold break-words"><bdi>{pendingEmail}</bdi></p>
				</div>

				<VerificationCodeInput
					bind:value={code}
					autofocus
					disabled={confirming}
					label={m('auth.register.code.aria_label')}
					digitLabel={(number) => m('auth.register.code.digit_label', { number })}
				/>

				<div class="text-center text-sm text-muted">
					{m('auth.register.code.did_not_receive')}
					<button
						type="button"
						class="cursor-pointer link disabled:cursor-default disabled:opacity-60"
						disabled={confirming || resending}
						onclick={resend}
					>
						{resending ? m('auth.register.code.resending') : m('auth.register.code.resend')}
					</button>
				</div>

				<FormError {error} />

				<div class="flex justify-end gap-2">
					<Button
						href={accountPath}
						variant="secondary"
						onclick={() =>
							clearPendingEmailVerification(serverScope.serverId, userId, pendingEmail)}
					>
						{m('settings.account.email.use_another')}
					</Button>
					<Button
						type="submit"
						disabled={code.length !== 6}
						loading={confirming}
						loadingText={m('auth.register.code.checking')}
					>
						{m('settings.account.email.verify')}
					</Button>
				</div>
			</form>
		{:else}
			<div class="flex max-w-xl flex-col items-start gap-4">
				<p class="text-muted">{m('settings.account.email.no_pending_verification')}</p>
				<Button href={accountPath} variant="secondary">
					{m('settings.account.email.use_another')}
				</Button>
			</div>
		{/if}
	</Panel>
</PaneContent>
