<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import NotificationsPage from '../../notifications/NotificationsPage.svelte';
</script>

<NotificationsPage backHref={resolve('/chat/[serverId]', { serverId: page.params.serverId! })} />
