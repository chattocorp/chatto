<!--
@component

Renders a compact user identity with the shared avatar and display name. Native
right-click and stationary touch long-press open the shared user profile menu.
-->
<script module lang="ts">
	type UserContextMenuModule = typeof import('$lib/components/menus/UserContextMenu.svelte');

	let userContextMenuModule: Promise<UserContextMenuModule> | null = null;

	function loadUserContextMenu() {
		userContextMenuModule ??= import('$lib/components/menus/UserContextMenu.svelte');
		return userContextMenuModule;
	}
</script>

<script lang="ts">
	import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
	import UserAvatar from '$lib/components/UserAvatar.svelte';
	import type { UserAvatarUserView } from '$lib/render/users';
	import {
		contextMenuTrigger,
		type ContextMenuTriggerDetails
	} from '$lib/ui/contextMenuTrigger.svelte';

	type IdentityUser = Omit<UserAvatarUserView, 'deleted' | 'presenceStatus'> & {
		deleted?: boolean;
		presenceStatus?: PresenceStatus;
	};

	let {
		user,
		size = 'sm',
		class: className,
		userContextMenuLoader = loadUserContextMenu
	}: {
		user: IdentityUser;
		size?: 'xs' | 'sm' | 'md';
		class?: string;
		userContextMenuLoader?: () => Promise<UserContextMenuModule>;
	} = $props();

	const profileUser = $derived<UserAvatarUserView>({
		...user,
		deleted: user.deleted ?? false,
		presenceStatus: user.presenceStatus ?? PresenceStatus.OFFLINE
	});
	let profileMenu = $state<ContextMenuTriggerDetails | null>(null);
	const profileMenuTrigger = contextMenuTrigger((details) => {
		profileMenu = details;
	});
</script>

<span
	class={['inline-flex min-w-0 items-center gap-2', className]}
	data-testid="user-identity"
	{@attach profileMenuTrigger}
>
	<UserAvatar user={profileUser} {size} useLiveProfile={false} />
	<bdi class="min-w-0 truncate font-medium text-text-top">
		{profileUser.displayName || profileUser.login}
	</bdi>
</span>

{#if profileMenu}
	{#await userContextMenuLoader() then { default: UserContextMenu }}
		<UserContextMenu
			user={profileUser}
			position={profileMenu.position}
			presentation={profileMenu.presentation}
			onClose={() => (profileMenu = null)}
		/>
	{/await}
{/if}
