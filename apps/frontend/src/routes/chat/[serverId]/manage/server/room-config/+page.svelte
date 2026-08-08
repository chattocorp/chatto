<script lang="ts">
	import { m } from '$lib/i18n/messages';
	import { PaneContent } from '$lib/ui';
	import PaneHeader from '$lib/ui/PaneHeader.svelte';
	import PageTitle from '$lib/ui/PageTitle.svelte';
	import RoomConfigPanel from '../../RoomConfigPanel.svelte';
</script>

<PageTitle
	title={m('admin.common.server_admin_page_title', { title: m('admin.room_config.title') })}
/>

<PaneHeader
	title={m('admin.room_config.title')}
	subtitle={m('admin.room_config.subtitle')}
	showMobileNav
/>

<PaneContent>
	<RoomConfigPanel scope={{ kind: 'server' }} />
</PaneContent>
