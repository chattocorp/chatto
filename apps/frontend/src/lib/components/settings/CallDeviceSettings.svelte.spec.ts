import { userEvent } from 'vitest/browser';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
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
    const play = vi.spyOn(HTMLMediaElement.prototype, 'play');
    const screen = render(CallDeviceSettings, {
      preferences: new CallPreferencesState('playback-refresh')
    });
    try {
      await screen.getByRole('button', { name: 'Start microphone test' }).click();
      await expect.element(screen.getByRole('button', { name: 'Stop test' })).toBeInTheDocument();
      await vi.waitFor(() => expect(play).toHaveBeenCalledOnce());
      const audio = play.mock.contexts[0] as HTMLAudioElement;
      await vi.waitFor(() => expect(audio.paused).toBe(false));
      const callsBefore = enumerate.mock.calls.length;
      navigator.mediaDevices.dispatchEvent(new Event('devicechange'));
      await vi.waitFor(() => expect(enumerate.mock.calls.length).toBeGreaterThan(callsBefore));
      await tick();
      expect((audio.srcObject as MediaStream).getAudioTracks()[0].readyState).toBe('live');
      expect(audio.paused).toBe(false);
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

it('enables effects independently, persists keyboard edits and resets processing', async () => {
  vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([visibleCamera]);
  const preferences = new CallPreferencesState('processing-ui');
  const screen = render(CallDeviceSettings, { preferences });
  await expect.element(screen.getByRole('checkbox', { name: 'Low-cut filter' })).toBeVisible();
  expect(screen.container.querySelector('details')).toBeNull();
  await expect.element(screen.getByRole('slider', { name: /^Bass/ })).toBeDisabled();
  await screen.getByRole('checkbox', { name: 'Equaliser', exact: true }).click();
  const bass = screen.container.querySelector<HTMLInputElement>('#microphone-bass')!;
  bass.focus();
  await userEvent.keyboard('{End}');
  expect(new CallPreferencesState('processing-ui').effects.bass).toBe(6);
  await screen.getByRole('checkbox', { name: 'Compressor', exact: true }).click();
  await expect.element(screen.getByRole('slider', { name: /^Amount/ })).toBeEnabled();
  expect(preferences.effects.lowCut).toBe(false);
  await screen.getByRole('button', { name: 'Reset processing' }).click();
  expect(preferences.effects.equalizer).toBe(false);
  expect(preferences.effects.compressor).toBe(false);
  expect(preferences.effects.bass).toBe(0);
});
