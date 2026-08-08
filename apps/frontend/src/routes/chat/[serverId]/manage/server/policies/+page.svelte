<script lang="ts">
	import { m } from '$lib/i18n/messages';
	import { PaneContent } from '$lib/ui';
	import PaneHeader from '$lib/ui/PaneHeader.svelte';
	import PageTitle from '$lib/ui/PageTitle.svelte';
	import PolicySettingsPanel from '../../PolicySettingsPanel.svelte';
</script>

<PageTitle
	title={m('admin.common.server_admin_page_title', { title: m('admin.policies.title') })}
/>

<PaneHeader
	title={m('admin.policies.title')}
	subtitle={m('admin.policies.subtitle')}
	showMobileNav
/>

<PaneContent>
	<PolicySettingsPanel scope={{ kind: 'server' }} />
</PaneContent>
