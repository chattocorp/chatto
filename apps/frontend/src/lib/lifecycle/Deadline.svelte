<!--
@component

Runs `onreached` once when an absolute deadline is reached while this headless
component is mounted. Dates beyond the browser's maximum timeout delay are
scheduled in safe chunks. Changing `at` or `offsetMilliseconds` reschedules the
deadline; unmounting clears it.

Use this when work is tied to an application timestamp rather than a relative
animation, debounce, request timeout, or store-owned lifecycle.
-->
<script lang="ts">
	const maximumTimeoutDelayMs = 2_147_483_647;

	let {
		at,
		offsetMilliseconds = 0,
		onreached
	}: {
		at: string | number | Date;
		offsetMilliseconds?: number;
		onreached: () => void;
	} = $props();

	$effect(() => {
		const timestamp =
			(typeof at === 'number' ? at : at instanceof Date ? at.getTime() : new Date(at).getTime()) +
			offsetMilliseconds;
		if (!Number.isFinite(timestamp)) return;

		let timeout: number | undefined;
		const schedule = () => {
			const remaining = timestamp - Date.now();
			timeout = window.setTimeout(() => {
				if (timestamp <= Date.now()) onreached();
				else schedule();
			}, Math.min(Math.max(remaining, 0), maximumTimeoutDelayMs));
		};
		schedule();

		return () => {
			if (timeout !== undefined) window.clearTimeout(timeout);
		};
	});
</script>
