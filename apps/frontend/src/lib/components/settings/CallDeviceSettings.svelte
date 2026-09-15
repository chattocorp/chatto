<!-- @component Browser-local call defaults and an explicitly started microphone test. -->
<script lang="ts">
  import MicrophoneSensitivity from './MicrophoneSensitivity.svelte';
  import { onMount } from 'svelte';
  import { m } from '$lib/i18n/messages';
  import { ChoiceRow, Hint, PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import Panel from '$lib/ui/Panel.svelte';
  import { Button, Checkbox } from '$lib/ui/form';
  import type { CallPreferencesState } from '$lib/state/server/callPreferences.svelte';
  import { CallDeviceTest } from '$lib/state/server/callDeviceTest.svelte';

  let {
    preferences,
    inCall = false,
    callLevel = 0,
    gateUnavailable = false
  }: {
    preferences: CallPreferencesState;
    inCall?: boolean;
    callLevel?: number;
    gateUnavailable?: boolean;
  } = $props();
  const test = new CallDeviceTest();
  let devices = $state<MediaDeviceInfo[]>([]);
  let deviceError = $state(false);
  let outputSupported = $state(false);
  let alive = false;
  let refreshGeneration = 0;

  async function refresh() {
    const generation = ++refreshGeneration;
    try {
      const result = await navigator.mediaDevices.enumerateDevices();
      if (!alive || generation !== refreshGeneration) return;
      devices = result;
      deviceError = false;
      return result;
    } catch {
      if (alive && generation === refreshGeneration) deviceError = true;
    }
  }

  onMount(() => {
    alive = true;
    outputSupported = 'setSinkId' in HTMLMediaElement.prototype;
    void discoverDevices();
    navigator.mediaDevices?.addEventListener('devicechange', refresh);
    return () => {
      alive = false;
      refreshGeneration++;
      navigator.mediaDevices?.removeEventListener('devicechange', refresh);
      test.stop();
    };
  });

  function options(kind: MediaDeviceKind, selected: string) {
    const matching = devices.filter(
      (device) => device.kind === kind && device.deviceId && device.deviceId !== 'default'
    );
    const result = [
      { value: '', label: m('voice.preferences.system_default') },
      ...matching.map((device) => ({
        value: device.deviceId,
        label: device.label || m('voice.unknown_device')
      }))
    ];
    if (selected && !result.some((option) => option.value === selected))
      result.push({ value: selected, label: m('voice.preferences.unavailable_device') });
    return result;
  }

  function select(kind: MediaDeviceKind, value: string) {
    test.stop();
    preferences.setDevice(kind, value);
  }

  async function startTest() {
    const speaker = devices.some(
      (device) => device.kind === 'audiooutput' && device.deviceId === preferences.speaker
    )
      ? preferences.speaker
      : '';
    await test.start(preferences.microphone, speaker, () => preferences.microphoneThreshold);
    if (alive) await refresh();
  }

  async function discoverDevices() {
    const listed = await refresh();
    if (!alive || inCall || listed?.some((device) => device.kind === 'videoinput' && device.label))
      return;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ video: true, audio: false });
      stream.getTracks().forEach((track) => track.stop());
      if (alive) await refresh();
    } catch (error) {
      if (alive && !(error instanceof DOMException && error.name === 'NotFoundError'))
        deviceError = true;
    }
  }
</script>

<PageTitle title={m('voice.preferences.title')} />
<PaneHeader title={m('voice.preferences.title')} subtitle={m('voice.preferences.scope')} />
<PaneContent>
  <div class="flex flex-col gap-6">
    <Panel title={m('voice.devices')} icon="iconify icon-[uil--headphones]">
      <div class="flex max-w-xl flex-col gap-5">
        <div>
          <h3 class="mb-2 font-medium">{m('voice.microphone')}</h3>
          <div role="radiogroup" aria-label={m('voice.microphone')} class="flex flex-col gap-2">
            {#each options('audioinput', preferences.microphone) as option (option.value)}
              <ChoiceRow
                label={option.label}
                selected={preferences.microphone === option.value}
                onclick={() => select('audioinput', option.value)}
              />
            {/each}
          </div>
        </div>
        <div>
          <h3 class="mb-2 font-medium">{m('voice.speaker')}</h3>
          <div role="radiogroup" aria-label={m('voice.speaker')} class="flex flex-col gap-2">
            {#each options('audiooutput', preferences.speaker) as option (option.value)}
              <ChoiceRow
                label={option.label}
                selected={preferences.speaker === option.value}
                disabled={!outputSupported}
                onclick={() => select('audiooutput', option.value)}
              />
            {/each}
          </div>
          {#if !outputSupported}<p class="mt-2 text-muted">
              {m('voice.preferences.output_unsupported')}
            </p>{/if}
        </div>
        <div>
          <h3 class="mb-2 font-medium">{m('voice.camera')}</h3>
          <div role="radiogroup" aria-label={m('voice.camera')} class="flex flex-col gap-2">
            {#each options('videoinput', preferences.camera) as option (option.value)}
              <ChoiceRow
                label={option.label}
                selected={preferences.camera === option.value}
                onclick={() => select('videoinput', option.value)}
              />
            {/each}
          </div>
        </div>
        {#if deviceError}<Hint>{m('voice.media_device_failed')}</Hint>{/if}
      </div>
    </Panel>
    <Panel title={m('voice.preferences.call_settings')} icon="iconify icon-[uil--phone]">
      <Checkbox
        id="call-join-muted"
        label={m('voice.preferences.join_muted')}
        bind:checked={() => preferences.joinMuted, (value) => preferences.setJoinMuted(value)}
      />
    </Panel>
    <Panel title={m('voice.preferences.test_title')} icon="iconify icon-[uil--microphone]">
      <div class="flex max-w-xl flex-col gap-4">
        <MicrophoneSensitivity
          {preferences}
          level={inCall ? callLevel : test.level}
          unavailable={inCall ? gateUnavailable : test.gateUnavailable}
        />
        <div class="flex flex-wrap gap-2">
          {#if !test.active && !test.pending}
            <Button onclick={startTest} disabled={inCall}
              >{m('voice.preferences.start_test')}</Button
            >
          {:else}
            <Button variant="secondary" onclick={() => test.stop()}
              >{m('voice.preferences.stop_test')}</Button
            >
          {/if}
        </div>
        {#if test.pending}<p role="status">{m('voice.preferences.waiting')}</p>{/if}
        {#if test.error}<Hint>{m('voice.preferences.test_failed')}</Hint>{/if}
        {#if inCall}<Hint>{m('voice.preferences.test_in_call')}</Hint>{/if}
      </div>
    </Panel>
  </div>
</PaneContent>
