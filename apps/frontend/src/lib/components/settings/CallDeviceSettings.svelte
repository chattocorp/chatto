<!-- @component Browser-local call defaults and an explicitly started microphone test. -->
<script lang="ts">
  import MicrophoneProcessing from './MicrophoneProcessing.svelte';
  import { onMount } from 'svelte';
  import { m } from '$lib/i18n/messages';
  import { ChoiceRow, Hint, PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import Panel from '$lib/ui/Panel.svelte';
  import { Button, Checkbox } from '$lib/ui/form';
  import type { CallPreferencesState } from '$lib/state/server/callPreferences.svelte';
  import { CallDeviceTest } from '$lib/state/server/callDeviceTest.svelte';

  let {
    preferences,
    onDeviceChange,
    inCall = false,
    callLevel = 0,
    gateUnavailable = false
  }: {
    preferences: CallPreferencesState;
    /** The active call owns switching and persists only successful changes. */
    onDeviceChange?: (kind: MediaDeviceKind, deviceId: string) => Promise<void>;
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
    outputSupported =
      (typeof AudioContext !== 'undefined' && 'setSinkId' in AudioContext.prototype) ||
      'setSinkId' in HTMLMediaElement.prototype;
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

  async function select(kind: MediaDeviceKind, value: string) {
    if (inCall && onDeviceChange) {
      await onDeviceChange(kind, value);
      return;
    }
    if (kind === 'audiooutput' && test.active) {
      if (await test.setSpeaker(value)) preferences.setDevice(kind, value);
      return;
    }
    const restart = kind !== 'videoinput' && (test.active || test.pending);
    preferences.setDevice(kind, value);
    if (restart) await startTest();
  }

  async function startTest() {
    await test.start(
      preferences.microphone,
      preferences.speaker,
      () => preferences.microphoneThreshold,
      () => preferences.effects
    );
    if (alive) await refresh();
  }

  // Request each input separately so a missing or blocked camera cannot hide audio devices.
  // Discovery never plays or publishes media, and releases even late capture results.
  async function discoverDevices() {
    let listed = await refresh();
    let discoveryFailed = false;
    for (const kind of ['audioinput', 'videoinput'] as const) {
      if (!alive || inCall) return;
      if (listed?.some((device) => device.kind === kind && device.label)) continue;
      try {
        const stream = await navigator.mediaDevices.getUserMedia({
          audio: kind === 'audioinput',
          video: kind === 'videoinput'
        });
        stream.getTracks().forEach((track) => track.stop());
        if (alive) listed = await refresh();
      } catch (error) {
        if (!(error instanceof DOMException && error.name === 'NotFoundError'))
          discoveryFailed = true;
      }
    }
    if (alive && discoveryFailed) deviceError = true;
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
    <Panel title={m('voice.preferences.processing')} icon="iconify icon-[uil--microphone]">
      <div class="flex max-w-xl flex-col gap-4">
        <MicrophoneProcessing
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
