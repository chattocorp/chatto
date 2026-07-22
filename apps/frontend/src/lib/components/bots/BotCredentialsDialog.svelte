<!-- SPDX-License-Identifier: Apache-2.0 -->
<!--
@component

Linear bot API-key flow. Exactly one dialog is visible at a time: confirmation
for rotation or revocation, followed by the show-once secret after rotation.
Creation can open the show-once step directly with `initialSecret`.
-->
<script lang="ts">
	import { untrack } from 'svelte';
	import { createBotAPI, type BotAccount } from '$lib/api-client/bots';
	import { useConnection } from '$lib/state/server/connection.svelte';
	import { ConfirmDialog, Dialog, Hint } from '$lib/ui';
	import { Button } from '$lib/ui/form';
	import { toast } from '$lib/ui/toast';
	import * as m from '$lib/i18n/messages';

	let {
		bot,
		action,
		initialSecret = null,
		onupdated,
		onclose
	}: {
		bot: BotAccount;
		action: 'rotate' | 'revoke' | 'show';
		initialSecret?: string | null;
		onupdated: (bot: BotAccount) => void;
		onclose: () => void;
	} = $props();

	const connection = useConnection();
	let stage = $state<'confirm' | 'show'>(
		untrack(() => (action === 'show' ? 'show' : 'confirm'))
	);
	let secret = $state<string | null>(untrack(() => initialSecret));
	let loading = $state(false);
	let error = $state<string | null>(null);

	function api() {
		const conn = connection();
		return createBotAPI({ baseUrl: conn.connectBaseUrl, bearerToken: conn.bearerToken });
	}

	async function confirm() {
		loading = true;
		error = null;
		try {
			if (action === 'rotate') {
				const result = await api().rotateAPIKey(bot.id);
				onupdated(result.bot);
				secret = result.apiKey;
				stage = 'show';
				toast.success(m['bots.credentials.toast.rotated']());
			} else {
				const updated = await api().revokeAPIKey(bot.id);
				onupdated(updated);
				toast.success(m['bots.credentials.toast.revoked']());
				onclose();
			}
		} catch (cause) {
			error = cause instanceof Error ? cause.message : String(cause);
		} finally {
			loading = false;
		}
	}

	async function copySecret() {
		if (!secret) return;
		try {
			await navigator.clipboard.writeText(secret);
			toast.success(m['common.copied_to_clipboard']());
		} catch {
			toast.error(m['bots.credentials.copy_failed']());
		}
	}
</script>

{#if stage === 'confirm'}
	<ConfirmDialog
		visible
		title={action === 'rotate'
			? m['bots.credentials.rotate_confirm_title']()
			: m['bots.credentials.revoke_confirm_title']()}
		tone={action === 'rotate' ? 'warning' : 'danger'}
		actionLabel={action === 'rotate'
			? m['bots.credentials.rotate']()
			: m['bots.credentials.revoke']()}
		{loading}
		onconfirm={confirm}
		{onclose}
	>
		<div class="flex flex-col gap-3">
			<p>
				{action === 'rotate'
					? m['bots.credentials.rotate_confirm_body']()
					: m['bots.credentials.revoke_confirm_body']()}
			</p>
			{#if error}<Hint tone="danger">{error}</Hint>{/if}
		</div>
	</ConfirmDialog>
{:else}
	<Dialog visible title={m['bots.credentials.title']({ bot: bot.displayName })} size="md" {onclose}>
		<div class="flex flex-col gap-4">
			<Hint tone="warning">{m['bots.credentials.show_once']()}</Hint>
			<div class="flex items-start gap-2 rounded-lg bg-surface-emphasized p-3">
				<code
					class="min-w-0 flex-1 break-all text-sm select-all"
					data-testid="bot-api-key-secret">{secret}</code
				>
				<Button variant="action" size="sm" onclick={copySecret}>
					<span class="iconify uil--copy" aria-hidden="true"></span>
					{m['bots.credentials.copy']()}
				</Button>
			</div>
		</div>
	</Dialog>
{/if}
