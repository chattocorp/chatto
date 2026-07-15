<!--
@component

Shared actions shown when a server icon or joined room row is right-clicked or long-pressed.
The parent owns read and leave behavior so this component stays presentation-only.
-->
<script lang="ts">
	import * as m from '$lib/i18n/messages';

	let {
		kind,
		canMarkRead,
		canLeave = true,
		onMarkRead,
		onLeave
	}: {
		kind: 'server' | 'room';
		canMarkRead: boolean;
		canLeave?: boolean;
		onMarkRead: () => void;
		onLeave: () => void;
	} = $props();
</script>

<div class="menu-section">
	<nav class="sidebar-nav">
		<button
			type="button"
			class="sidebar-item disabled:cursor-not-allowed disabled:opacity-50"
			onclick={onMarkRead}
			disabled={!canMarkRead}
			role="menuitem"
		>
			<span class="sidebar-icon iconify uil--check-circle" aria-hidden="true"></span>
			{m['room_list.mark_as_read']()}
		</button>

		{#if canLeave}
			<div role="separator" class="mx-2 my-1 border-t border-text/10"></div>
			<button
				type="button"
				class="sidebar-item text-danger hover:text-danger"
				onclick={onLeave}
				role="menuitem"
			>
				<span
					class={[
						'sidebar-icon iconify',
						kind === 'server' ? 'uil--minus-circle' : 'uil--sign-out-alt'
					]}
					aria-hidden="true"
				></span>
				{kind === 'server'
					? m['room_list.remove_server']()
					: m['room_list.leave_room']()}
			</button>
		{/if}
	</nav>
</div>
