import { beforeEach, describe, expect, it } from 'vitest';
import { availableCallDevice, CallPreferencesState } from './callPreferences.svelte';

describe('CallPreferencesState', () => {
  beforeEach(() => localStorage.clear());

  it('persists sensitivity per server and defaults older preferences to off', () => {
    const state = new CallPreferencesState('sensitivity');
    expect(state.microphoneThreshold).toBe(-60);
    state.setMicrophoneThreshold(-32);
    expect(new CallPreferencesState('sensitivity').microphoneThreshold).toBe(-32);
    expect(new CallPreferencesState('other').microphoneThreshold).toBe(-60);
    state.setMicrophoneThreshold(NaN);
    expect(new CallPreferencesState('sensitivity').microphoneThreshold).toBe(-60);
  });

  it('restores devices and join-muted independently for each server', () => {
    const first = new CallPreferencesState('first');
    first.setDevice('audioinput', 'mic');
    first.setDevice('audiooutput', 'speaker');
    first.setDevice('videoinput', 'camera');
    first.setJoinMuted(true);
    const restored = new CallPreferencesState('first');
    expect([restored.microphone, restored.speaker, restored.camera, restored.joinMuted]).toEqual([
      'mic',
      'speaker',
      'camera',
      true
    ]);
    const other = new CallPreferencesState('other');
    expect([other.microphone, other.speaker, other.camera, other.joinMuted]).toEqual([
      '',
      '',
      '',
      false
    ]);
  });

  it('retains unavailable devices for their return and supports resetting to default', () => {
    const state = new CallPreferencesState('first');
    state.setDevice('audioinput', 'mic');
    expect(availableCallDevice(state.microphone, [])).toBe('');
    expect(state.microphone).toBe('mic');
    expect(availableCallDevice(state.microphone, [{ deviceId: 'mic' } as MediaDeviceInfo])).toBe(
      'mic'
    );
    state.setDevice('audioinput', '');
    expect(new CallPreferencesState('first').microphone).toBe('');
  });

  it('normalizes corrupt and partially invalid storage without enabling capture', () => {
    localStorage.setItem(
      'chatto:i:first:callPreferences',
      '{"microphone":5,"camera":"cam","joinMuted":"true"}'
    );
    const state = new CallPreferencesState('first');
    expect(state.microphone).toBe('');
    expect(state.camera).toBe('cam');
    expect(state.joinMuted).toBe(false);
    localStorage.setItem('chatto:i:first:callPreferences', 'invalid');
    expect(new CallPreferencesState('first').camera).toBe('');
  });
});

it('normalizes, persists and resets effects without changing device or join choices', () => {
  const state = new CallPreferencesState('effects');
  state.setDevice('audioinput', 'chosen');
  state.setJoinMuted(true);
  state.setMicrophoneThreshold(-25);
  state.setEffects({
    equalizer: true,
    bass: 500,
    mid: NaN,
    treble: -500,
    compressor: true,
    amount: 150
  });
  expect(new CallPreferencesState('effects').effects).toEqual({
    lowCut: false,
    equalizer: true,
    bass: 6,
    mid: 0,
    treble: -6,
    compressor: true,
    amount: 100
  });
  state.resetProcessing();
  const restored = new CallPreferencesState('effects');
  expect(restored.effects).toEqual({
    lowCut: false,
    equalizer: false,
    bass: 0,
    mid: 0,
    treble: 0,
    compressor: false,
    amount: 50
  });
  expect(restored.microphoneThreshold).toBe(-60);
  expect(restored.microphone).toBe('chosen');
  expect(restored.joinMuted).toBe(true);
});
