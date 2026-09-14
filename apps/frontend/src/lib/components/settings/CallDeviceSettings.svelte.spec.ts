import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { CallPreferencesState } from '$lib/state/server/callPreferences.svelte';
import CallDeviceSettings from './CallDeviceSettings.svelte';

describe('Call device settings', () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => vi.restoreAllMocks());

  it('shows remembered unavailable devices without requesting capture', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([]);
    const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia');
    const preferences = new CallPreferencesState('settings-test');
    preferences.setDevice('audioinput', 'missing');
    const screen = render(CallDeviceSettings, { preferences });
    await expect
      .element(screen.getByRole('option', { name: 'Saved device unavailable' }))
      .toBeInTheDocument();
    expect(capture).not.toHaveBeenCalled();
    await screen.getByRole('checkbox', { name: 'Join calls with microphone muted' }).click();
    expect(new CallPreferencesState('settings-test').joinMuted).toBe(true);
    await screen.getByRole('combobox', { name: 'Microphone', exact: true }).selectOptions('');
    expect(new CallPreferencesState('settings-test').microphone).toBe('');
  });

  it('keeps a recorded clip playable when the device list refreshes', async () => {
    const enumerate = vi
      .spyOn(navigator.mediaDevices, 'enumerateDevices')
      .mockImplementation(async () => []);
    const context = new AudioContext();
    const oscillator = context.createOscillator();
    const destination = context.createMediaStreamDestination();
    oscillator.connect(destination);
    oscillator.start();
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockImplementation(async () => {
      await context.resume();
      return destination.stream;
    });
    const screen = render(CallDeviceSettings, {
      preferences: new CallPreferencesState('playback-refresh')
    });
    try {
      await screen.getByRole('button', { name: 'Start microphone test' }).click();
      await screen.getByRole('button', { name: 'Record a test' }).click();
      await screen.getByRole('button', { name: 'Stop recording' }).click();
      await vi.waitFor(() => expect(screen.container.querySelector('audio')).not.toBeNull());
      const audio = screen.container.querySelector('audio')!;
      const source = audio.getAttribute('src');
      expect(source).toMatch(/^blob:/);
      const callsBefore = enumerate.mock.calls.length;
      navigator.mediaDevices.dispatchEvent(new Event('devicechange'));
      await vi.waitFor(() => expect(enumerate.mock.calls.length).toBeGreaterThan(callsBefore));
      await tick();
      expect(audio.getAttribute('src')).toBe(source);
    } finally {
      await screen.unmount();
      oscillator.stop();
      await context.close();
    }
  });

  it('blocks microphone testing during a call', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([]);
    const screen = render(CallDeviceSettings, {
      preferences: new CallPreferencesState('busy'),
      inCall: true
    });
    await expect
      .element(screen.getByRole('button', { name: 'Start microphone test' }))
      .toBeDisabled();
  });

  it('releases capture that resolves after the settings page closes', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([]);
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
