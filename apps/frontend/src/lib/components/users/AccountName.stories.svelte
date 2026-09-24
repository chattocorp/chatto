<script module lang="ts">
	import { defineMeta } from '@storybook/addon-svelte-csf';
	import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
	import AccountName from './AccountName.svelte';
	import DirectMessageName from './DirectMessageName.svelte';
	import UserIdentity from './UserIdentity.svelte';
	import MessageView from '$lib/components/messages/MessageView.svelte';
	import MentionAutocomplete from '$lib/components/composer/MentionAutocomplete.svelte';
	import { PaneHeader, Panel, UserCard } from '$lib/ui';
	const { Story } = defineMeta({
		title: 'Components/Users/AccountName',
		component: AccountName,
		tags: ['autodocs']
	});
	const bot = {
		id: 'bot',
		login: 'chatto_bot',
		displayName: 'ChattoBot',
		isBot: true,
		deleted: false,
		presenceStatus: PresenceStatus.ONLINE
	};
	const human = { ...bot, id: 'alice', login: 'alice', displayName: 'Alice', isBot: false };
	const messageBody =
		"Here's a quick and classic recipe for you:\n\n**Ingredients:**\n- 200g spaghetti\n- 100g guanciale or pancetta";
</script>

<script lang="ts">
	import { createPresenceCache } from '$lib/state/presenceCache.svelte';
	import { provideUserProfiles } from '$lib/state/userProfiles.svelte';
	createPresenceCache();
	provideUserProfiles();
</script>

<Story name="Bot" args={{ name: 'ChattoBot', identity: bot }} />
<Story name="Message badge" args={{ name: 'ChattoBot', identity: bot, badgeSize: 'md' }} />
<Story name="Profile heading" asChild>
	<div class="text-lg font-semibold"><AccountName name="TestBot" identity={bot} /></div>
</Story>
<Story name="Human" args={{ name: 'Alice', identity: human }} />
<Story name="Self-DM" asChild><DirectMessageName participants={[human]} currentUserId="alice" /></Story>
<Story name="Deleted" args={{ name: 'Deleted user', identity: { isBot: true, deleted: true } }} />
<Story name="Long name" asChild
	><div class="w-40">
		<AccountName name="A very long assistant display name" identity={bot} />
	</div></Story
>
<Story name="RTL" asChild
	><div dir="rtl" class="w-48"><AccountName name="مساعد فريق العمل" identity={bot} /></div></Story
>

<Story name="Account surfaces" asChild>
	<div class="flex max-w-3xl flex-col gap-6 bg-background p-4 text-text">
		<PaneHeader title="ChattoBot (BOT)" titleContent={dmTitle} />
		{#snippet dmTitle()}<DirectMessageName participants={[bot]} />{/snippet}
		<div class="min-w-0">
			<MessageView
				eventId="bot-message"
				actor={bot}
				displayName={bot.displayName}
				body={messageBody}
			/>
		</div>
		<Panel title="Direct messages"
			><div class="flex flex-col gap-2">
				<DirectMessageName participants={[bot]} /><DirectMessageName participants={[human, bot]} />
			</div></Panel
		>
		<Panel title="Profile"><UserIdentity user={bot} /></Panel>
		<Panel title="Mention picker"
			><div class="flex h-16 items-end">
				<div class="relative w-full">
					<MentionAutocomplete
						query="chatto"
						members={[bot]}
						onSelect={() => {}}
						onClose={() => {}}
					/>
				</div>
			</div></Panel
		>
		<UserCard name={bot.displayName} identity={bot} username={bot.login} variant="row"
			>{#snippet avatar()}<span
					class="grid size-8 place-items-center rounded-full bg-surface-emphasized">C</span
				>{/snippet}</UserCard
		>
	</div>
</Story>
