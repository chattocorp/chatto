<!-- @component Device-wide consent and a local test for the desktop host's native alerts. -->
<script lang="ts">
  import {
    desktopNotifications,
    hasDesktopNotifications,
    showDesktopNotification,
    clearDesktopNotifications
  } from '$lib/desktop/notifications.svelte';
  import { m } from '$lib/i18n/messages';
  import { Panel, Hint } from '$lib/ui';
  import { Checkbox, Button } from '$lib/ui/form';
  let testing = $state(false);
  let failed = $state(false);
  let sent = $state(false);
  async function test() {
    testing = true;
    failed = false;
    sent = false;
    try {
      clearDesktopNotifications(['test']);
      await showDesktopNotification(
        'test',
        { title: 'Chatto', body: m('settings.notifications.push.test_sent') },
        () => {}
      );
      sent = true;
    } catch {
      failed = true;
    } finally {
      testing = false;
    }
  }
</script>

{#if hasDesktopNotifications()}
  <Panel title={m('settings.notifications.push.title')} icon="iconify icon-[uil--bell]">
    <div class="flex max-w-2xl flex-col gap-4">
      <Checkbox
        id="desktop-notifications"
        label={m('settings.notifications.push.label')}
        description={m('settings.notifications.push.description', { device: 'Chatto Desktop' })}
        bind:checked={desktopNotifications.enabled}
      />
      {#if desktopNotifications.enabled}
        <div>
          <Button size="sm" onclick={test} loading={testing} disabled={testing}
            >{m('settings.notifications.push.test_button')}</Button
          >
        </div>
      {/if}
      {#if failed}<Hint tone="warning">{m('settings.notifications.push.blocked_description')}</Hint
        >{/if}
      {#if sent}<p role="status" class="text-success">
          {m('settings.notifications.push.test_sent')}
        </p>{/if}
    </div>
  </Panel>
{/if}
