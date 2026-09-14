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
