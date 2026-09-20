<!-- @component Renders each DM participant with its own account identity. -->
<script lang="ts">
	import { m } from '$lib/i18n/messages';
	import { buildDirectMessagePresentation, type UserAvatarUserView } from '$lib/render/users';
	import AccountName from './AccountName.svelte';

	let {
		participants,
		currentUserId,
		getDisplayName = (_id, fallback) => fallback
	}: {
		participants: Array<
			Pick<UserAvatarUserView, 'id' | 'login' | 'displayName' | 'isBot'> & { deleted?: boolean }
		>;
		currentUserId?: string | null;
		getDisplayName?: (id: string, fallback: string) => string;
	} = $props();
	const presentation = $derived(
		buildDirectMessagePresentation(participants, currentUserId, m('common.you'), getDisplayName)
	);
</script>

<span class="inline-flex max-w-full min-w-0 items-baseline gap-1">
	{#each presentation.visibleParticipants as participant, index (participant.id)}
		<span class="inline-flex min-w-0 items-baseline">
			<AccountName
				name={participant.id === currentUserId
					? m('common.you')
					: getDisplayName(participant.id, participant.displayName || participant.login)}
				identity={participant}
			/>
			{#if index < presentation.visibleParticipants.length - 1}<span class="shrink-0">,</span>{/if}
		</span>
	{:else}
		<bdi>{m('common.you')}</bdi>
	{/each}
</span>
