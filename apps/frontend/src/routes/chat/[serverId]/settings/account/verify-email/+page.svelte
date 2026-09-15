<script lang="ts">
	import { beforeNavigate, goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { Code, ConnectError } from '@connectrpc/connect';
	import { onDestroy } from 'svelte';
	import { createAccountAPI } from '$lib/api-client/account';
	import { m } from '$lib/i18n/messages';
	import { serverIdToSegment } from '$lib/navigation';
	import { queryClient } from '$lib/query/client';
	import { adminQueryKeys } from '$lib/query/admin';
	import { settingsQueryKeys } from '$lib/query/settings';
	import { useServerScope } from '$lib/state/server/scope.svelte';
	import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
	import Panel from '$lib/ui/Panel.svelte';
	import { PaneContent, PaneHeader } from '$lib/ui';
	import { Button, FormError, VerificationCodeInput } from '$lib/ui/form';
	import { toast } from '$lib/ui/toast/toastState.svelte';
	import {
		clearPendingEmailVerification,
		readPendingEmailVerification
	} from '$lib/verifiedEmailChallenge';

	const serverScope = useServerScope();
	let componentActive = true;
	let navigationGeneration = 0;

	beforeNavigate(() => {
		navigationGeneration += 1;
	});
	onDestroy(() => {
		componentActive = false;
		navigationGeneration += 1;
	});

	type EmailActionScope = {
		serverId: string;
		connection: ServerConnection;
		userId: string;
		navigationGeneration: number;
	};

	const accountPath = $derived(
		resolve('/chat/[serverId]/settings/account', {
			serverId: serverIdToSegment(serverScope.serverId)
		})
	);
	const viewerUserId = $derived(serverScope.store.currentUser.user?.id ?? '');
	let pendingEmail = $derived(
		viewerUserId ? readPendingEmailVerification(serverScope.serverId, viewerUserId) : ''
	);

	let code = $state('');
	let error = $state('');
	let confirming = $state(false);
	let resending = $state(false);

	function emailActionScope(): EmailActionScope {
		return {
			serverId: serverScope.serverId,
			connection: serverScope.connection,
			userId: viewerUserId,
			navigationGeneration
		};
	}

	function isCurrentEmailScope(scope: EmailActionScope): boolean {
		return (
			serverScope.isCurrent() &&
			scope.serverId === serverScope.serverId &&
			scope.connection.queryScope === serverScope.connection.queryScope &&
			scope.userId !== '' &&
			scope.userId === viewerUserId
		);
	}

	function isCurrentEmailContext(scope: EmailActionScope): boolean {
		return componentActive && isCurrentEmailScope(scope);
	}

	function isCurrentEmailAction(scope: EmailActionScope): boolean {
		return isCurrentEmailContext(scope) && scope.navigationGeneration === navigationGeneration;
	}

	async function confirm(event: SubmitEvent) {
		event.preventDefault();
		const address = pendingEmail;
		const submittedCode = code;
		const scope = emailActionScope();
		if (!address || !scope.userId || submittedCode.length !== 6) {
			error = m('auth.register.code.missing');
			return;
		}
		confirming = true;
		error = '';
		try {
			const emails = await scope.connection
				.getAPI(createAccountAPI)
				.confirmEmailVerification(scope.userId, address, submittedCode);
			clearPendingEmailVerification(scope.serverId, scope.userId, address);
			if (!isCurrentEmailScope(scope)) return;
			queryClient.setQueryData(
				settingsQueryKeys.verifiedEmails(scope.serverId, scope.connection, scope.userId),
				emails
			);
			void queryClient.invalidateQueries({
				queryKey: adminQueryKeys.membersRoot(scope.serverId, scope.connection)
			});
			void queryClient.invalidateQueries({
				queryKey: adminQueryKeys.member(scope.serverId, scope.connection, scope.userId),
				exact: true
			});
			if (isCurrentEmailContext(scope)) pendingEmail = '';
			if (!isCurrentEmailAction(scope)) return;
			toast.success(m('settings.account.email.verified'));
			await goto(accountPath, { replaceState: true });
		} catch (reason) {
			if (!isCurrentEmailContext(scope)) return;
			error =
				reason instanceof Error ? reason.message : m('settings.account.email.confirm_failed');
		} finally {
			if (isCurrentEmailContext(scope)) confirming = false;
		}
	}

	async function resend() {
		const address = pendingEmail;
		const scope = emailActionScope();
		if (!address || !scope.userId) return;
		resending = true;
		error = '';
		try {
			await scope.connection
				.getAPI(createAccountAPI)
				.requestEmailVerification(scope.userId, address);
			if (!isCurrentEmailAction(scope)) return;
			code = '';
			toast.success(m('settings.account.email.code_sent'));
		} catch (reason) {
			if (!isCurrentEmailContext(scope)) return;
			if (reason instanceof ConnectError && reason.code === Code.AlreadyExists) {
				clearPendingEmailVerification(scope.serverId, scope.userId, address);
				pendingEmail = '';
			}
			error =
				reason instanceof ConnectError && reason.code === Code.AlreadyExists
					? m('settings.account.email.already_verified')
					: reason instanceof Error
						? reason.message
						: m('settings.account.email.request_failed');
		} finally {
			if (isCurrentEmailContext(scope)) resending = false;
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
							clearPendingEmailVerification(
								serverScope.serverId,
								viewerUserId,
								pendingEmail
							)}
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
