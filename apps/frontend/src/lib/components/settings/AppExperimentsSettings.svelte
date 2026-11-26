<!-- @component Browser-wide experimental features in App Preferences. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { experimentalFeatures } from '$lib/state/experimentalFeatures.svelte';
  import { PageTitle, PaneContent, PaneHeader, Panel } from '$lib/ui';
  import { Checkbox } from '$lib/ui/form';
</script>

<PageTitle title={m('settings.experiments.nav')} scope="app" />

<div class="pane-page">
  <PaneHeader
    title={m('settings.experiments.nav')}
    subtitle={m('settings.app_preferences.subtitle')}
  />
  <PaneContent>
    <Panel title={m('settings.experiments.title')} icon="iconify icon-[uil--flask]">
      <div class="flex max-w-md flex-col gap-3">
        <Checkbox
          id="new-message-header"
          label={m('settings.experiments.new_message_header')}
          bind:checked={experimentalFeatures.newMessageHeader.enabled}
        />
        <Checkbox
          id="swipe-to-close-thread"
          label={m('settings.experiments.swipe_to_close_thread')}
          bind:checked={experimentalFeatures.swipeToCloseThread.enabled}
        />
      </div>
    </Panel>
  </PaneContent>
</div>
