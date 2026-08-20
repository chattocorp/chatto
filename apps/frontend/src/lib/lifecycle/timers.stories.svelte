<script module lang="ts">
	import { defineMeta } from '@storybook/addon-svelte-csf';

	const componentDescription = `
		Headless timers whose setup and cleanup follow Svelte's component lifecycle. Use Interval for
		periodic component-owned work and Deadline for work tied to an absolute timestamp. Keep
		debounces, request timeouts, animation scheduling, and store or protocol timers with their
		respective owners.
	`.trim();

	const { Story } = defineMeta({
		title: 'Foundations/Lifecycle timers',
		tags: ['autodocs'],
		parameters: {
			docs: {
				description: { component: componentDescription }
			}
		}
	});
</script>

<script lang="ts">
	import Panel from '$lib/components/admin/Panel.svelte';
	import Pill from '$lib/ui/Pill.svelte';
	import Button from '$lib/ui/form/Button.svelte';
	import Deadline from './Deadline.svelte';
	import Interval from './Interval.svelte';

	const deadlineDelayMilliseconds = 5_000;

	let intervalTicks = $state(0);
	let intervalRunning = $state(true);
	let deadlineAt = $state(Date.now() + deadlineDelayMilliseconds);
	let deadlineNow = $state(Date.now());
	let deadlineReached = $state(false);
	let remainingSeconds = $derived(
		Math.max(0, (deadlineAt - deadlineNow) / 1_000).toFixed(1)
	);

	function resetInterval() {
		intervalTicks = 0;
		intervalRunning = true;
	}

	function restartDeadline() {
		deadlineNow = Date.now();
		deadlineAt = deadlineNow + deadlineDelayMilliseconds;
		deadlineReached = false;
	}

	function reachDeadline() {
		deadlineNow = Date.now();
		deadlineReached = true;
	}
</script>

<Story
	name="Interval"
	asChild
	parameters={{
		docs: {
			description: {
				story:
					'Conditionally render Interval when its periodic work should run. Removing it from the component tree clears the browser interval automatically.'
			}
		}
	}}
>
	{#if intervalRunning}
		<Interval milliseconds={1_000} ontick={() => (intervalTicks += 1)} />
	{/if}

	<div class="w-full max-w-xl">
		<Panel title="Periodic refresh" subtitle="Ticks once per second while the component is mounted.">
			<div class="flex flex-col gap-5">
				<div class="surface-box flex items-center justify-between gap-4 p-4">
					<div>
						<p class="font-medium text-text-top">Ticks received</p>
						<p class="text-muted">{intervalTicks}</p>
					</div>
					<Pill tone={intervalRunning ? 'success' : 'muted'}>
						{intervalRunning ? 'Running' : 'Unmounted'}
					</Pill>
				</div>

				<div class="flex flex-wrap gap-2">
					<Button
						variant="secondary"
						onclick={() => (intervalRunning = !intervalRunning)}
					>
						{intervalRunning ? 'Unmount interval' : 'Mount interval'}
					</Button>
					<Button variant="ghost" onclick={resetInterval}>Reset</Button>
				</div>
			</div>
		</Panel>
	</div>
</Story>

<Story
	name="Deadline"
	asChild
	parameters={{
		docs: {
			description: {
				story:
					'Deadline accepts an absolute Date, timestamp, or date string and runs once when that instant is reached. Changing the target reschedules it.'
			}
		}
	}}
>
	<Interval milliseconds={100} ontick={() => (deadlineNow = Date.now())} />
	<Deadline at={deadlineAt} onreached={reachDeadline} />

	<div class="w-full max-w-xl">
		<Panel title="Semantic expiry" subtitle="A five-second absolute deadline that can be rescheduled.">
			<div class="flex flex-col gap-5">
				<div class="surface-box flex items-center justify-between gap-4 p-4">
					<div>
						<p class="font-medium text-text-top">Time remaining</p>
						<p class="text-muted">{remainingSeconds} seconds</p>
					</div>
					<Pill tone={deadlineReached ? 'success' : 'muted'}>
						{deadlineReached ? 'Reached' : 'Scheduled'}
					</Pill>
				</div>

				<div>
					<Button variant="secondary" onclick={restartDeadline}>Restart deadline</Button>
				</div>
			</div>
		</Panel>
	</div>
</Story>
