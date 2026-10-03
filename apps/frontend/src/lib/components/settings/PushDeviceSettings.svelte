<!--
@component

Shows whether the current server sends push notifications to this device,
lets the user send a test notification, and turns push on or off for this
device. Push is on for every registered server or for none of them, so the
switch in any server's panel applies to all servers.

The panel names the browser and platform that receive the notifications and
the origin that notification clicks open. It never asks for notification
permission without the user: while permission is unset, the panel offers an
Enable action that asks the browser from that click. Renders nothing when the
server has no Web Push configuration or the app does not run in a browser.
-->
<script lang="ts">
  import { Code, ConnectError } from '@connectrpc/connect';
  import { m } from '$lib/i18n/messages';
  import { describePushDevice } from '$lib/notifications/pushDevice';
  import {
    disablePushOnAllServers,
    enablePushOnAllServers,
    getPermission,
    getPushCapability,
    hasSavedPushRegistration,
    isBrowserWebPushRuntime,
    isPushDisabledOnThisDevice,
    pushRegistrationFailure,
    retryPushRegistration,
    sendTestNotification
  } from '$lib/notifications/pushNotifications';
  import { snoozePushPrompt } from '$lib/notifications/pushPrompt';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Hint, Panel } from '$lib/ui';
  import { Button } from '$lib/ui/form';

  type TestResult = { kind: 'sent' } | { kind: 'failed'; message: string };

  const serverScope = useServerScope();
  const serverId = serverScope.serverId;
  const store = serverScope.store;

  const capability = getPushCapability();
  const device = describePushDevice(navigator.userAgent, navigator.maxTouchPoints);
  const mobile =
    device.platform === 'iOS' || device.platform === 'iPadOS' || device.platform === 'Android';
  const deviceLabel =
    device.browser && device.platform
      ? m('settings.notifications.push.device', {
          browser: device.browser,
          platform: device.platform
        })
      : (device.browser ?? m('settings.notifications.push.device_unknown'));

  const visible = $derived(isBrowserWebPushRuntime() && store.serverInfo.pushNotificationsEnabled);
  const permission = $derived(getPermission());
  const registered = $derived(hasSavedPushRegistration(serverId, store.accountId));
  const registrationFailure = $derived(pushRegistrationFailure(serverId));
  const disabledOnDevice = $derived(isPushDisabledOnThisDevice());
  let retrying = $state(false);
  let enabling = $state(false);
  let disabling = $state(false);
  let turnOffFailed = $state(false);
  /** The browser closed the permission request, or did not show it at all. */
  let promptUnanswered = $state(false);

  let testing = $state(false);
  let testResult = $state<TestResult | null>(null);

  async function sendTest(): Promise<void> {
    testing = true;
    testResult = null;
    try {
      await sendTestNotification(serverId);
      testResult = { kind: 'sent' };
    } catch (error) {
      // The server reports every delivery failure as unavailable, so a
      // generic network message would mislead here.
      const rateLimited = ConnectError.from(error).code === Code.ResourceExhausted;
      testResult = {
        kind: 'failed',
        message: rateLimited
          ? m('settings.notifications.push.test_rate_limited')
          : m('settings.notifications.push.test_failed')
      };
    } finally {
      testing = false;
    }
  }

  async function enable(): Promise<void> {
    enabling = true;
    turnOffFailed = false;
    try {
      const result = await enablePushOnAllServers();
      promptUnanswered = result.permission === 'default';
      // A dismissed browser prompt counts as Not now for the invitation too.
      if (promptUnanswered) snoozePushPrompt();
    } finally {
      enabling = false;
    }
  }

  async function turnOff(): Promise<void> {
    disabling = true;
    turnOffFailed = false;
    try {
      await disablePushOnAllServers();
    } catch (error) {
      console.error('Failed to turn push notifications off:', error);
      turnOffFailed = true;
    } finally {
      disabling = false;
    }
  }

  async function retryRegistration(): Promise<void> {
    retrying = true;
    try {
      await retryPushRegistration(serverId);
    } finally {
      retrying = false;
    }
  }
</script>

{#if visible}
  <Panel title={m('settings.notifications.push.title')} icon="iconify icon-[uil--bell]">
    <div class="flex max-w-2xl flex-col gap-4" data-testid="push-notification-settings">
      {#if capability === 'ios_home_screen_required'}
        <Hint icon="icon-[uil--mobile-android]">
          <p class="font-medium">{m('settings.notifications.push.ios_home_screen_title')}</p>
          <p>{m('settings.notifications.push.ios_home_screen_description')}</p>
        </Hint>
      {:else if capability !== 'supported'}
        <Hint>{m('settings.notifications.push.not_supported')}</Hint>
      {:else if permission === 'denied'}
        <Hint tone="warning" icon="icon-[uil--bell-slash]">
          <p class="font-medium">{m('settings.notifications.push.blocked_title')}</p>
          <p>{m('settings.notifications.push.blocked_description')}</p>
        </Hint>
      {:else if disabledOnDevice}
        <div class="flex flex-col gap-1">
          <p class="font-medium text-text-top">
            {m('settings.notifications.push.disabled_title')}
          </p>
          <p class="text-muted">{m('settings.notifications.push.device_wide')}</p>
        </div>
        <div>
          <Button
            size="sm"
            onclick={enable}
            disabled={enabling}
            loading={enabling}
            loadingText={m('settings.notifications.push_prompt.enabling')}
          >
            {m('settings.notifications.push_prompt.title')}
          </Button>
        </div>
      {:else if permission !== 'granted'}
        <div class="flex flex-col gap-1">
          <p class="font-medium text-text-top">{m('settings.notifications.push.off_title')}</p>
          <p class="text-muted">{m('settings.notifications.push_prompt.message')}</p>
        </div>
        {#if promptUnanswered}
          <Hint tone="warning">
            <p>{m('settings.notifications.push.prompt_unanswered')}</p>
          </Hint>
        {/if}
        <div>
          <Button
            size="sm"
            onclick={enable}
            disabled={enabling}
            loading={enabling}
            loadingText={m('settings.notifications.push_prompt.enabling')}
          >
            {m('settings.notifications.push_prompt.title')}
          </Button>
        </div>
      {:else if !registered && registrationFailure}
        <Hint tone="warning">
          <p class="font-medium">{m('settings.notifications.push.setup_failed_title')}</p>
          <p>{m('settings.notifications.push.setup_failed_description')}</p>
          <p class="mt-2 break-words" data-testid="push-setup-failure">
            {m('settings.notifications.push.setup_failed_details', {
              reason: registrationFailure
            })}
          </p>
        </Hint>
        <div>
          <Button
            variant="secondary"
            size="sm"
            onclick={retryRegistration}
            disabled={retrying}
            loading={retrying}
          >
            {m('common.retry')}
          </Button>
        </div>
      {:else if !registered}
        <p class="text-muted" role="status">{m('settings.notifications.push.registering')}</p>
      {:else}
        <p>{m('settings.notifications.push.active_description')}</p>
        <div class="flex items-center gap-3 surface-box p-3" data-testid="push-device">
          <span
            class="iconify shrink-0 text-2xl text-muted {mobile
              ? 'icon-[uil--mobile-android]'
              : 'icon-[uil--desktop]'}"
            aria-hidden="true"
          ></span>
          <div class="min-w-0">
            <p class="font-medium text-text-top">{deviceLabel}</p>
            <p class="truncate text-muted">
              {m('settings.notifications.push.opens_at', { origin: window.location.host })}
            </p>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <Button
            size="sm"
            onclick={sendTest}
            disabled={testing}
            loading={testing}
            loadingText={m('settings.notifications.push.testing')}
          >
            {m('settings.notifications.push.test_button')}
          </Button>
          {#if testResult?.kind === 'sent'}
            <p class="text-success" role="status">{m('settings.notifications.push.test_sent')}</p>
          {:else if testResult?.kind === 'failed'}
            <p class="text-danger" role="alert">{testResult.message}</p>
          {/if}
        </div>
      {/if}
      {#if turnOffFailed}
        <Hint tone="warning">
          <p>{m('settings.notifications.push.turn_off_failed')}</p>
        </Hint>
        <div>
          <Button
            variant="secondary"
            size="sm"
            onclick={turnOff}
            disabled={disabling}
            loading={disabling}
          >
            {m('common.retry')}
          </Button>
        </div>
      {:else if capability === 'supported' && permission === 'granted' && !disabledOnDevice}
        <div class="flex flex-col items-start gap-2">
          <p class="text-muted">{m('settings.notifications.push.device_wide')}</p>
          <Button
            variant="secondary"
            size="sm"
            onclick={turnOff}
            disabled={disabling}
            loading={disabling}
            loadingText={m('settings.notifications.push.turning_off')}
          >
            {m('settings.notifications.push.turn_off')}
          </Button>
        </div>
      {/if}
    </div>
  </Panel>
{/if}
