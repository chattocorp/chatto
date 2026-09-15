import { userEvent } from 'vitest/browser';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { OutputAudioContext } from '$lib/state/server/callDeviceTest.svelte';
import { MicrophoneProcessor } from '$lib/audio/microphoneProcessor';
import { CallPreferencesState } from '$lib/state/server/callPreferences.svelte';
import CallDeviceSettings from './CallDeviceSettings.svelte';

const visibleCamera = {
  kind: 'videoinput',
  deviceId: 'camera',
  label: 'Camera'
} as MediaDeviceInfo;

describe('Call device settings', () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => vi.restoreAllMocks());

  it('persists threshold changes made with the keyboard', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([visibleCamera]);
    const screen = render(CallDeviceSettings, {
      preferences: new CallPreferencesState('threshold-control')
    });
    const slider = screen.container.querySelector<HTMLInputElement>('input[type=range]')!;
    slider.focus();
    await userEvent.keyboard('{Home}{ArrowRight}');
    expect(new CallPreferencesState('threshold-control').microphoneThreshold).toBe(-59);
  });

  it('shows remembered unavailable devices without requesting capture', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([visibleCamera]);
    const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia');
    const preferences = new CallPreferencesState('settings-test');
    preferences.setDevice('audioinput', 'missing');
    const screen = render(CallDeviceSettings, { preferences });
    await expect
      .element(screen.getByRole('radio', { name: 'Saved device unavailable' }))
      .toBeInTheDocument();
    expect(capture).not.toHaveBeenCalled();
    await screen.getByRole('checkbox', { name: 'Join calls with microphone muted' }).click();
    expect(new CallPreferencesState('settings-test').joinMuted).toBe(true);
    await screen
      .getByRole('radiogroup', { name: 'Microphone', exact: true })
      .getByRole('radio', { name: 'System default' })
      .click();
    expect(new CallPreferencesState('settings-test').microphone).toBe('');
  });

  it('keeps live monitoring active when the device list refreshes', async () => {
    const enumerate = vi
      .spyOn(navigator.mediaDevices, 'enumerateDevices')
      .mockImplementation(async () => [visibleCamera]);
    const context = new AudioContext();
    const oscillator = context.createOscillator();
    const destination = context.createMediaStreamDestination();
    oscillator.connect(destination);
    oscillator.start();
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockImplementation(async () => {
      await context.resume();
      return destination.stream;
    });
    const monitor = vi.spyOn(MicrophoneProcessor.prototype, 'connectMonitor');
    const preferences = new CallPreferencesState('playback-refresh');
    preferences.setVoiceAmount(50);
    const screen = render(CallDeviceSettings, { preferences });
    try {
      await screen.getByRole('button', { name: 'Start microphone test' }).click();
      await expect.element(screen.getByRole('button', { name: 'Stop test' })).toBeInTheDocument();
      await vi.waitFor(() => expect(monitor).toHaveBeenCalledOnce());
      const output = monitor.mock.calls[0][0];
      const analyser = output.context.createAnalyser();
      (monitor.mock.contexts[0] as MicrophoneProcessor).connectMonitor(analyser);
      const samples = new Float32Array(analyser.fftSize);
      const expectOutput = () => {
        expect(output.context.state).toBe('running');
        analyser.getFloatTimeDomainData(samples);
        expect(Math.max(...samples.map(Math.abs))).toBeGreaterThan(0.1);
      };
      await vi.waitFor(expectOutput);
      const callsBefore = enumerate.mock.calls.length;
      navigator.mediaDevices.dispatchEvent(new Event('devicechange'));
      await vi.waitFor(() => expect(enumerate.mock.calls.length).toBeGreaterThan(callsBefore));
      await tick();
      expect(destination.stream.getAudioTracks()[0].readyState).toBe('live');
      await vi.waitFor(expectOutput);
    } finally {
      await screen.unmount();
      oscillator.stop();
      await context.close();
    }
  });

  it('requests camera access directly when camera names are hidden and releases it immediately', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([]);
    const stop = vi.fn();
    const capture = vi
      .spyOn(navigator.mediaDevices, 'getUserMedia')
      .mockResolvedValue({ getTracks: () => [{ stop }] } as unknown as MediaStream);
    const screen = render(CallDeviceSettings, {
      preferences: new CallPreferencesState('discover')
    });
    await vi.waitFor(() => expect(capture).toHaveBeenCalledWith({ video: true, audio: false }));
    expect(stop).toHaveBeenCalledOnce();
    expect(screen.container.textContent).not.toContain('Show cameras');
  });

  it('does not request discovery access during a call', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([]);
    const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia');
    render(CallDeviceSettings, {
      preferences: new CallPreferencesState('busy-discovery'),
      inCall: true
    });
    await tick();
    expect(capture).not.toHaveBeenCalled();
  });

  it('blocks microphone testing during a call', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([visibleCamera]);
    const screen = render(CallDeviceSettings, {
      preferences: new CallPreferencesState('busy'),
      inCall: true
    });
    await expect
      .element(screen.getByRole('button', { name: 'Start microphone test' }))
      .toBeDisabled();
  });

  it('releases capture that resolves after the settings page closes', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([visibleCamera]);
    let resolve!: (stream: MediaStream) => void;
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        })
    );
    const stop = vi.fn();
    const screen = render(CallDeviceSettings, { preferences: new CallPreferencesState('close') });
    await screen.getByRole('button', { name: 'Start microphone test' }).click();
    await screen.unmount();
    resolve({ getTracks: () => [{ stop }] } as unknown as MediaStream);
    await vi.waitFor(() => expect(stop).toHaveBeenCalledOnce());
  });
});

it('offers a continuous voice slider while keeping the gate separate', async () => {
  vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([visibleCamera]);
  const preferences = new CallPreferencesState('voice-ui');
  preferences.setMicrophoneThreshold(-30);
  const screen = render(CallDeviceSettings, { preferences });
  const slider = screen.getByRole('slider', { name: /^Voice Quality/ });
  await expect.element(slider).toHaveValue('0');
  slider.element().focus();
  await userEvent.keyboard('{End}');
  expect(new CallPreferencesState('voice-ui').voiceAmount).toBe(100);
  expect(screen.container.querySelector('[data-rainbow-band]')).not.toBeNull();
  expect(screen.container.querySelector('.awesome-text')).not.toBeNull();
  await expect.element(slider).toHaveAttribute('aria-valuetext', 'AWESOME');
  const input = slider.element() as HTMLInputElement;
  input.value = '37.5';
  input.dispatchEvent(new Event('input', { bubbles: true }));
  await tick();
  expect(preferences.voiceAmount).toBe(37.5);
  expect(screen.container.querySelector('[data-rainbow-band]')).toBeNull();
  expect(preferences.effects.treble).toBe(2.25);
  await userEvent.keyboard('{Home}');
  expect(preferences.effects.compressor).toBe(false);
  expect(preferences.microphoneThreshold).toBe(-30);
  expect(screen.container.querySelectorAll('input[type=range]')).toHaveLength(2);
  expect(screen.container.querySelector('details')).toBeNull();
  await expect.element(screen.getByText('Pretty cool', { exact: true })).toBeVisible();
});

it('disables the voice slider when processing is unavailable', async () => {
  vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([visibleCamera]);
  const screen = render(CallDeviceSettings, {
    preferences: new CallPreferencesState('unavailable-voice'),
    inCall: true,
    gateUnavailable: true
  });
  await expect.element(screen.getByRole('slider', { name: /^Voice Quality/ })).toBeDisabled();
});

it('switches the selected test output without stopping capture or requiring another Start click', async () => {
  const context = new AudioContext();
  const stream = context.createMediaStreamDestination().stream;
  const preferences = new CallPreferencesState('live-output');
  preferences.setVoiceAmount(50);
  const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
  const sink = vi.spyOn(AudioContext.prototype as OutputAudioContext, 'setSinkId').mockResolvedValue(undefined);
  vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([
    visibleCamera,
    {kind: 'audiooutput', deviceId: 'headphones', label: 'Headphones'} as MediaDeviceInfo
  ]);
  const screen = render(CallDeviceSettings, {preferences});
  try {
    await screen.getByRole('button', {name: 'Start microphone test'}).click();
    await expect.element(screen.getByRole('button', {name: 'Stop test'})).toBeInTheDocument();
    await screen.getByRole('radio', {name: 'Headphones'}).click();
    await vi.waitFor(() => expect(sink).toHaveBeenLastCalledWith('headphones'));
    await vi.waitFor(() => expect(preferences.speaker).toBe('headphones'));
    expect(capture).toHaveBeenCalledOnce();
    expect(stream.getAudioTracks()[0].readyState).toBe('live');
    await expect.element(screen.getByRole('button', {name: 'Stop test'})).toBeInTheDocument();
    await screen.getByRole('radiogroup', {name:'Speaker', exact:true})
      .getByRole('radio', {name:'System default'}).click();
    await vi.waitFor(() => expect(sink).toHaveBeenLastCalledWith(''));
    expect(capture).toHaveBeenCalledOnce();
  } finally {
    await screen.unmount();
    await context.close();
    vi.restoreAllMocks();
  }
});
