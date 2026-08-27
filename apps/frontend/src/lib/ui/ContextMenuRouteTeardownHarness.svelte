<script lang="ts">
	import ContextMenu from './ContextMenu.svelte';

	let route = $state<'room' | 'profile'>('room');
</script>

{#if route === 'room'}
	<section data-testid="room-route">
		<h1>Room</h1>
		<ContextMenu position={{ x: 24, y: 32 }} onclose={() => {}}>
			<button type="button" onclick={() => (route = 'profile')}>View profile</button>
		</ContextMenu>
	</section>
{:else}
	<section data-testid="profile-route">
		<h1>Profile</h1>
	</section>
{/if}
