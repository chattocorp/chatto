<!-- @component Shared badge for an active bot account. -->
<script lang="ts">
	import { BOT_ACCOUNT_LABEL } from '$lib/render/accountName';
	import IdentityBadge from './IdentityBadge.svelte';

	let { size = 'sm' }: { size?: 'sm' | 'md' } = $props();
</script>

<IdentityBadge label={BOT_ACCOUNT_LABEL} testId="bot-badge" {size} />
