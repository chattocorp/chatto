<!-- @component App-wide update controls backed by the optional desktop host capability. -->
<script lang="ts">
  import { Panel, PageTitle, PaneContent, PaneHeader, Hint, LoadingFog } from '$lib/ui';
  import { Button, Checkbox, Select } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';
  import { desktopUpdates, type DesktopUpdates } from '$lib/desktop/updates.svelte';

  let { updates = desktopUpdates }: { updates?: DesktopUpdates } = $props();
  const state = $derived(updates.status);
  const statusText = $derived(state ? m(`settings.desktop.status.${state.status}`) : '');
</script>

<PageTitle title={m('settings.desktop.title')} scope="app" />
<div class="pane-page">
  <PaneHeader title={m('settings.desktop.title')} subtitle={m('settings.desktop.subtitle')} />
  <PaneContent>
    <div class="flex flex-col gap-6">
      <Panel title={m('settings.desktop.updates')} icon="iconify icon-[uil--download-alt]">
        {#if updates.loading}
          <LoadingFog class="h-24" />
        {:else if !state}
          <Hint>{m('settings.desktop.unavailable')}</Hint>
        {:else}
          <div class="flex flex-col gap-4">
            <p class="text-muted">{m('settings.desktop.version', { version: state.version })}</p>
            <div role="status" class="flex flex-col gap-2">
              <p class="font-medium">{statusText}</p>
              {#if state.availableVersion && ['ready', 'downloading', 'installing'].includes(state.status)}
                <p class="text-muted">Chatto {state.availableVersion}</p>
              {/if}
            </div>
            {#if state.status === 'downloading' && state.progress !== null}
              <progress
                class="w-full accent-action"
                max="100"
                value={state.progress}
                aria-label={statusText}>{state.progress}%</progress
              >
            {/if}
            {#if updates.failed}
              <Hint>{m('settings.desktop.status.error')}</Hint>
            {/if}
            {#if state.status === 'ready'}
              <p class="text-muted">{m('settings.desktop.restart_warning')}</p>
              <div>
                <Button disabled={updates.busy} onclick={() => updates.install()}
                  >{m('settings.desktop.restart')}</Button
                >
              </div>
            {:else}
              <div>
                <Button disabled={updates.busy} onclick={() => updates.check()}
                  >{m('settings.desktop.check')}</Button
                >
              </div>
            {/if}
          </div>
        {/if}
      </Panel>
      {#if state}
        <Panel title={m('settings.desktop.preferences')} icon="iconify icon-[uil--setting]">
          <div class="flex max-w-md flex-col gap-4">
            <Checkbox
              id="desktop-automatic"
              label={m('settings.desktop.automatic')}
              disabled={updates.busy}
              checked={state.automatic}
              onCheckedChange={(value) => updates.setAutomatic(value)}
            />
            <Select
              id="desktop-channel"
              label={m('settings.desktop.channel')}
              value={state.channel}
              disabled={updates.busy}
              options={[
                { value: 'stable', label: m('settings.desktop.stable') },
                { value: 'beta', label: m('settings.desktop.beta') }
              ]}
              onValueChange={(value) => updates.setChannel(value)}
            />
            <p class="text-muted">{m('settings.desktop.channel_help')}</p>
            <p class="text-muted">{m('settings.desktop.privacy')}</p>
          </div>
        </Panel>
      {/if}
    </div>
  </PaneContent>
</div>
