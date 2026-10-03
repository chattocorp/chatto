<!--
@component

Takes care of Web Push for the chat shell.

While the browser's notification permission is unset and a signed-in server
supports push, the component shows an invitation to enable push
notifications. Enable asks the browser for permission from that click, which
every browser accepts. Not now, or a dismissed browser prompt, hides the
invitation on this device for 14 days. Browsers stop showing a site's
permission prompt for a while after repeated dismissals, so Chatto never asks
the browser without the user's Enable.

After the browser grants permission, the component keeps every eligible
server's subscription saved. A server that becomes eligible later, for example
after it is added, is registered at once. When the window gets focus and once
an hour, the component checks every server: it saves a subscription again when
the last save in this page is a day old, failed, or the browser replaced the
subscription. These refreshes keep the server-side subscriptions from expiring
while the device is in use.

Include this component once in the chat root.
-->
<script lang="ts">
  import { serverRegistry } from '$lib/client';
  import { m } from '$lib/i18n/messages';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import {
    enablePushOnAllServers,
    getPermission,
    getPushCapability,
    getPushRegistrationTargets,
    refreshPushSubscriptions
  } from '$lib/notifications/pushNotifications';
  import { isPushPromptSnoozed, snoozePushPrompt } from '$lib/notifications/pushPrompt';
  import { TopOverlayNotice } from '$lib/ui';
  import { toast } from '$lib/ui/toast';

  const refreshCheckIntervalMs = 60 * 60 * 1000;

  let snoozed = $state(isPushPromptSnoozed());
  let enabling = $state(false);

  // The authentication notice uses the same place at the top of the window and
  // is more urgent, so the invitation waits while any server needs sign-in.
  const showPrompt = $derived(
    !snoozed &&
      getPermission() === 'default' &&
      getPushCapability() === 'supported' &&
      getPushRegistrationTargets().length > 0 &&
      !serverRegistry.servers.some((server) => server.reauthRequiredAt != null)
  );

  function onFocus(): void {
    // Another tab can choose Not now.
    snoozed = isPushPromptSnoozed();
    void refreshPushSubscriptions();
  }

  function notNow(): void {
    snoozePushPrompt();
    snoozed = true;
  }

  async function enable(): Promise<void> {
    enabling = true;
    try {
      const result = await enablePushOnAllServers();
      if (result.permission === 'default') {
        // The user dismissed the browser prompt; treat it as Not now.
        notNow();
      } else if (
        result.registrations.length > 0 &&
        result.registrations.every((registration) => registration.registered)
      ) {
        // A failed save shows its reason in the server's notification settings.
        toast.success(m('settings.notifications.push_prompt.enabled'));
      }
    } finally {
      enabling = false;
    }
  }

  $effect(() => {
    const targets = getPushRegistrationTargets();
    if (getPermission() !== 'granted' || targets.length === 0) return;

    void refreshPushSubscriptions();
  });
</script>

<svelte:window onfocus={onFocus} />
<Interval milliseconds={refreshCheckIntervalMs} ontick={() => void refreshPushSubscriptions()} />

{#if showPrompt}
  <TopOverlayNotice
    title={m('settings.notifications.push_prompt.title')}
    message={m('settings.notifications.push_prompt.message')}
    icon="icon-[uil--bell]"
    loading={enabling}
    primaryAction={{
      label: m('settings.notifications.push_prompt.enable'),
      onclick: () => void enable()
    }}
    secondaryAction={{ label: m('settings.notifications.push_prompt.not_now'), onclick: notNow }}
  />
{/if}
