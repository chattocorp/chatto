<!--
@component

Runs `ontick` at a fixed interval while this headless component is mounted.
Changing `milliseconds` replaces the browser interval; unmounting clears it.

Use this when a periodic task's lifetime naturally follows Svelte markup. Store
and protocol lifecycles should continue to own their timers directly.
-->
<script lang="ts">
	let {
		milliseconds,
		ontick
	}: {
		milliseconds: number;
		ontick: () => void;
	} = $props();

	$effect(() => {
		if (!Number.isFinite(milliseconds) || milliseconds <= 0) return;
		const interval = window.setInterval(() => ontick(), milliseconds);
		return () => window.clearInterval(interval);
	});
</script>
