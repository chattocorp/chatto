<!--
@component

Shared bot collection used by owner settings and Server Admin. Rows navigate to
the shared bot detail editor; only creation remains a collection-level dialog.
-->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { onMount } from 'svelte';
	import { serverIdToSegment } from '$lib/navigation';
	import { getActiveServer } from '$lib/state/activeServer.svelte';
	import { useConnection } from '$lib/state/server/connection.svelte';
	import { createBotAPI, type BotAccount } from '$lib/api-client/bots';
	import { createUserAPI } from '$lib/api-client/users';
	import { Panel, DataTable } from '$lib/components/admin';
	import { Button, TextArea, TextInput } from '$lib/ui/form';
	import { EmptyState, FormDialog, Hint, Pill } from '$lib/ui';
	import { toast } from '$lib/ui/toast';
	import { BotManagementStore } from './BotManagementStore.svelte';
	import BotCredentialsDialog from './BotCredentialsDialog.svelte';
	import * as m from '$lib/i18n/messages';

	let {
		scope,
		canCreate = false,
		scrollContainer
	}: {
		scope: 'owner' | 'admin';
		canCreate?: boolean;
		scrollContainer?: HTMLDivElement;
	} = $props();

	const connection = useConnection();
	const store = new BotManagementStore(
		() => (scope === 'owner' ? 'owned' : 'manageable'),
		() => {
			const conn = connection();
			return createBotAPI({ baseUrl: conn.connectBaseUrl, bearerToken: conn.bearerToken });
		},
		() => {
			const conn = connection();
			return createUserAPI({ baseUrl: conn.connectBaseUrl, bearerToken: conn.bearerToken });
		}
	);

	let createVisible = $state(false);
	let login = $state('');
	let displayName = $state('');
	let botDescription = $state('');
	let saving = $state(false);
	let saveError = $state<string | null>(null);
	let createdBot = $state<BotAccount | null>(null);
	let createdSecret = $state<string | null>(null);

	const normalizedLogin = $derived(login.trim());
	const normalizedDisplayName = $derived(displayName.trim());
	const normalizedDescription = $derived(botDescription.trim());
	const loginValid = $derived(normalizedLogin.toLowerCase().endsWith('_bot'));
	const formValid = $derived(
		loginValid && normalizedDisplayName.length > 0 && normalizedDescription.length > 0
	);

	onMount(() => void store.load());

	function openCreate() {
		login = '';
		displayName = '';
		botDescription = '';
		saveError = null;
		createVisible = true;
	}

	function closeCreate() {
		if (saving) return;
		createVisible = false;
		saveError = null;
	}

	function openBot(bot: BotAccount) {
		const serverId = serverIdToSegment(getActiveServer());
		void goto(
			scope === 'owner'
				? resolve('/chat/[serverId]/settings/bots/[botId]', { serverId, botId: bot.id })
				: resolve('/chat/[serverId]/manage/server/bots/[botId]', { serverId, botId: bot.id })
		);
	}

	async function createBot() {
		if (!formValid) return;
		saving = true;
		saveError = null;
		try {
			const created = await store.create({
				login: normalizedLogin,
				displayName: normalizedDisplayName,
				description: normalizedDescription
			});
			createVisible = false;
			createdBot = created.bot;
			createdSecret = created.apiKey;
			toast.success(m['bots.toast.created']());
		} catch (error) {
			saveError = error instanceof Error ? error.message : m['bots.error.save_failed']();
		} finally {
			saving = false;
		}
	}

	function closeCreatedSecret() {
		const bot = createdBot;
		createdBot = null;
		createdSecret = null;
		if (bot) openBot(bot);
	}

	function ownerLabel(bot: BotAccount): string {
		const owner = store.owner(bot);
		return owner ? `${owner.displayName} (@${owner.login})` : bot.ownerId;
	}
</script>

<div class="flex min-h-0 flex-1 flex-col">
	{#if store.error}
		<Hint tone="danger">{store.error}</Hint>
	{/if}

	{#if store.loading && store.bots.length === 0}
		<div class="text-muted">{m['bots.loading']()}</div>
	{:else if store.bots.length === 0}
		<Panel>
			<EmptyState icon="uil--robot" title={m['bots.empty.title']()}>
				<div class="flex flex-col items-center gap-4">
					<p>{scope === 'owner' ? m['bots.empty.owner']() : m['bots.empty.admin']()}</p>
					{#if canCreate}
						<Button variant="secondary" onclick={openCreate}>
							<span class="iconify uil--plus" aria-hidden="true"></span>
							{m['bots.action.create']()}
						</Button>
					{/if}
				</div>
			</EmptyState>
		</Panel>
	{:else}
		{#if canCreate}
			<div class="mb-4 flex justify-end">
				<Button variant="secondary" size="sm" onclick={openCreate}>
					<span class="iconify uil--plus" aria-hidden="true"></span>
					{m['bots.action.create']()}
				</Button>
			</div>
		{/if}
		<Panel noPadding>
			<DataTable
				items={store.bots}
				columns={scope === 'admin' ? 3 : 2}
				getKey={(bot) => bot.id}
				hasMore={store.hasMore && !store.error}
				loadingMore={store.loadingMore}
				onLoadMore={() => store.loadMore()}
				loadMoreRoot={scrollContainer}
				loadingMoreMessage={m['bots.loading_more']()}
				onRowClick={openBot}
			>
				{#snippet header()}
					<th class="table-header-cell">{m['bots.field.bot']()}</th>
					{#if scope === 'admin'}
						<th class="table-header-cell">{m['bots.field.owner']()}</th>
					{/if}
					<th class="table-header-cell">{m['bots.field.description']()}</th>
				{/snippet}
				{#snippet row(bot)}
					<td class="px-4 py-3">
						<div class="flex min-w-0 items-center gap-3">
							<div
								class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-surface-emphasized text-neutral-action"
								aria-hidden="true"
							>
								<span class="iconify text-xl uil--robot"></span>
							</div>
							<div class="min-w-0">
								<div class="flex items-center gap-2">
									<span class="truncate font-medium text-text-top">{bot.displayName}</span>
									<Pill tone="neutral">{m['bots.badge.bot']()}</Pill>
								</div>
								<p class="truncate text-sm text-muted">@{bot.login}</p>
							</div>
						</div>
					</td>
					{#if scope === 'admin'}
						<td class="px-4 py-3 text-muted">{ownerLabel(bot)}</td>
					{/if}
					<td class="max-w-md px-4 py-3 text-muted">
						<p class="line-clamp-2">{bot.description}</p>
					</td>
				{/snippet}
			</DataTable>
		</Panel>
	{/if}
</div>

<FormDialog
	bind:visible={createVisible}
	title={m['bots.dialog.create_title']()}
	submitLabel={m['bots.action.create']()}
	submitLoadingText={m['bots.action.creating']()}
	submitIcon="iconify uil--plus"
	loading={saving}
	disabled={!formValid}
	error={saveError}
	onsubmit={createBot}
	onclose={closeCreate}
>
	{#snippet description()}{m['bots.dialog.description']()}{/snippet}
	<TextInput
		id="bot-display-name"
		label={m['bots.field.display_name']()}
		placeholder={m['bots.placeholder.display_name']()}
		maxlength={100}
		required
		bind:value={displayName}
	/>
	<TextInput
		id="bot-username"
		label={m['bots.field.username']()}
		placeholder={m['bots.placeholder.username']()}
		description={m['bots.help.username']()}
		error={login.length > 0 && !loginValid ? m['bots.error.username_suffix']() : undefined}
		maxlength={64}
		required
		bind:value={login}
	/>
	<TextArea
		id="bot-description"
		label={m['bots.field.description']()}
		placeholder={m['bots.placeholder.description']()}
		description={m['bots.help.description']()}
		maxBytes={2000}
		rows={5}
		required
		bind:value={botDescription}
	/>
</FormDialog>

{#if createdBot}
	<BotCredentialsDialog
		bot={createdBot}
		action="show"
		initialSecret={createdSecret}
		onupdated={(bot) => store.replace(bot)}
		onclose={closeCreatedSecret}
	/>
{/if}
