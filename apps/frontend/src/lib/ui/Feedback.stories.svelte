<script module lang="ts">
	import { defineMeta } from '@storybook/addon-svelte-csf';
	import { expect } from 'storybook/test';
	import MenuItem from './MenuItem.svelte';
	import MenuSection from './MenuSection.svelte';
	import UserCard from './UserCard.svelte';
	import ActivityListRow from './ActivityListRow.svelte';
	import CompactActionButton from './CompactActionButton.svelte';

	const { Story } = defineMeta({ title: 'Design System/Feedback' });
</script>

<script lang="ts">
	import { provideMenuContext } from './menuContext.svelte';

	provideMenuContext({ presentation: () => 'floating', containerRole: () => 'menu' });
	let expanded = $state(false);
</script>

{#snippet avatar()}
	<span class="grid size-8 place-items-center rounded-full bg-surface-emphasized" aria-hidden="true">A</span>
{/snippet}

<Story
	name="Shared hover timing"
	asChild
	play={async ({ canvasElement }) => {
		// Check compiled CSS: Tailwind property utilities must not replace token timing.
		const controls = canvasElement.querySelectorAll(
			'.menu-entry, .hover-reveal-action, .feedback-quick, .icon-action'
		);
		const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		for (const control of controls) {
			await expect(getComputedStyle(control).transitionDuration).toBe(reducedMotion ? '0s' : '0.05s');
			await expect(getComputedStyle(control).transitionTimingFunction).toBe('ease-out');
		}
		if (reducedMotion) return;
		try {
			canvasElement.style.setProperty('--motion-duration-feedback', '80ms');
			for (const control of controls) {
				await expect(getComputedStyle(control).transitionDuration).toBe('0.08s');
			}
			for (const control of canvasElement.querySelectorAll('.pill-button')) {
				await expect(getComputedStyle(control).transitionDuration).toBe('0.08s, 0.08s, 0.15s');
			}
		} finally {
			canvasElement.style.removeProperty('--motion-duration-feedback');
		}
	}}
>
	<div class="flex w-80 flex-col gap-4">
		<div class="menu" role="menu" aria-label="Message actions">
			<MenuSection>
				<MenuItem icon="icon-[uil--corner-up-left]">Reply</MenuItem>
				<MenuItem icon="icon-[uil--copy]">Copy link</MenuItem>
				<MenuItem disabled>Unavailable action</MenuItem>
			</MenuSection>
		</div>
		<UserCard
			name="Alice"
			username="alice"
			variant="row"
			{avatar}
			menu={{
				label: 'Member options',
				revealOnHover: true,
				expanded,
				onclick: () => (expanded = !expanded)
			}}
		/>
		<ActivityListRow onclick={() => {}}>
			New reply in General
			{#snippet actions()}
				<CompactActionButton label="Reply options">
					<span class="iconify icon-[uil--ellipsis-v]" aria-hidden="true"></span>
				</CompactActionButton>
			{/snippet}
		</ActivityListRow>
		<button class="icon-action self-start" aria-label="More actions">
			<span class="iconify icon-[uil--ellipsis-v]" aria-hidden="true"></span>
		</button>
	</div>
</Story>
