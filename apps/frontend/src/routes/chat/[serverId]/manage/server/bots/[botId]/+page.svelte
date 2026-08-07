<script lang="ts">
	import { page } from '$app/state';
	import { BotDetail } from '$lib/components/bots';

	const botId = $derived(page.params.botId!);
</script>

<BotDetail {botId} scope="admin" />
