<!-- @component Browser-local call defaults and an explicitly started microphone test. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { m } from '$lib/i18n/messages';
  import { Hint, PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import Panel from '$lib/ui/Panel.svelte';
  import { Button, Checkbox, Select } from '$lib/ui/form';
  import type { CallPreferencesState } from '$lib/state/server/callPreferences.svelte';
  import { CallDeviceTest } from '$lib/state/server/callDeviceTest.svelte';

  let { preferences, inCall = false }: { preferences: CallPreferencesState; inCall?: boolean } =
    $props();
  const test = new CallDeviceTest();
  let devices = $state<MediaDeviceInfo[]>([]);
  let deviceError = $state(false);
  let outputSupported = $state(false);
  let recordingSupported = $state(false);
  let cameraPending = $state(false);
  let alive = false;
  let refreshGeneration = 0;

  async function refresh() {
    const generation = ++refreshGeneration;
    try {
      const result = await navigator.mediaDevices.enumerateDevices();
      if (!alive || generation !== refreshGeneration) return;
      devices = result;
      deviceError = false;
    } catch {
      if (alive && generation === refreshGeneration) deviceError = true;
    }
  }

  onMount(() => {
    alive = true;
    outputSupported = 'setSinkId' in HTMLMediaElement.prototype;
    recordingSupported = typeof MediaRecorder !== 'undefined';
    void refresh();
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
    await test.start(preferences.microphone);
    if (alive) await refresh();
  }

  async function showCameras() {
    if (cameraPending) return;
    cameraPending = true;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ video: true, audio: false });
      stream.getTracks().forEach((track) => track.stop());
      if (alive) await refresh();
    } catch {
      if (alive) deviceError = true;
    } finally {
      if (alive) cameraPending = false;
    }
  }

  function playback(node: HTMLAudioElement) {
    let current = true;
    if (outputSupported) {
      const preferred = devices.some(
        (device) => device.kind === 'audiooutput' && device.deviceId === preferences.speaker
      )
        ? preferences.speaker
        : '';
      void node.setSinkId(preferred).catch(() => {
        if (current) deviceError = true;
      });
    }
    return () => {
      current = false;
      node.pause();
      node.removeAttribute('src');
      node.load();
    };
  }
</script>

<PageTitle title={m('voice.preferences.title')} />
<PaneHeader title={m('voice.preferences.title')} subtitle={m('voice.preferences.scope')} />
<PaneContent>
  <div class="flex flex-col gap-6">
    <Panel title={m('voice.devices')} icon="iconify icon-[uil--headphones]">
      <div class="flex max-w-xl flex-col gap-5">
        <Select
          id="call-microphone"
          label={m('voice.microphone')}
          options={options('audioinput', preferences.microphone)}
          bind:value={() => preferences.microphone, (value) => select('audioinput', value)}
        />
        <Select
          id="call-speaker"
          label={m('voice.speaker')}
          options={options('audiooutput', preferences.speaker)}
          disabled={!outputSupported}
          bind:value={() => preferences.speaker, (value) => select('audiooutput', value)}
        />
        {#if !outputSupported}<p class="text-muted">
            {m('voice.preferences.output_unsupported')}
          </p>{/if}
        <Select
          id="call-camera"
          label={m('voice.camera')}
          options={options('videoinput', preferences.camera)}
          bind:value={() => preferences.camera, (value) => select('videoinput', value)}
        />
        <div>
          <Button variant="secondary" onclick={showCameras} disabled={inCall || cameraPending}
            >{m('voice.preferences.show_cameras')}</Button
          >
        </div>
        <p class="text-muted">{m('voice.preferences.camera_access')}</p>
        <p class="text-muted">{m('voice.preferences.fallback')}</p>
        <Checkbox
          id="call-join-muted"
          label={m('voice.preferences.join_muted')}
          bind:checked={() => preferences.joinMuted, (value) => preferences.setJoinMuted(value)}
        />
        {#if inCall}<Hint>{m('voice.preferences.next_call')}</Hint>{/if}
        {#if deviceError}<Hint>{m('voice.media_device_failed')}</Hint>{/if}
      </div>
    </Panel>
    <Panel title={m('voice.preferences.test_title')} icon="iconify icon-[uil--microphone]">
      <div class="flex max-w-xl flex-col gap-4">
        <p class="text-muted">{m('voice.preferences.test_description')}</p>
        <span id="call-input-label">{m('voice.preferences.input_level')}</span>
        <div
          role="meter"
          aria-labelledby="call-input-label"
          aria-valuemin="0"
          aria-valuemax="1"
          aria-valuenow={test.level}
          class="h-3 overflow-hidden rounded-full bg-surface"
        >
          <div class="h-full rounded-full bg-action" style:width={`${test.level * 100}%`}></div>
        </div>
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
          {#if test.active && !test.recording}
            <Button variant="secondary" onclick={() => test.record()} disabled={!recordingSupported}
              >{m('voice.preferences.record')}</Button
            >
          {:else if test.recording}
            <Button onclick={() => test.finishRecording()}
              >{m('voice.preferences.finish_recording')}</Button
            >
          {/if}
        </div>
        {#if test.pending}<p role="status">{m('voice.preferences.waiting')}</p>{/if}
        {#if test.recording}<p role="status">{m('voice.preferences.recording')}</p>{/if}
        {#if test.error}<Hint>{m('voice.preferences.test_failed')}</Hint>{/if}
        {#if inCall}<Hint>{m('voice.preferences.test_in_call')}</Hint>{/if}
        {#if !recordingSupported}<p class="text-muted">
            {m('voice.preferences.recording_unsupported')}
          </p>{/if}
        {#if test.clipURL}
          <audio
            controls
            src={test.clipURL}
            {@attach playback}
            aria-label={m('voice.preferences.playback')}
            class="w-full"
          ></audio>
        {/if}
      </div>
    </Panel>
  </div>
</PaneContent>
