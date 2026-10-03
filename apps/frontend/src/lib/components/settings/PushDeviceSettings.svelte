<!--
@component

Turns push notifications on or off for this device and lets the user send a
test notification through the current server.

Push is on for every registered server or for none of them, so the checkbox in
any server's panel applies to all servers. Checking it asks the browser for
notification permission from that click; Chatto never asks without the user.
The panel explains instead of offering the checkbox when this browser cannot
use push or blocks notifications. Renders nothing when the server has no Web
Push configuration or the app does not run in a browser.
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
  import { Button, Checkbox } from '$lib/ui/form';

  type TestResult = { kind: 'sent' } | { kind: 'failed'; message: string };
  /** A problem of the latest checkbox change that the browser state does not show. */
  type ToggleProblem = 'prompt_unanswered' | 'turn_off_failed';

  const serverScope = useServerScope();
  const serverId = serverScope.serverId;
  const store = serverScope.store;

  const capability = getPushCapability();
  const device = describePushDevice(navigator.userAgent, navigator.maxTouchPoints);
  const deviceLabel =
    device.browser && device.platform
      ? m('settings.notifications.push.device', {
          browser: device.browser,
          platform: device.platform
        })
      : (device.browser ?? m('settings.notifications.push.device_unknown'));

  const visible = $derived(isBrowserWebPushRuntime() && store.serverInfo.pushNotificationsEnabled);
  const permission = $derived(getPermission());
  const on = $derived(permission === 'granted' && !isPushDisabledOnThisDevice());
  /** Follows `on`; the checkbox overrides it until its change finishes. */
  let checked = $derived(on);
  const registered = $derived(hasSavedPushRegistration(serverId, store.accountId));
  const registrationFailure = $derived(
    on && !registered ? pushRegistrationFailure(serverId) : null
  );

  let changing = $state(false);
  let problem = $state<ToggleProblem | null>(null);
  let retrying = $state(false);
  let testing = $state(false);
  let testResult = $state<TestResult | null>(null);

  const error = $derived.by(() => {
    if (problem === 'prompt_unanswered') {
      return m('settings.notifications.push.prompt_unanswered');
    }
    if (problem === 'turn_off_failed') return m('settings.notifications.push.turn_off_failed');
    if (registrationFailure) {
      return m('settings.notifications.push.setup_failed', { reason: registrationFailure });
    }
    return undefined;
  });
  const description = $derived(
    on && !registered
      ? m('settings.notifications.push.registering')
      : m('settings.notifications.push.description', { device: deviceLabel })
  );

  async function change(event: Event): Promise<void> {
    // Read the new value before the first await: the browser accepts the
    // permission request only during this click.
    const turnOn = (event.currentTarget as HTMLInputElement).checked;
    changing = true;
    problem = null;
    try {
      if (turnOn) {
        const result = await enablePushOnAllServers();
        if (result.permission === 'default') {
          problem = 'prompt_unanswered';
          // A dismissed browser prompt counts as Not now for the invitation too.
          snoozePushPrompt();
        }
      } else {
        await disablePushOnAllServers();
      }
    } catch (error) {
      console.error('Failed to change push notifications on this device:', error);
      if (!turnOn) problem = 'turn_off_failed';
    } finally {
      changing = false;
      checked = on;
    }
  }

  /** Retries the failed turn-off, or else the failed save for this server. */
  async function retry(): Promise<void> {
    const turningOff = problem === 'turn_off_failed';
    retrying = true;
    problem = null;
    try {
      if (turningOff) await disablePushOnAllServers();
      else await retryPushRegistration(serverId);
    } catch (error) {
      // A failed save shows its reason through the registration state.
      console.error('Failed to retry push notifications on this device:', error);
      if (turningOff) problem = 'turn_off_failed';
    } finally {
      retrying = false;
    }
  }

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
      {:else}
        <Checkbox
          id="push-on-this-device"
          label={m('settings.notifications.push.label')}
          {description}
          {error}
          loading={changing}
          bind:checked
          onchange={change}
        />
        {#if problem === 'turn_off_failed' || (registrationFailure && !problem)}
          <div>
            <Button
              variant="secondary"
              size="sm"
              onclick={retry}
              disabled={retrying}
              loading={retrying}
            >
              {m('common.retry')}
            </Button>
          </div>
        {:else if on && registered}
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
              <p class="text-success" role="status">
                {m('settings.notifications.push.test_sent')}
              </p>
            {:else if testResult?.kind === 'failed'}
              <p class="text-danger" role="alert">{testResult.message}</p>
            {/if}
          </div>
        {/if}
      {/if}
    </div>
  </Panel>
{/if}
