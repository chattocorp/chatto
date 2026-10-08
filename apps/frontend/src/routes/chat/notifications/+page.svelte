<script lang="ts">
  import NotificationFeed from '$lib/components/notifications/NotificationFeed.svelte';
  import { PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
</script>

<PageTitle title={m('chat.notifications.title')} />

<div class="pane-page">
  <PaneHeader title={m('chat.notifications.title')} subtitle={m('chat.notifications.subtitle')} />

  <PaneContent fillHeight>
    <NotificationFeed />
  </PaneContent>
</div>
